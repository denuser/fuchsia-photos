"""Optional CPU face inference on a viewer preview."""

import hashlib
import io
import math
import os
import threading
from pathlib import Path

from PIL import Image, UnidentifiedImageError

MAX_UPLOAD_BYTES = 16 * 1024 * 1024
MAX_IMAGE_PIXELS = 20_000_000
MAX_FACES = 64
DIMENSIONS = 128
# Old film scans are grainy, flat and often black and white, so the detector is
# unsure of faces it finds at once on a modern photo. When the first pass finds
# nothing, the preview is retried with its contrast stretched (and enlarged if
# small) at this lower confidence. ML_FACE_FALLBACK_SCORE overrides it; 0 or
# a value of 0.9 and above turns the retry off.
FALLBACK_SCORE = 0.6
FALLBACK_UPSCALE_BELOW = 1600


# The weights ML_FACES downloads from Hugging Face, with their licenses, into its
# cache. A file whose hash differs is refused, so the model identifier and the
# stored face vectors stay the same as with files supplied by hand.
WEIGHTS = (
    (
        "opencv/face_detection_yunet",
        "face_detection_yunet_2023mar.onnx",
        "8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4",
    ),
    (
        "opencv/face_recognition_sface",
        "face_recognition_sface_2021dec.onnx",
        "0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79",
    ),
)


def download(weights: tuple[str, str, str]) -> str:
    from huggingface_hub import hf_hub_download

    repo, filename, sha256 = weights
    path = hf_hub_download(repo, filename)
    hf_hub_download(repo, "LICENSE")
    with open(path, "rb") as file:
        digest = hashlib.file_digest(file, "sha256").hexdigest()
    if digest != sha256:
        raise RuntimeError(f"{repo}/{filename} has SHA-256 {digest}, expected {sha256}")
    return path


class InvalidPreview(ValueError):
    pass


class FaceInference:
    def __init__(self, detector_path: str, recognizer_path: str) -> None:
        import cv2
        import numpy as np

        self._cv2 = cv2
        self._np = np
        self._lock = threading.Lock()
        digests = []
        for path in (detector_path, recognizer_path):
            with Path(path).open("rb") as weights:
                digests.append(hashlib.file_digest(weights, "sha256").hexdigest())
        # Bump the version when preprocessing or inference settings change.
        self.model = f"yunet-sface@1/{cv2.__version__}/{'/'.join(digests)}"
        self._detector = cv2.FaceDetectorYN.create(
            detector_path, "", (320, 320), 0.9, 0.3, 5000
        )
        self._recognizer = cv2.FaceRecognizerSF.create(recognizer_path, "")
        try:
            self._fallback_score = float(os.environ.get("ML_FACE_FALLBACK_SCORE", FALLBACK_SCORE))
        except ValueError:
            self._fallback_score = FALLBACK_SCORE

    @classmethod
    def from_environment(cls, token: str) -> "FaceInference | None":
        enabled = os.environ.get("ML_FACES", "").lower() in ("1", "true", "yes")
        detector = os.environ.get("ML_FACE_DETECTOR", "")
        recognizer = os.environ.get("ML_FACE_RECOGNIZER", "")
        if not enabled and not detector and not recognizer:
            return None
        if bool(detector) != bool(recognizer):
            raise RuntimeError("Set both ML_FACE_DETECTOR and ML_FACE_RECOGNIZER, or neither")
        if not token:
            raise RuntimeError("ML_TOKEN is required for face inference")
        if not detector:
            detector, recognizer = (download(weights) for weights in WEIGHTS)
        return cls(detector, recognizer)

    def analyze(self, preview: bytes | bytearray) -> list[dict]:
        try:
            with Image.open(io.BytesIO(preview)) as image:
                width, height = image.size
                if width * height > MAX_IMAGE_PIXELS or min(width, height) < 16:
                    raise InvalidPreview("unsupported preview dimensions")
                bgr = self._cv2.cvtColor(self._np.asarray(image.convert("RGB")), self._cv2.COLOR_RGB2BGR)
        except (UnidentifiedImageError, OSError, Image.DecompressionBombError) as error:
            raise InvalidPreview("cannot decode preview") from error

        results = []
        with self._lock:
            bgr, detections, retry = self._detect(bgr)
            if detections is None:
                return results
            for face in sorted(detections, key=lambda row: row[-1], reverse=True)[:MAX_FACES]:
                x, y, w, h = map(float, face[:4])
                left, top = max(0.0, x), max(0.0, y)
                right, bottom = min(bgr.shape[1], x + w), min(bgr.shape[0], y + h)
                if right <= left or bottom <= top:
                    continue
                aligned = self._recognizer.alignCrop(bgr, face)
                vector = self._recognizer.feature(aligned).reshape(-1)
                norm = float(self._np.linalg.norm(vector))
                if vector.size != DIMENSIONS or not math.isfinite(norm) or norm == 0:
                    raise RuntimeError("invalid face embedding")
                results.append({
                    "box": [
                        left / bgr.shape[1],
                        top / bgr.shape[0],
                        (right - left) / bgr.shape[1],
                        (bottom - top) / bgr.shape[0],
                    ],
                    "embedding": (vector / norm).tolist(),
                    "score": float(face[-1]),
                    "retry": retry,
                })
        return results

    def _run(self, bgr, score: float):
        height, width = bgr.shape[:2]
        self._detector.setScoreThreshold(score)
        self._detector.setInputSize((width, height))
        return self._detector.detect(bgr)[1]

    @property
    def retry_floor(self) -> float | None:
        """The least score of a face found by the contrast retry, or None if it is off."""
        return self._fallback_score if 0 < self._fallback_score < 0.9 else None

    def _detect(self, bgr):
        """Return the image the faces were found on, the detections (or None),
        and whether they come from the contrast retry."""
        found = self._run(bgr, 0.9)
        if found is not None or self.retry_floor is None:
            return bgr, found, False
        cv2 = self._cv2
        lab = cv2.cvtColor(bgr, cv2.COLOR_BGR2LAB)
        clahe = cv2.createCLAHE(clipLimit=3.0, tileGridSize=(8, 8))
        lab[:, :, 0] = clahe.apply(lab[:, :, 0])
        enhanced = cv2.cvtColor(lab, cv2.COLOR_LAB2BGR)
        found = self._run(enhanced, self._fallback_score)
        if found is not None:
            return enhanced, found, True
        height, width = enhanced.shape[:2]
        if max(width, height) < FALLBACK_UPSCALE_BELOW:
            enlarged = cv2.resize(enhanced, None, fx=2, fy=2, interpolation=cv2.INTER_CUBIC)
            return enlarged, self._run(enlarged, self._fallback_score), True
        return enhanced, None, True

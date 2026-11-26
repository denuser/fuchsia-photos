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
            self._detector.setInputSize((width, height))
            _, detections = self._detector.detect(bgr)
            if detections is None:
                return results
            for face in sorted(detections, key=lambda row: row[-1], reverse=True)[:MAX_FACES]:
                x, y, w, h = map(float, face[:4])
                left, top = max(0.0, x), max(0.0, y)
                right, bottom = min(width, x + w), min(height, y + h)
                if right <= left or bottom <= top:
                    continue
                aligned = self._recognizer.alignCrop(bgr, face)
                vector = self._recognizer.feature(aligned).reshape(-1)
                norm = float(self._np.linalg.norm(vector))
                if vector.size != DIMENSIONS or not math.isfinite(norm) or norm == 0:
                    raise RuntimeError("invalid face embedding")
                results.append({
                    "box": [left / width, top / height, (right - left) / width, (bottom - top) / height],
                    "embedding": (vector / norm).tolist(),
                })
        return results

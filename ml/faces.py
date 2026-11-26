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
        detector = os.environ.get("ML_FACE_DETECTOR", "")
        recognizer = os.environ.get("ML_FACE_RECOGNIZER", "")
        if not detector and not recognizer:
            return None
        if not detector or not recognizer:
            raise RuntimeError("Set both ML_FACE_DETECTOR and ML_FACE_RECOGNIZER")
        if not token:
            raise RuntimeError("ML_TOKEN is required for face inference")
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

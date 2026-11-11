"""Stateless ML service of Fuchsia Photos.

It turns photos and search texts into vectors and keeps nothing: the
application stores the vectors and checks permissions. See the "Machine
learning" section of docs/architecture.md.
"""

import io
import os
import threading

import torch
from fastapi import Depends, FastAPI, Header, HTTPException, UploadFile
from PIL import Image
from pydantic import BaseModel
from transformers import AutoModel, AutoProcessor

MODEL_NAME = os.environ.get("ML_MODEL", "google/siglip2-base-patch16-384")
# Change it whenever the preprocessing below changes, so that the application
# does not mix vectors made in different ways.
PREPROCESSING_VERSION = 1
MODEL_ID = f"{MODEL_NAME}@{PREPROCESSING_VERSION}"
TOKEN = os.environ.get("ML_TOKEN", "")
# The least cosine similarity between a search text and a photo for the photo
# to count as a match. Absolute similarities differ between models, so each
# needs its own. Calibrated on Russian queries against a home library on
# 2026-11-10: matches came down to 0.068 and unrelated photos up to 0.072, so
# no value separates them. 0.070 keeps nearly all matches and lets a few
# unrelated photos in at the end of the list. Without a value for the model,
# search returns the nearest photos.
MIN_SIMILARITY = {
    "google/siglip2-base-patch16-384": 0.070,
}
min_similarity_setting = os.environ.get("ML_MIN_SIMILARITY")
MIN_SIMILARITY_FOR_MODEL = (
    float(min_similarity_setting) if min_similarity_setting else MIN_SIMILARITY.get(MODEL_NAME)
)
MAX_BATCH = 64
# SigLIP 2 was trained on lowercase text padded to 64 tokens.
TEXT_LENGTH = 64


def pick_device() -> str:
    if torch.cuda.is_available():
        return "cuda"
    if torch.backends.mps.is_available():
        return "mps"
    return "cpu"


DEVICE = pick_device()
# float32 everywhere, so that vectors match across devices.
model = AutoModel.from_pretrained(MODEL_NAME, dtype=torch.float32).to(DEVICE).eval()
processor = AutoProcessor.from_pretrained(MODEL_NAME)
# One model on one device: run one request at a time.
lock = threading.Lock()

app = FastAPI(title="Fuchsia Photos ML")


def check_token(authorization: str = Header(default="")) -> None:
    if TOKEN and authorization != f"Bearer {TOKEN}":
        raise HTTPException(status_code=401, detail="invalid token")


def normalized(features: torch.Tensor) -> list[list[float]]:
    features = torch.nn.functional.normalize(features.float(), dim=-1)
    return features.cpu().tolist()


def as_tensor(output) -> torch.Tensor:
    # Newer transformers return a model output instead of a bare tensor.
    if isinstance(output, torch.Tensor):
        return output
    return output.pooler_output


def text_features(text: str) -> torch.Tensor:
    inputs = processor(
        text=[text], padding="max_length", max_length=TEXT_LENGTH, truncation=True, return_tensors="pt"
    ).to(DEVICE)
    with lock, torch.inference_mode():
        return as_tensor(model.get_text_features(**inputs))


DIMENSIONS = text_features("photo").shape[-1]


@app.get("/health", dependencies=[Depends(check_token)])
def health() -> dict:
    return {"model": MODEL_ID, "dimensions": DIMENSIONS, "device": DEVICE, "minSimilarity": MIN_SIMILARITY_FOR_MODEL}


class TextRequest(BaseModel):
    text: str


@app.post("/embed/text", dependencies=[Depends(check_token)])
def embed_text(request: TextRequest) -> dict:
    text = request.text.strip().lower()
    if not text:
        raise HTTPException(status_code=400, detail="empty text")
    return {
        "model": MODEL_ID,
        "embedding": normalized(text_features(text))[0],
        "minSimilarity": MIN_SIMILARITY_FOR_MODEL,
    }


@app.post("/embed/image", dependencies=[Depends(check_token)])
def embed_image(images: list[UploadFile]) -> dict:
    """Returns one result per image, in order: an embedding or an error.

    A file that cannot be decoded fails alone, so the application can skip it
    and keep the rest of the batch.
    """
    if len(images) > MAX_BATCH:
        raise HTTPException(status_code=400, detail=f"at most {MAX_BATCH} images")
    results: list[dict] = [{} for _ in images]
    decoded: list[Image.Image] = []
    positions: list[int] = []
    for i, upload in enumerate(images):
        try:
            image = Image.open(io.BytesIO(upload.file.read()))
            decoded.append(image.convert("RGB"))
            positions.append(i)
        except Exception as error:  # noqa: BLE001 - any decoding failure is reported per image
            results[i] = {"error": f"decode: {error}"}
    if decoded:
        inputs = processor(images=decoded, return_tensors="pt").to(DEVICE)
        with lock, torch.inference_mode():
            features = as_tensor(model.get_image_features(**inputs))
        for position, embedding in zip(positions, normalized(features)):
            results[position] = {"embedding": embedding}
    return {"model": MODEL_ID, "results": results}

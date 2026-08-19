"""
M8-D: Embedding endpoint for Knowledge Base vector generation.
Integrated into the agent-service FastAPI app.
"""
from __future__ import annotations

import time
from typing import List

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field


class EmbeddingRequest(BaseModel):
    text: str = Field(..., min_length=1, max_length=50000)
    model: str = Field(default="text-embedding-3-small")


class EmbeddingResponse(BaseModel):
    embedding: List[float]
    model: str
    usage: int
    elapsed_ms: int


router = APIRouter(prefix="/v1", tags=["embeddings"])

_embedding_dim = 1536


def _generate_mock_embedding(text: str, dim: int = _embedding_dim) -> List[float]:
    import hashlib

    h = hashlib.sha256(text.encode()).digest()
    seed = int.from_bytes(h[:8], "big")
    import struct
    import math

    values = []
    for i in range(dim):
        seed = (seed * 1103515245 + 12345) & 0x7FFFFFFF
        val = (seed / 0x7FFFFFFF) * 2 - 1
        values.append(val)

    norm = math.sqrt(sum(v * v for v in values))
    if norm > 0:
        values = [v / norm for v in values]
    return values


@router.post("/embeddings", response_model=EmbeddingResponse)
async def create_embedding(req: EmbeddingRequest) -> EmbeddingResponse:
    start = time.time()

    text = req.text.strip()
    if not text:
        raise HTTPException(status_code=400, detail="Empty text")

    embedding = _generate_mock_embedding(text)
    elapsed = int((time.time() - start) * 1000)

    return EmbeddingResponse(
        embedding=embedding,
        model=req.model,
        usage=len(text),
        elapsed_ms=elapsed,
    )


@router.get("/embeddings/health")
async def embedding_health():
    return {"status": "ok", "model": "text-embedding-3-small (mock)", "dimensions": _embedding_dim}
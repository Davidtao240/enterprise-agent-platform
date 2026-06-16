"""File loading utilities with three-tier fallback: MinIO → inline data → sample data."""

from __future__ import annotations

import io
import logging
from typing import Any

import pandas as pd

from app.core.config import settings

logger = logging.getLogger(__name__)

# Canonical sample data from SAMPLE_FINANCE_DATA.md
_SAMPLE_ROWS: list[dict[str, Any]] = [
    {
        "month": "2026-05",
        "department": "Finance Center",
        "revenue": 1200000,
        "cost": 760000,
        "gross_profit": 440000,
        "net_profit": 310000,
        "customer_count": 860,
        "order_count": 1430,
    },
    {
        "month": "2026-05",
        "department": "East Region",
        "revenue": 680000,
        "cost": 420000,
        "gross_profit": 260000,
        "net_profit": 180000,
        "customer_count": 420,
        "order_count": 760,
    },
    {
        "month": "2026-05",
        "department": "South Region",
        "revenue": 520000,
        "cost": 340000,
        "gross_profit": 180000,
        "net_profit": 130000,
        "customer_count": 310,
        "order_count": 540,
    },
]

_SAMPLE_COLUMNS = [
    "month", "department", "revenue", "cost", "gross_profit",
    "net_profit", "customer_count", "order_count",
]


async def fetch_file_from_minio(file_id: str) -> bytes:
    """Download file content from MinIO by file_id."""
    try:
        from minio import Minio
    except ImportError:
        raise RuntimeError("minio package not installed")

    client = Minio(
        settings.minio_endpoint,
        access_key=settings.minio_access_key,
        secret_key=settings.minio_secret_key,
        secure=False,
    )
    try:
        response = client.get_object(settings.minio_bucket, file_id)
        return response.read()
    except Exception as e:
        raise FileNotFoundError(f"File '{file_id}' not found in MinIO: {e}")
    finally:
        try:
            response.close()
            response.release_conn()
        except Exception:
            pass


def parse_csv(content: bytes | str) -> list[dict[str, Any]]:
    """Parse CSV content into list of dicts, normalizing column names."""
    if isinstance(content, bytes):
        content = content.decode("utf-8", errors="replace")
    df = pd.read_csv(io.StringIO(content))
    df.columns = [str(c).strip() for c in df.columns]
    # Convert numeric columns
    for col in df.columns:
        try:
            df[col] = pd.to_numeric(df[col])
        except (ValueError, TypeError):
            pass
    return df.to_dict(orient="records")


def parse_excel(content: bytes) -> list[dict[str, Any]]:
    """Parse Excel content into list of dicts."""
    df = pd.read_excel(io.BytesIO(content))
    df.columns = [str(c).strip() for c in df.columns]
    for col in df.columns:
        try:
            df[col] = pd.to_numeric(df[col])
        except (ValueError, TypeError):
            pass
    return df.to_dict(orient="records")


def generate_sample_data() -> tuple[list[str], list[dict[str, Any]]]:
    """Return the canonical sample finance dataset."""
    return _SAMPLE_COLUMNS, _SAMPLE_ROWS


async def load_data(
    file_id: str | None = None,
    inline_data: list[dict[str, Any]] | None = None,
) -> tuple[list[str], list[dict[str, Any]], list[str]]:
    """Three-tier file loading with automatic fallback.

    Returns (columns, rows, warnings).
    """
    warnings: list[str] = []

    # Tier 1: inline data
    if inline_data:
        if isinstance(inline_data, list) and len(inline_data) > 0:
            if isinstance(inline_data[0], dict):
                columns = list(inline_data[0].keys())
                logger.info("Using inline data: %d rows, columns=%s", len(inline_data), columns)
                return columns, inline_data, warnings
            else:
                warnings.append("Inline data rows are not dicts, falling back.")
        else:
            warnings.append("Inline data is empty, falling back.")

    # Tier 2: MinIO file
    if file_id:
        try:
            content = await fetch_file_from_minio(file_id)
            ext = file_id.rsplit(".", 1)[-1].lower() if "." in file_id else ""
            if ext in ("xlsx", "xls"):
                rows = parse_excel(content)
            else:
                rows = parse_csv(content)
            if rows:
                columns = list(rows[0].keys())
                logger.info("Loaded file from MinIO: %d rows, columns=%s", len(rows), columns)
                return columns, rows, warnings
            warnings.append("MinIO file parsed but is empty, falling back to sample data.")
        except FileNotFoundError:
            warnings.append(f"File '{file_id}' not found in MinIO, using sample data.")
        except Exception as e:
            warnings.append(f"Failed to load file '{file_id}': {e}, using sample data.")

    # Tier 3: sample data fallback
    logger.info("Using sample data as fallback.")
    warnings.append("Using built-in sample finance data (no file uploaded).")
    return generate_sample_data()[0], generate_sample_data()[1], warnings

"""File loading utilities with three-tier fallback: MinIO → inline data → sample data."""

from __future__ import annotations

import io
import logging
from typing import Any, Callable

import httpx
import pandas as pd

from app.core.config import settings

logger = logging.getLogger(__name__)

async def fetch_file_from_minio(file_id: str) -> bytes:
    """Download file content from MinIO by file_id."""
    if settings.file_service_url:
        url = f"{settings.file_service_url.rstrip('/')}/internal/v1/files/{file_id}/content"
        async with httpx.AsyncClient(timeout=30.0) as client:
            try:
                resp = await client.get(url)
                resp.raise_for_status()
                return resp.content
            except Exception as e:
                logger.warning("Failed to load file '%s' from file service: %s", file_id, e)

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


async def load_data(
    file_id: str | None = None,
    inline_data: list[dict[str, Any]] | None = None,
    fallback_data: Callable[
        [],
        tuple[list[str], list[dict[str, Any]]],
    ] | None = None,
    fallback_warning: str = "未读取到有效数据，已使用配置的后备数据。",
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
                warnings.append("内联数据行格式无效，已切换到后备数据源。")
        else:
            warnings.append("内联数据为空，已切换到后备数据源。")

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
            warnings.append("上传文件解析后没有数据，已使用内置示例数据。")
        except FileNotFoundError:
            warnings.append(f"未找到文件“{file_id}”，已使用内置示例数据。")
        except Exception as e:
            warnings.append(f"文件“{file_id}”加载失败：{e}；已使用内置示例数据。")

    # Tier 3: graph-bound profile fallback
    if fallback_data is None:
        raise ValueError("No valid input data and no fallback data profile configured.")
    logger.info("Using profile data as fallback.")
    warnings.append(fallback_warning)
    columns, rows = fallback_data()
    return columns, rows, warnings

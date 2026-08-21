"""pgvector client for vector similarity search.

Replaces Qdrant for vector storage and retrieval.
Integrates with the shared PostgreSQL database.
"""

from __future__ import annotations

import logging
import time
from typing import Any

import psycopg
import psycopg.rows
from psycopg_pool import ConnectionPool

from app.core.config import settings

logger = logging.getLogger(__name__)


class PgVectorClient:
    """Thin client over PostgreSQL pgvector for embedding storage and cosine search."""

    def __init__(self, db_url: str | None = None) -> None:
        self._db_url = db_url if db_url is not None else settings.database_url
        if not self._db_url:
            raise ValueError("DATABASE_URL is required for pgvector client")
        self._pool: ConnectionPool | None = None

    def _get_pool(self) -> ConnectionPool:
        if self._pool is None:
            self._pool = ConnectionPool(
                self._db_url,
                min_size=1,
                max_size=10,
                row_factory=psycopg.rows.dict_row,
            )
        return self._pool

    def _run_with_retry(self, operation, *args, **kwargs):
        max_retries = 3
        for attempt in range(max_retries):
            try:
                return operation(*args, **kwargs)
            except Exception as e:
                if attempt == max_retries - 1:
                    raise
                logger.warning(f"pgvector operation failed (attempt {attempt+1}/{max_retries}): {e}")
                time.sleep(0.1 * (2 ** attempt))

    def ping(self) -> bool:
        try:
            pool = self._get_pool()
            with pool.connection() as conn:
                conn.execute("SELECT 1")
            return True
        except Exception:
            return False

    def insert_chunk(
        self,
        tenant_id: str,
        document_id: str,
        chunk_index: int,
        content: str,
        embedding: list[float],
        token_count: int | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> None:
        """Insert a single chunk with its embedding vector."""
        def _op():
            with self._get_pool().connection() as conn:
                with conn.cursor() as cur:
                    cur.execute(
                        """
                        INSERT INTO knowledge_chunks
                            (tenant_id, document_id, chunk_index, content, embedding, token_count, metadata_json)
                        VALUES (%s, %s, %s, %s, %s::vector, %s, %s::jsonb)
                        """,
                        (tenant_id, document_id, chunk_index, content, str(embedding), token_count, metadata),
                    )
                conn.commit()
        self._run_with_retry(_op)

    def insert_chunks_batch(
        self,
        chunks: list[dict[str, Any]],
    ) -> None:
        """Batch insert chunks. Each chunk must have: tenant_id, document_id, chunk_index, content, embedding."""
        if not chunks:
            return

        def _op():
            with self._get_pool().connection() as conn:
                with conn.cursor() as cur:
                    for chunk in chunks:
                        cur.execute(
                            """
                            INSERT INTO knowledge_chunks
                                (tenant_id, document_id, chunk_index, content, embedding, token_count, metadata_json)
                            VALUES (%s, %s, %s, %s, %s::vector, %s, %s::jsonb)
                            ON CONFLICT DO NOTHING
                            """,
                            (
                                chunk["tenant_id"],
                                chunk["document_id"],
                                chunk["chunk_index"],
                                chunk["content"],
                                str(chunk["embedding"]),
                                chunk.get("token_count"),
                                chunk.get("metadata"),
                            ),
                        )
                conn.commit()
        self._run_with_retry(_op)

    def search_similar(
        self,
        tenant_id: str,
        query_embedding: list[float],
        top_k: int = 5,
        document_id: str | None = None,
        min_similarity: float = 0.5,
    ) -> list[dict[str, Any]]:
        """Perform cosine similarity search within a tenant's chunks."""
        sql = """
            SELECT id, document_id, chunk_index, content, token_count,
                   1 - (embedding <=> %s::vector) AS similarity
            FROM knowledge_chunks
            WHERE tenant_id = %s
        """
        params: list[Any] = [str(query_embedding), tenant_id]

        if document_id is not None:
            sql += " AND document_id = %s"
            params.append(document_id)

        sql += """
            AND 1 - (embedding <=> %s::vector) >= %s
            ORDER BY similarity DESC
            LIMIT %s
        """
        params.extend([str(query_embedding), min_similarity, top_k])

        def _op():
            with self._get_pool().connection() as conn:
                with conn.cursor() as cur:
                    cur.execute(sql, params)
                    rows = cur.fetchall()
                    return [dict(row) for row in rows]
        return self._run_with_retry(_op)

    def delete_document_chunks(self, document_id: str) -> None:
        """Delete all chunks for a document."""
        def _op():
            with self._get_pool().connection() as conn:
                with conn.cursor() as cur:
                    cur.execute(
                        "DELETE FROM knowledge_chunks WHERE document_id = %s",
                        (document_id,),
                    )
                conn.commit()
        self._run_with_retry(_op)

    def mark_document_ready(self, document_id: str, chunk_count: int) -> None:
        """Update document status to ready after chunks are indexed."""
        def _op():
            with self._get_pool().connection() as conn:
                with conn.cursor() as cur:
                    cur.execute(
                        """
                        UPDATE knowledge_documents
                        SET status = 'ready',
                            chunk_count = %s,
                            updated_at = NOW()
                        WHERE id = %s
                        """,
                        (chunk_count, document_id),
                    )
                conn.commit()
        self._run_with_retry(_op)


_client: PgVectorClient | None = None


def get_pgvector_client() -> PgVectorClient:
    global _client
    if _client is None:
        _client = PgVectorClient()
    return _client
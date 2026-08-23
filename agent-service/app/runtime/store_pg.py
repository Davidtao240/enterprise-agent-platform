"""PostgreSQL-backed RuntimeStore — ADR-009.

Interface-compatible twin of ``app.runtime.store.RuntimeStore`` (SQLite).
Selected at startup when RUNTIME_DATABASE_URL / RUNTIME_STORE_BACKEND=postgres
is configured; unit tests and local dev keep the SQLite path unchanged.

Differences from the SQLite twin, by design:
- connection pooling (``psycopg_pool.ConnectionPool``) instead of one new
  connection per operation,
- writes serialize on the run row via ``SELECT ... FOR UPDATE`` — no
  process-wide asyncio lock; different runs execute concurrently,
- ``create_run`` is an atomic ``INSERT ... ON CONFLICT DO NOTHING`` replay,
- ``delivery_order`` is a PG identity column (SQLite used AUTOINCREMENT),
- ``checkpoint.saved`` events report ``backend: postgres``.
"""

from __future__ import annotations

import asyncio
import json
import uuid
from datetime import UTC, datetime, timedelta
from typing import Any, Callable

import psycopg
from psycopg.rows import dict_row
from psycopg_pool import ConnectionPool

from app.runtime.models import (
    AcceptedRunResponse,
    CancelRunRequest,
    ResumeRunRequest,
    RuntimeEventEnvelope,
    StartRunRequest,
)
from app.runtime.store import (
    TERMINAL_STATUSES,
    RuntimeStoreError,
    _canonical,
    _hash,
    _utcnow,
)

SCHEMA = "agent_runtime"

RUNS_TABLE = f"{SCHEMA}.runtime_v2_runs"
INTERRUPTS_TABLE = f"{SCHEMA}.runtime_v2_interrupts"
COMMANDS_TABLE = f"{SCHEMA}.runtime_v2_commands"
OUTBOX_TABLE = f"{SCHEMA}.runtime_v2_event_outbox"

# Lock wait budget: writes serialize on the run row via FOR UPDATE; a stuck
# holder must fail fast instead of piling up workers.
LOCK_TIMEOUT_SECONDS = 10

_DDL_STATEMENTS: tuple[str, ...] = (
    f"CREATE SCHEMA IF NOT EXISTS {SCHEMA}",
    f"""CREATE TABLE IF NOT EXISTS {RUNS_TABLE} (
        run_id TEXT PRIMARY KEY,
        request_hash TEXT NOT NULL,
        request_json TEXT NOT NULL,
        thread_id TEXT NOT NULL,
        tenant_id TEXT NOT NULL,
        graph_key TEXT NOT NULL,
        graph_version TEXT NOT NULL,
        attempt BIGINT NOT NULL CHECK (attempt > 0),
        status TEXT NOT NULL,
        checkpoint_version BIGINT NOT NULL DEFAULT 0 CHECK (checkpoint_version >= 0),
        next_sequence BIGINT NOT NULL DEFAULT 0 CHECK (next_sequence >= 0),
        next_step_sequence BIGINT NOT NULL DEFAULT 0 CHECK (next_step_sequence >= 0),
        active_step_id TEXT,
        active_step_sequence BIGINT,
        active_interrupt_id TEXT,
        accepted_at TEXT NOT NULL,
        updated_at TEXT NOT NULL
    )""",
    f"""CREATE TABLE IF NOT EXISTS {INTERRUPTS_TABLE} (
        interrupt_id TEXT PRIMARY KEY,
        run_id TEXT NOT NULL REFERENCES {RUNS_TABLE}(run_id),
        checkpoint_version BIGINT NOT NULL CHECK (checkpoint_version > 0),
        kind TEXT NOT NULL,
        status TEXT NOT NULL,
        resume_schema_json TEXT NOT NULL,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL
    )""",
    f"""CREATE TABLE IF NOT EXISTS {COMMANDS_TABLE} (
        run_id TEXT NOT NULL REFERENCES {RUNS_TABLE}(run_id),
        operation TEXT NOT NULL,
        idempotency_key TEXT NOT NULL,
        request_hash TEXT NOT NULL,
        request_json TEXT NOT NULL,
        response_json TEXT NOT NULL,
        status TEXT NOT NULL,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL,
        PRIMARY KEY (run_id, operation, idempotency_key)
    )""",
    # Single in-flight resume per run (same semantics as the SQLite twin).
    f"""CREATE UNIQUE INDEX IF NOT EXISTS idx_runtime_v2_one_active_resume
        ON {COMMANDS_TABLE} (run_id)
        WHERE operation = 'resume' AND status = 'accepted'""",
    f"""CREATE TABLE IF NOT EXISTS {OUTBOX_TABLE} (
        delivery_order BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
        event_id TEXT NOT NULL UNIQUE,
        run_id TEXT NOT NULL REFERENCES {RUNS_TABLE}(run_id),
        sequence BIGINT NOT NULL,
        body_json TEXT NOT NULL,
        status TEXT NOT NULL DEFAULT 'pending',
        delivery_attempts BIGINT NOT NULL DEFAULT 0,
        last_error TEXT,
        next_attempt_at TEXT,
        delivered_at TEXT,
        created_at TEXT NOT NULL,
        UNIQUE (run_id, sequence)
    )""",
    # Recovery scan (list_recoverable_run_ids) and delivery scans.
    f"CREATE INDEX IF NOT EXISTS idx_runtime_v2_runs_status ON {RUNS_TABLE} (status)",
    f"CREATE INDEX IF NOT EXISTS idx_runtime_v2_outbox_pending ON {OUTBOX_TABLE} (run_id, status)",
)


class PostgresRuntimeStore:
    """Runtime metadata + at-least-once event outbox on PostgreSQL (ADR-009)."""

    backend_name = "postgres"

    def __init__(self, dsn: str) -> None:
        self.dsn = dsn
        self._pool: ConnectionPool | None = None

    def _get_pool(self) -> ConnectionPool:
        # 惰性建池:setup() 正常路径先调用,这里兜底(例如测试直接调用方法)。
        if self._pool is None:
            self._pool = ConnectionPool(
                conninfo=self.dsn,
                min_size=1,
                max_size=8,
                kwargs={"row_factory": dict_row, "autocommit": False},
                timeout=LOCK_TIMEOUT_SECONDS,
                open=True,
            )
        return self._pool

    async def setup(self) -> None:
        await asyncio.to_thread(self._setup_sync)

    def _setup_sync(self) -> None:
        with self._get_pool().connection() as conn:
            for statement in _DDL_STATEMENTS:
                conn.execute(statement)

    async def close(self) -> None:
        """关闭连接池(进程优雅退出时调用)。"""
        pool, self._pool = self._pool, None
        if pool is not None:
            await asyncio.to_thread(pool.close)

    async def _write(self, operation: Callable[[psycopg.Connection], Any]) -> Any:
        # 无进程级全局锁:同一 run 的状态迁移由 SELECT ... FOR UPDATE 行锁
        # 串行化,不同 run 的写入在池中不同连接上并发执行。
        return await asyncio.to_thread(self._write_sync, operation)

    def _write_sync(self, operation: Callable[[psycopg.Connection], Any]) -> Any:
        # psycopg 在 with 块退出时提交;行级锁 + lock_timeout 取代 SQLite
        # 的 BEGIN IMMEDIATE 数据库锁。
        with self._get_pool().connection() as conn:
            conn.execute(f"SET LOCAL lock_timeout = '{LOCK_TIMEOUT_SECONDS}s'")
            return operation(conn)

    async def _read(self, operation: Callable[[psycopg.Connection], Any]) -> Any:
        return await asyncio.to_thread(self._read_sync, operation)

    def _read_sync(self, operation: Callable[[psycopg.Connection], Any]) -> Any:
        with self._get_pool().connection() as conn:
            return operation(conn)

    async def create_run(self, request: StartRunRequest) -> tuple[AcceptedRunResponse, bool]:
        request_data = request.model_dump(mode="json")
        request_json = _canonical(request_data)
        request_hash = _hash(request_data)

        def operation(conn: psycopg.Connection) -> tuple[AcceptedRunResponse, bool]:
            now = _utcnow().isoformat()
            # 原子幂等插入:并发提交相同 run_id 时恰好一行落库,
            # 冲突方落到 SELECT 重放分支,避免 check-then-insert 竞态。
            inserted = conn.execute(
                f"""INSERT INTO {RUNS_TABLE}
                   (run_id, request_hash, request_json, thread_id, tenant_id,
                    graph_key, graph_version, attempt, status, accepted_at, updated_at)
                   VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)
                   ON CONFLICT (run_id) DO NOTHING
                   RETURNING *""",
                (
                    request.run_id,
                    request_hash,
                    request_json,
                    request.thread_id,
                    request.trusted_context.tenant_id,
                    request.graph.key,
                    request.graph.version,
                    request.attempt,
                    "queued",
                    now,
                    now,
                ),
            ).fetchone()
            if inserted is not None:
                return self._accepted(inserted, replayed=False), True

            existing = conn.execute(
                f"SELECT * FROM {RUNS_TABLE} WHERE run_id = %s", (request.run_id,)
            ).fetchone()
            if existing["request_hash"] != request_hash:
                raise RuntimeStoreError(
                    "RUN_IDEMPOTENCY_CONFLICT",
                    "run_id already exists with a different Start request",
                )
            return self._accepted(existing, replayed=True), False

        return await self._write(operation)

    @staticmethod
    def _accepted(row: dict[str, Any], replayed: bool) -> AcceptedRunResponse:
        return AcceptedRunResponse(
            run_id=row["run_id"],
            status=row["status"],
            accepted_at=datetime.fromisoformat(row["accepted_at"]),
            checkpoint_version=row["checkpoint_version"],
            replayed=replayed,
        )

    async def get_run(self, run_id: str) -> dict[str, Any]:
        def operation(conn: psycopg.Connection) -> dict[str, Any]:
            row = conn.execute(
                f"SELECT * FROM {RUNS_TABLE} WHERE run_id = %s", (run_id,)
            ).fetchone()
            if row is None:
                raise RuntimeStoreError("RUN_NOT_FOUND", "durable runtime run was not found", 404)
            value = dict(row)
            value["request"] = json.loads(value.pop("request_json"))
            return value

        return await self._read(operation)

    async def list_recoverable_run_ids(self) -> list[str]:
        return await self._read(
            lambda conn: [
                row["run_id"]
                for row in conn.execute(
                    f"SELECT run_id FROM {RUNS_TABLE} WHERE status IN ('queued','running')"
                ).fetchall()
            ]
        )

    async def begin_execution(self, run_id: str, recovered: bool) -> bool:
        def operation(conn: psycopg.Connection) -> bool:
            row = self._locked_run(conn, run_id)
            if row["status"] in TERMINAL_STATUSES or row["status"].startswith("waiting_"):
                return False
            now = _utcnow()
            if row["status"] == "queued":
                conn.execute(
                    f"UPDATE {RUNS_TABLE} SET status = 'running', updated_at = %s WHERE run_id = %s",
                    (now.isoformat(), run_id),
                )
                row = self._locked_run(conn, run_id)
                self._append_event(conn, row, "run.started", {}, now=now)
                row = self._locked_run(conn, run_id)
            step_id = row["active_step_id"]
            step_sequence = row["active_step_sequence"]
            if step_id is None:
                step_id = str(uuid.uuid4())
                step_sequence = row["next_step_sequence"] + 1
                conn.execute(
                    f"""UPDATE {RUNS_TABLE}
                       SET active_step_id = %s, active_step_sequence = %s,
                           next_step_sequence = %s, updated_at = %s
                       WHERE run_id = %s""",
                    (step_id, step_sequence, step_sequence, now.isoformat(), run_id),
                )
                row = self._locked_run(conn, run_id)
            self._append_event(
                conn,
                row,
                "step.started",
                {
                    "step_id": step_id,
                    "step_sequence": step_sequence,
                    "step_type": "system",
                    "name": "graph_execution",
                    "recovered": recovered,
                },
                now=now,
            )
            return True

        return await self._write(operation)

    async def record_result(
        self,
        run_id: str,
        checkpoint_ref: str,
        state_hash: str,
        interrupt_data: dict[str, Any] | None,
        envelope: dict[str, Any] | None = None,
    ) -> None:
        def operation(conn: psycopg.Connection) -> None:
            row = self._locked_run(conn, run_id)
            if row["status"] == "cancelled":
                return
            if row["status"] != "running":
                raise RuntimeStoreError("RUN_INVALID_STATE", f"cannot complete run from {row['status']}")
            if row["active_step_id"] is None or row["active_step_sequence"] is None:
                raise RuntimeStoreError("RUN_INVALID_STATE", "run has no active execution step")
            now = _utcnow()
            version = row["checkpoint_version"] + 1
            conn.execute(
                f"UPDATE {RUNS_TABLE} SET checkpoint_version = %s, updated_at = %s WHERE run_id = %s",
                (version, now.isoformat(), run_id),
            )
            row = self._locked_run(conn, run_id)
            self._append_event(
                conn,
                row,
                "checkpoint.saved",
                {"backend": "postgres", "checkpoint_ref": checkpoint_ref, "state_hash": state_hash},
                checkpoint_version=version,
                now=now,
            )
            row = self._locked_run(conn, run_id)
            self._append_event(
                conn,
                row,
                "step.completed",
                {
                    "step_id": row["active_step_id"],
                    "step_sequence": row["active_step_sequence"],
                    "step_type": "system",
                    "name": "graph_execution",
                },
                checkpoint_version=version,
                now=now,
            )
            row = self._locked_run(conn, run_id)
            conn.execute(
                f"""UPDATE {RUNS_TABLE}
                   SET active_step_id = NULL, active_step_sequence = NULL, updated_at = %s
                   WHERE run_id = %s""",
                (now.isoformat(), run_id),
            )
            row = self._locked_run(conn, run_id)

            if interrupt_data is not None:
                interrupt_id = interrupt_data["interrupt_id"]
                kind = interrupt_data["kind"]
                wait_status = "waiting_external" if kind == "external" else "waiting_human"
                resume_schema = interrupt_data["resume_schema"]
                conn.execute(
                    f"""INSERT INTO {INTERRUPTS_TABLE}
                       (interrupt_id, run_id, checkpoint_version, kind, status,
                        resume_schema_json, created_at, updated_at)
                       VALUES (%s,%s,%s,%s,%s,%s,%s,%s)
                       ON CONFLICT (interrupt_id) DO UPDATE SET
                         checkpoint_version = excluded.checkpoint_version,
                         kind = excluded.kind,
                         status = 'pending',
                         resume_schema_json = excluded.resume_schema_json,
                         updated_at = excluded.updated_at""",
                    (
                        interrupt_id,
                        run_id,
                        version,
                        kind,
                        "pending",
                        _canonical(resume_schema),
                        now.isoformat(),
                        now.isoformat(),
                    ),
                )
                conn.execute(
                    f"""UPDATE {RUNS_TABLE}
                       SET status = %s, active_interrupt_id = %s, updated_at = %s
                       WHERE run_id = %s""",
                    (wait_status, interrupt_id, now.isoformat(), run_id),
                )
                row = self._locked_run(conn, run_id)
                self._append_event(
                    conn,
                    row,
                    "run.interrupted",
                    {
                        "interrupt_id": interrupt_id,
                        "kind": kind,
                        "wait_status": wait_status,
                        "resume_schema": resume_schema,
                    },
                    checkpoint_version=version,
                    now=now,
                )
            else:
                conn.execute(
                    f"UPDATE {RUNS_TABLE} SET status = 'succeeded', updated_at = %s WHERE run_id = %s",
                    (now.isoformat(), run_id),
                )
                row = self._locked_run(conn, run_id)
                # run.succeeded 事件携带最终输出(M2-A):output/usage 与 V1
                # envelope 同形,Go 应用事件时落 agent_runs.output_summary_json。
                success_payload: dict[str, Any] = {}
                if envelope is not None:
                    success_payload = {
                        "output": envelope.get("output") or {},
                        "usage": envelope.get("usage") or {},
                    }
                self._append_event(
                    conn, row, "run.succeeded", success_payload,
                    checkpoint_version=version, now=now
                )
            conn.execute(
                f"""UPDATE {COMMANDS_TABLE} SET status = 'completed', updated_at = %s
                   WHERE run_id = %s AND operation = 'resume' AND status = 'accepted'""",
                (now.isoformat(), run_id),
            )

        await self._write(operation)

    async def record_failure(self, run_id: str, code: str, message: str) -> None:
        def operation(conn: psycopg.Connection) -> None:
            row = self._locked_run(conn, run_id)
            if row["status"] in TERMINAL_STATUSES:
                return
            now = _utcnow()
            if row["active_step_id"] is not None:
                self._append_event(
                    conn,
                    row,
                    "step.failed",
                    {
                        "step_id": row["active_step_id"],
                        "step_sequence": row["active_step_sequence"],
                        "step_type": "system",
                        "name": "graph_execution",
                        "status": "failed",
                        "error": {"code": code},
                    },
                    checkpoint_version=row["checkpoint_version"] or None,
                    now=now,
                )
            conn.execute(
                f"""UPDATE {RUNS_TABLE} SET status = 'failed', active_step_id = NULL,
                   active_step_sequence = NULL, updated_at = %s WHERE run_id = %s""",
                (now.isoformat(), run_id),
            )
            row = self._locked_run(conn, run_id)
            self._append_event(
                conn,
                row,
                "run.failed",
                {"error": {"code": code, "message": message[:255]}},
                checkpoint_version=row["checkpoint_version"] or None,
                now=now,
            )
            conn.execute(
                f"""UPDATE {COMMANDS_TABLE} SET status = 'failed', updated_at = %s
                   WHERE run_id = %s AND status = 'accepted'""",
                (now.isoformat(), run_id),
            )

        await self._write(operation)

    async def get_interrupt(self, run_id: str, interrupt_id: str) -> dict[str, Any]:
        def operation(conn: psycopg.Connection) -> dict[str, Any]:
            row = conn.execute(
                f"""SELECT * FROM {INTERRUPTS_TABLE}
                   WHERE run_id = %s AND interrupt_id = %s""",
                (run_id, interrupt_id),
            ).fetchone()
            if row is None:
                raise RuntimeStoreError("INTERRUPT_NOT_FOUND", "interrupt was not found", 404)
            value = dict(row)
            value["resume_schema"] = json.loads(value.pop("resume_schema_json"))
            return value

        return await self._read(operation)

    async def accept_resume(self, request: ResumeRunRequest) -> AcceptedRunResponse:
        request_data = request.model_dump(mode="json")
        request_hash = _hash(request_data)

        def operation(conn: psycopg.Connection) -> AcceptedRunResponse:
            replay = self._command_replay(
                conn, request.run_id, "resume", request.idempotency_key, request_hash
            )
            if replay is not None:
                replay["replayed"] = True
                return AcceptedRunResponse.model_validate(replay)

            row = self._locked_run(conn, request.run_id)
            if row["status"] not in {"waiting_human", "waiting_external"}:
                raise RuntimeStoreError("RUN_INVALID_STATE", f"run is {row['status']}, not waiting")
            if row["active_interrupt_id"] != request.interrupt_id:
                raise RuntimeStoreError("INTERRUPT_CONFLICT", "interrupt is not active for this run")
            if row["checkpoint_version"] != request.expected_checkpoint_version:
                raise RuntimeStoreError(
                    "CHECKPOINT_VERSION_CONFLICT",
                    f"expected checkpoint {request.expected_checkpoint_version}, current is {row['checkpoint_version']}",
                )
            interrupt = conn.execute(
                f"SELECT status FROM {INTERRUPTS_TABLE} WHERE interrupt_id = %s AND run_id = %s",
                (request.interrupt_id, request.run_id),
            ).fetchone()
            if interrupt is None or interrupt["status"] != "pending":
                raise RuntimeStoreError("INTERRUPT_CONFLICT", "interrupt was already consumed")

            now = _utcnow()
            response = AcceptedRunResponse(
                run_id=request.run_id,
                status="queued",
                accepted_at=now,
                checkpoint_version=row["checkpoint_version"],
            )
            conn.execute(
                f"""INSERT INTO {COMMANDS_TABLE}
                   (run_id, operation, idempotency_key, request_hash, request_json,
                    response_json, status, created_at, updated_at)
                   VALUES (%s,%s,%s,%s,%s,%s,'accepted',%s,%s)""",
                (
                    request.run_id,
                    "resume",
                    request.idempotency_key,
                    request_hash,
                    _canonical(request_data),
                    response.model_dump_json(),
                    now.isoformat(),
                    now.isoformat(),
                ),
            )
            conn.execute(
                f"UPDATE {INTERRUPTS_TABLE} SET status = 'resumed', updated_at = %s WHERE interrupt_id = %s",
                (now.isoformat(), request.interrupt_id),
            )
            conn.execute(
                f"""UPDATE {RUNS_TABLE}
                   SET status = 'running', active_interrupt_id = NULL, updated_at = %s
                   WHERE run_id = %s""",
                (now.isoformat(), request.run_id),
            )
            row = self._locked_run(conn, request.run_id)
            self._append_event(
                conn,
                row,
                "run.resumed",
                {
                    "interrupt_id": request.interrupt_id,
                    "idempotency_key": request.idempotency_key,
                },
                checkpoint_version=row["checkpoint_version"],
                now=now,
            )
            return response

        return await self._write(operation)

    async def get_pending_resume(self, run_id: str) -> dict[str, Any] | None:
        def operation(conn: psycopg.Connection) -> dict[str, Any] | None:
            row = conn.execute(
                f"""SELECT request_json FROM {COMMANDS_TABLE}
                   WHERE run_id = %s AND operation = 'resume' AND status = 'accepted'
                   ORDER BY created_at DESC LIMIT 1""",
                (run_id,),
            ).fetchone()
            return json.loads(row["request_json"]) if row is not None else None

        return await self._read(operation)

    async def cancel(self, request: CancelRunRequest) -> AcceptedRunResponse:
        request_data = request.model_dump(mode="json")
        request_hash = _hash(request_data)

        def operation(conn: psycopg.Connection) -> AcceptedRunResponse:
            replay = self._command_replay(
                conn, request.run_id, "cancel", request.idempotency_key, request_hash
            )
            if replay is not None:
                replay["replayed"] = True
                return AcceptedRunResponse.model_validate(replay)

            row = self._locked_run(conn, request.run_id)
            if row["status"] in {"succeeded", "failed"}:
                raise RuntimeStoreError("RUN_INVALID_STATE", f"terminal run is {row['status']}")
            now = _utcnow()
            response = AcceptedRunResponse(
                run_id=request.run_id,
                status="cancelled",
                accepted_at=now,
                checkpoint_version=row["checkpoint_version"],
            )
            conn.execute(
                f"""INSERT INTO {COMMANDS_TABLE}
                   (run_id, operation, idempotency_key, request_hash, request_json,
                    response_json, status, created_at, updated_at)
                   VALUES (%s,%s,%s,%s,%s,%s,'completed',%s,%s)""",
                (
                    request.run_id,
                    "cancel",
                    request.idempotency_key,
                    request_hash,
                    _canonical(request_data),
                    response.model_dump_json(),
                    now.isoformat(),
                    now.isoformat(),
                ),
            )
            if row["status"] != "cancelled":
                if row["active_step_id"] is not None:
                    self._append_event(
                        conn,
                        row,
                        "step.failed",
                        {
                            "step_id": row["active_step_id"],
                            "step_sequence": row["active_step_sequence"],
                            "step_type": "system",
                            "name": "graph_execution",
                            "status": "cancelled",
                        },
                        checkpoint_version=row["checkpoint_version"] or None,
                        now=now,
                    )
                    row = self._locked_run(conn, request.run_id)
                conn.execute(
                    f"UPDATE {INTERRUPTS_TABLE} SET status = 'cancelled', updated_at = %s WHERE run_id = %s AND status = 'pending'",
                    (now.isoformat(), request.run_id),
                )
                conn.execute(
                    f"""UPDATE {RUNS_TABLE} SET status = 'cancelled', active_interrupt_id = NULL,
                       active_step_id = NULL, active_step_sequence = NULL, updated_at = %s
                       WHERE run_id = %s""",
                    (now.isoformat(), request.run_id),
                )
                row = self._locked_run(conn, request.run_id)
                self._append_event(
                    conn,
                    row,
                    "run.cancelled",
                    {"reason": request.reason},
                    checkpoint_version=row["checkpoint_version"] or None,
                    now=now,
                )
            return response

        return await self._write(operation)

    async def pending_events(self, limit: int = 100) -> list[dict[str, Any]]:
        """返回所有待投递且退避已到期的事件(不保证 run 内顺序,供测试/巡检)。"""
        return await self._read(
            lambda conn: [
                {**dict(row), "body": json.loads(row["body_json"])}
                for row in conn.execute(
                    f"""SELECT * FROM {OUTBOX_TABLE}
                       WHERE status = 'pending'
                         AND (next_attempt_at IS NULL OR next_attempt_at <= %s)
                       ORDER BY delivery_order LIMIT %s""",
                    (_utcnow().isoformat(), limit),
                ).fetchall()
            ]
        )

    async def pending_head_events(self, limit: int = 100) -> list[dict[str, Any]]:
        """返回投递批次:每个 run 只取其最靠前、退避已到期的待投递事件。

        Head-of-line 阻塞:若某 run 的最早未投递事件仍在退避期,该 run 的
        后续事件不会被选出,从而保证跨批次也不会乱序投递。
        """
        return await self._read(
            lambda conn: [
                {**dict(row), "body": json.loads(row["body_json"])}
                for row in conn.execute(
                    f"""SELECT * FROM {OUTBOX_TABLE} e
                       WHERE e.status = 'pending'
                         AND (e.next_attempt_at IS NULL OR e.next_attempt_at <= %s)
                         AND e.delivery_order = (
                             SELECT MIN(head.delivery_order)
                             FROM {OUTBOX_TABLE} head
                             WHERE head.run_id = e.run_id
                               AND head.status = 'pending'
                         )
                       ORDER BY e.delivery_order LIMIT %s""",
                    (_utcnow().isoformat(), limit),
                ).fetchall()
            ]
        )

    async def mark_event_delivered(self, event_id: str) -> None:
        await self._write(
            lambda conn: conn.execute(
                f"""UPDATE {OUTBOX_TABLE}
                   SET status = 'delivered', delivered_at = %s,
                       delivery_attempts = delivery_attempts + 1,
                       last_error = NULL, next_attempt_at = NULL
                   WHERE event_id = %s AND status = 'pending'""",
                (_utcnow().isoformat(), event_id),
            )
        )

    async def mark_event_dead(self, event_id: str, error: str) -> None:
        """终态死信:Go 返回 4xx(永久拒绝)时调用,不再重试。

        Dead-letter 不参与 head-of-line 阻塞,后续事件可继续投递;
        事件永久拒绝本身需要告警排查。
        """
        await self._write(
            lambda conn: conn.execute(
                f"""UPDATE {OUTBOX_TABLE}
                   SET status = 'dead', delivery_attempts = delivery_attempts + 1,
                       last_error = %s, next_attempt_at = NULL, delivered_at = NULL
                   WHERE event_id = %s AND status = 'pending'""",
                (error[:500], event_id),
            )
        )

    async def mark_event_failed(self, event_id: str, error: str) -> None:
        def operation(conn: psycopg.Connection) -> None:
            row = conn.execute(
                f"SELECT delivery_attempts FROM {OUTBOX_TABLE} WHERE event_id = %s",
                (event_id,),
            ).fetchone()
            if row is None:
                return
            delay_seconds = min(60, 2 ** min(row["delivery_attempts"], 6))
            next_attempt = _utcnow() + timedelta(seconds=delay_seconds)
            conn.execute(
                f"""UPDATE {OUTBOX_TABLE}
                   SET delivery_attempts = delivery_attempts + 1, last_error = %s,
                       next_attempt_at = %s
                   WHERE event_id = %s AND status = 'pending'""",
                (error[:500], next_attempt.isoformat(), event_id),
            )

        await self._write(operation)

    @staticmethod
    def _locked_run(conn: psycopg.Connection, run_id: str) -> dict[str, Any]:
        # FOR UPDATE serializes concurrent state transitions on the same run
        # across processes — the PG replacement for SQLite BEGIN IMMEDIATE.
        row = conn.execute(
            f"SELECT * FROM {RUNS_TABLE} WHERE run_id = %s FOR UPDATE", (run_id,)
        ).fetchone()
        if row is None:
            raise RuntimeStoreError("RUN_NOT_FOUND", "durable runtime run was not found", 404)
        return row

    async def enqueue_runtime_event(
        self,
        run_id: str,
        event_type: str,
        payload: dict[str, Any],
    ) -> None:
        """Enqueue an arbitrary runtime event (tool.called / artifact.ready / ...).

        Used by LangGraph tool hooks to report per-step execution details to Go.
        Must be called while the run is active; silently drops if the run is not
        found (to avoid breaking tool execution on event-delivery failures).
        """

        def operation(conn: psycopg.Connection) -> None:
            try:
                row = self._locked_run(conn, run_id)
            except RuntimeStoreError:
                return
            now = _utcnow()
            self._append_event(
                conn,
                row,
                event_type,
                payload,
                checkpoint_version=row["checkpoint_version"] or None,
                now=now,
            )

        await self._write(operation)

    @staticmethod
    def _command_replay(
        conn: psycopg.Connection,
        run_id: str,
        operation: str,
        idempotency_key: str,
        request_hash: str,
    ) -> dict[str, Any] | None:
        row = conn.execute(
            f"""SELECT request_hash, response_json FROM {COMMANDS_TABLE}
               WHERE run_id = %s AND operation = %s AND idempotency_key = %s""",
            (run_id, operation, idempotency_key),
        ).fetchone()
        if row is None:
            return None
        if row["request_hash"] != request_hash:
            raise RuntimeStoreError(
                "IDEMPOTENCY_KEY_CONFLICT",
                "idempotency_key was reused with a different request",
            )
        return json.loads(row["response_json"])

    @staticmethod
    def _append_event(
        conn: psycopg.Connection,
        run: dict[str, Any],
        event_type: str,
        payload: dict[str, Any],
        *,
        checkpoint_version: int | None = None,
        now: datetime | None = None,
    ) -> RuntimeEventEnvelope:
        occurred_at = now or _utcnow()
        sequence = run["next_sequence"] + 1
        event = RuntimeEventEnvelope(
            event_id=str(uuid.uuid4()),
            tenant_id=run["tenant_id"],
            run_id=run["run_id"],
            sequence=sequence,
            attempt=run["attempt"],
            type=event_type,
            occurred_at=occurred_at,
            payload=payload,
            checkpoint_version=checkpoint_version,
        )
        conn.execute(
            f"UPDATE {RUNS_TABLE} SET next_sequence = %s, updated_at = %s WHERE run_id = %s",
            (sequence, occurred_at.isoformat(), run["run_id"]),
        )
        conn.execute(
            f"""INSERT INTO {OUTBOX_TABLE}
               (event_id, run_id, sequence, body_json, created_at)
               VALUES (%s,%s,%s,%s,%s)""",
            (
                event.event_id,
                run["run_id"],
                sequence,
                event.model_dump_json(),
                occurred_at.isoformat(),
            ),
        )
        return event

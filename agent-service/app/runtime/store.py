from __future__ import annotations

import asyncio
import hashlib
import json
import sqlite3
import uuid
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any, Callable

from app.runtime.models import (
    AcceptedRunResponse,
    CancelRunRequest,
    ResumeRunRequest,
    RuntimeEventEnvelope,
    StartRunRequest,
)


TERMINAL_STATUSES = {"succeeded", "failed", "cancelled"}


class RuntimeStoreError(Exception):
    def __init__(self, code: str, message: str, status_code: int = 409) -> None:
        super().__init__(message)
        self.code = code
        self.message = message
        self.status_code = status_code


def _utcnow() -> datetime:
    return datetime.now(UTC)


def _canonical(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def _hash(value: Any) -> str:
    return hashlib.sha256(_canonical(value).encode("utf-8")).hexdigest()


class RuntimeStore:
    """Runtime metadata and an at-least-once event outbox beside LangGraph SQLite."""

    def __init__(self, db_path: str) -> None:
        self.db_path = db_path
        self._lock = asyncio.Lock()

    def _connect(self) -> sqlite3.Connection:
        conn = sqlite3.connect(self.db_path, timeout=30)
        conn.row_factory = sqlite3.Row
        conn.execute("PRAGMA foreign_keys = ON")
        conn.execute("PRAGMA busy_timeout = 30000")
        return conn

    async def setup(self) -> None:
        Path(self.db_path).parent.mkdir(parents=True, exist_ok=True)
        async with self._lock:
            await asyncio.to_thread(self._setup_sync)

    def _setup_sync(self) -> None:
        with self._connect() as conn:
            conn.execute("PRAGMA journal_mode = WAL")
            conn.executescript(
                """
                CREATE TABLE IF NOT EXISTS runtime_v2_runs (
                    run_id TEXT PRIMARY KEY,
                    request_hash TEXT NOT NULL,
                    request_json TEXT NOT NULL,
                    thread_id TEXT NOT NULL,
                    tenant_id TEXT NOT NULL,
                    graph_key TEXT NOT NULL,
                    graph_version TEXT NOT NULL,
                    attempt INTEGER NOT NULL CHECK (attempt > 0),
                    status TEXT NOT NULL,
                    checkpoint_version INTEGER NOT NULL DEFAULT 0 CHECK (checkpoint_version >= 0),
                    next_sequence INTEGER NOT NULL DEFAULT 0 CHECK (next_sequence >= 0),
                    next_step_sequence INTEGER NOT NULL DEFAULT 0 CHECK (next_step_sequence >= 0),
                    active_step_id TEXT,
                    active_step_sequence INTEGER,
                    active_interrupt_id TEXT,
                    accepted_at TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                );

                CREATE TABLE IF NOT EXISTS runtime_v2_interrupts (
                    interrupt_id TEXT PRIMARY KEY,
                    run_id TEXT NOT NULL REFERENCES runtime_v2_runs(run_id),
                    checkpoint_version INTEGER NOT NULL CHECK (checkpoint_version > 0),
                    kind TEXT NOT NULL,
                    status TEXT NOT NULL,
                    resume_schema_json TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                );

                CREATE TABLE IF NOT EXISTS runtime_v2_commands (
                    run_id TEXT NOT NULL REFERENCES runtime_v2_runs(run_id),
                    operation TEXT NOT NULL,
                    idempotency_key TEXT NOT NULL,
                    request_hash TEXT NOT NULL,
                    request_json TEXT NOT NULL,
                    response_json TEXT NOT NULL,
                    status TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    updated_at TEXT NOT NULL,
                    PRIMARY KEY (run_id, operation, idempotency_key)
                );

                CREATE UNIQUE INDEX IF NOT EXISTS idx_runtime_v2_one_active_resume
                ON runtime_v2_commands (run_id)
                WHERE operation = 'resume' AND status = 'accepted';

                CREATE TABLE IF NOT EXISTS runtime_v2_event_outbox (
                    delivery_order INTEGER PRIMARY KEY AUTOINCREMENT,
                    event_id TEXT NOT NULL UNIQUE,
                    run_id TEXT NOT NULL REFERENCES runtime_v2_runs(run_id),
                    sequence INTEGER NOT NULL,
                    body_json TEXT NOT NULL,
                    status TEXT NOT NULL DEFAULT 'pending',
                    delivery_attempts INTEGER NOT NULL DEFAULT 0,
                    last_error TEXT,
                    next_attempt_at TEXT,
                    delivered_at TEXT,
                    created_at TEXT NOT NULL,
                    UNIQUE (run_id, sequence)
                );
                """
            )
            columns = {
                row[1]
                for row in conn.execute("PRAGMA table_info(runtime_v2_event_outbox)")
            }
            if "next_attempt_at" not in columns:
                conn.execute(
                    "ALTER TABLE runtime_v2_event_outbox ADD COLUMN next_attempt_at TEXT"
                )
            if "status" not in columns:
                conn.execute(
                    "ALTER TABLE runtime_v2_event_outbox "
                    "ADD COLUMN status TEXT NOT NULL DEFAULT 'pending'"
                )
            # 升级回填:历史上以 delivered_at 表达送达的事件,标记为 delivered。
            conn.execute(
                """UPDATE runtime_v2_event_outbox
                   SET status = 'delivered'
                   WHERE status = 'pending' AND delivered_at IS NOT NULL"""
            )
            run_columns = {
                row[1] for row in conn.execute("PRAGMA table_info(runtime_v2_runs)")
            }
            for column, definition in (
                ("next_step_sequence", "INTEGER NOT NULL DEFAULT 0"),
                ("active_step_id", "TEXT"),
                ("active_step_sequence", "INTEGER"),
            ):
                if column not in run_columns:
                    conn.execute(
                        f"ALTER TABLE runtime_v2_runs ADD COLUMN {column} {definition}"
                    )

    async def _write(self, operation: Callable[[sqlite3.Connection], Any]) -> Any:
        async with self._lock:
            return await asyncio.to_thread(self._write_sync, operation)

    def _write_sync(self, operation: Callable[[sqlite3.Connection], Any]) -> Any:
        with self._connect() as conn:
            conn.execute("BEGIN IMMEDIATE")
            return operation(conn)

    async def _read(self, operation: Callable[[sqlite3.Connection], Any]) -> Any:
        return await asyncio.to_thread(self._read_sync, operation)

    def _read_sync(self, operation: Callable[[sqlite3.Connection], Any]) -> Any:
        with self._connect() as conn:
            return operation(conn)

    async def create_run(self, request: StartRunRequest) -> tuple[AcceptedRunResponse, bool]:
        request_data = request.model_dump(mode="json")
        request_json = _canonical(request_data)
        request_hash = _hash(request_data)

        def operation(conn: sqlite3.Connection) -> tuple[AcceptedRunResponse, bool]:
            existing = conn.execute(
                "SELECT * FROM runtime_v2_runs WHERE run_id = ?", (request.run_id,)
            ).fetchone()
            if existing is not None:
                if existing["request_hash"] != request_hash:
                    raise RuntimeStoreError(
                        "RUN_IDEMPOTENCY_CONFLICT",
                        "run_id already exists with a different Start request",
                    )
                return self._accepted(existing, replayed=True), False

            now = _utcnow().isoformat()
            conn.execute(
                """INSERT INTO runtime_v2_runs
                   (run_id, request_hash, request_json, thread_id, tenant_id,
                    graph_key, graph_version, attempt, status, accepted_at, updated_at)
                   VALUES (?,?,?,?,?,?,?,?,?,?,?)""",
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
            )
            row = conn.execute(
                "SELECT * FROM runtime_v2_runs WHERE run_id = ?", (request.run_id,)
            ).fetchone()
            return self._accepted(row, replayed=False), True

        return await self._write(operation)

    @staticmethod
    def _accepted(row: sqlite3.Row, replayed: bool) -> AcceptedRunResponse:
        return AcceptedRunResponse(
            run_id=row["run_id"],
            status=row["status"],
            accepted_at=datetime.fromisoformat(row["accepted_at"]),
            checkpoint_version=row["checkpoint_version"],
            replayed=replayed,
        )

    async def get_run(self, run_id: str) -> dict[str, Any]:
        def operation(conn: sqlite3.Connection) -> dict[str, Any]:
            row = conn.execute(
                "SELECT * FROM runtime_v2_runs WHERE run_id = ?", (run_id,)
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
                row[0]
                for row in conn.execute(
                    "SELECT run_id FROM runtime_v2_runs WHERE status IN ('queued','running')"
                ).fetchall()
            ]
        )

    async def begin_execution(self, run_id: str, recovered: bool) -> bool:
        def operation(conn: sqlite3.Connection) -> bool:
            row = self._locked_run(conn, run_id)
            if row["status"] in TERMINAL_STATUSES or row["status"].startswith("waiting_"):
                return False
            now = _utcnow()
            if row["status"] == "queued":
                conn.execute(
                    "UPDATE runtime_v2_runs SET status = 'running', updated_at = ? WHERE run_id = ?",
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
                    """UPDATE runtime_v2_runs
                       SET active_step_id = ?, active_step_sequence = ?,
                           next_step_sequence = ?, updated_at = ?
                       WHERE run_id = ?""",
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
        def operation(conn: sqlite3.Connection) -> None:
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
                "UPDATE runtime_v2_runs SET checkpoint_version = ?, updated_at = ? WHERE run_id = ?",
                (version, now.isoformat(), run_id),
            )
            row = self._locked_run(conn, run_id)
            self._append_event(
                conn,
                row,
                "checkpoint.saved",
                {"backend": "sqlite", "checkpoint_ref": checkpoint_ref, "state_hash": state_hash},
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
                """UPDATE runtime_v2_runs
                   SET active_step_id = NULL, active_step_sequence = NULL, updated_at = ?
                   WHERE run_id = ?""",
                (now.isoformat(), run_id),
            )
            row = self._locked_run(conn, run_id)

            if interrupt_data is not None:
                interrupt_id = interrupt_data["interrupt_id"]
                kind = interrupt_data["kind"]
                wait_status = "waiting_external" if kind == "external" else "waiting_human"
                resume_schema = interrupt_data["resume_schema"]
                conn.execute(
                    """INSERT INTO runtime_v2_interrupts
                       (interrupt_id, run_id, checkpoint_version, kind, status,
                        resume_schema_json, created_at, updated_at)
                       VALUES (?,?,?,?,?,?,?,?)
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
                    """UPDATE runtime_v2_runs
                       SET status = ?, active_interrupt_id = ?, updated_at = ?
                       WHERE run_id = ?""",
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
                    "UPDATE runtime_v2_runs SET status = 'succeeded', updated_at = ? WHERE run_id = ?",
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
                """UPDATE runtime_v2_commands SET status = 'completed', updated_at = ?
                   WHERE run_id = ? AND operation = 'resume' AND status = 'accepted'""",
                (now.isoformat(), run_id),
            )

        await self._write(operation)

    async def record_failure(self, run_id: str, code: str, message: str) -> None:
        def operation(conn: sqlite3.Connection) -> None:
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
                """UPDATE runtime_v2_runs SET status = 'failed', active_step_id = NULL,
                   active_step_sequence = NULL, updated_at = ? WHERE run_id = ?""",
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
                """UPDATE runtime_v2_commands SET status = 'failed', updated_at = ?
                   WHERE run_id = ? AND status = 'accepted'""",
                (now.isoformat(), run_id),
            )

        await self._write(operation)

    async def get_interrupt(self, run_id: str, interrupt_id: str) -> dict[str, Any]:
        def operation(conn: sqlite3.Connection) -> dict[str, Any]:
            row = conn.execute(
                """SELECT * FROM runtime_v2_interrupts
                   WHERE run_id = ? AND interrupt_id = ?""",
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

        def operation(conn: sqlite3.Connection) -> AcceptedRunResponse:
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
                "SELECT status FROM runtime_v2_interrupts WHERE interrupt_id = ? AND run_id = ?",
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
                """INSERT INTO runtime_v2_commands
                   (run_id, operation, idempotency_key, request_hash, request_json,
                    response_json, status, created_at, updated_at)
                   VALUES (?,?,?,?,?,?, 'accepted', ?,?)""",
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
                """UPDATE runtime_v2_interrupts SET status = 'resumed', updated_at = ?
                   WHERE interrupt_id = ?""",
                (now.isoformat(), request.interrupt_id),
            )
            conn.execute(
                """UPDATE runtime_v2_runs
                   SET status = 'running', active_interrupt_id = NULL, updated_at = ?
                   WHERE run_id = ?""",
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
        def operation(conn: sqlite3.Connection) -> dict[str, Any] | None:
            row = conn.execute(
                """SELECT request_json FROM runtime_v2_commands
                   WHERE run_id = ? AND operation = 'resume' AND status = 'accepted'
                   ORDER BY created_at DESC LIMIT 1""",
                (run_id,),
            ).fetchone()
            return json.loads(row["request_json"]) if row is not None else None

        return await self._read(operation)

    async def cancel(self, request: CancelRunRequest) -> AcceptedRunResponse:
        request_data = request.model_dump(mode="json")
        request_hash = _hash(request_data)

        def operation(conn: sqlite3.Connection) -> AcceptedRunResponse:
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
                """INSERT INTO runtime_v2_commands
                   (run_id, operation, idempotency_key, request_hash, request_json,
                    response_json, status, created_at, updated_at)
                   VALUES (?,?,?,?,?,?,'completed',?,?)""",
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
                    "UPDATE runtime_v2_interrupts SET status = 'cancelled', updated_at = ? WHERE run_id = ? AND status = 'pending'",
                    (now.isoformat(), request.run_id),
                )
                conn.execute(
                    """UPDATE runtime_v2_runs SET status = 'cancelled', active_interrupt_id = NULL,
                       active_step_id = NULL, active_step_sequence = NULL, updated_at = ?
                       WHERE run_id = ?""",
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
                    """SELECT * FROM runtime_v2_event_outbox
                       WHERE status = 'pending'
                         AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
                       ORDER BY delivery_order LIMIT ?""",
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
                    """SELECT * FROM runtime_v2_event_outbox e
                       WHERE e.status = 'pending'
                         AND (e.next_attempt_at IS NULL OR e.next_attempt_at <= ?)
                         AND e.delivery_order = (
                             SELECT MIN(head.delivery_order)
                             FROM runtime_v2_event_outbox head
                             WHERE head.run_id = e.run_id
                               AND head.status = 'pending'
                         )
                       ORDER BY e.delivery_order LIMIT ?""",
                    (_utcnow().isoformat(), limit),
                ).fetchall()
            ]
        )

    async def mark_event_delivered(self, event_id: str) -> None:
        await self._write(
            lambda conn: conn.execute(
                """UPDATE runtime_v2_event_outbox
                   SET status = 'delivered', delivered_at = ?,
                       delivery_attempts = delivery_attempts + 1,
                       last_error = NULL, next_attempt_at = NULL
                   WHERE event_id = ? AND status = 'pending'""",
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
                """UPDATE runtime_v2_event_outbox
                   SET status = 'dead', delivery_attempts = delivery_attempts + 1,
                       last_error = ?, next_attempt_at = NULL, delivered_at = NULL
                   WHERE event_id = ? AND status = 'pending'""",
                (error[:500], event_id),
            )
        )

    async def mark_event_failed(self, event_id: str, error: str) -> None:
        def operation(conn: sqlite3.Connection) -> None:
            row = conn.execute(
                "SELECT delivery_attempts FROM runtime_v2_event_outbox WHERE event_id = ?",
                (event_id,),
            ).fetchone()
            if row is None:
                return
            delay_seconds = min(60, 2 ** min(row["delivery_attempts"], 6))
            next_attempt = _utcnow() + timedelta(seconds=delay_seconds)
            conn.execute(
                """UPDATE runtime_v2_event_outbox
                   SET delivery_attempts = delivery_attempts + 1, last_error = ?,
                       next_attempt_at = ?
                   WHERE event_id = ? AND status = 'pending'""",
                (error[:500], next_attempt.isoformat(), event_id),
            )

        await self._write(operation)

    @staticmethod
    def _locked_run(conn: sqlite3.Connection, run_id: str) -> sqlite3.Row:
        row = conn.execute("SELECT * FROM runtime_v2_runs WHERE run_id = ?", (run_id,)).fetchone()
        if row is None:
            raise RuntimeStoreError("RUN_NOT_FOUND", "durable runtime run was not found", 404)
        return row

    @staticmethod
    def _command_replay(
        conn: sqlite3.Connection,
        run_id: str,
        operation: str,
        idempotency_key: str,
        request_hash: str,
    ) -> dict[str, Any] | None:
        row = conn.execute(
            """SELECT request_hash, response_json FROM runtime_v2_commands
               WHERE run_id = ? AND operation = ? AND idempotency_key = ?""",
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
        conn: sqlite3.Connection,
        run: sqlite3.Row,
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
            "UPDATE runtime_v2_runs SET next_sequence = ?, updated_at = ? WHERE run_id = ?",
            (sequence, occurred_at.isoformat(), run["run_id"]),
        )
        conn.execute(
            """INSERT INTO runtime_v2_event_outbox
               (event_id, run_id, sequence, body_json, created_at)
               VALUES (?,?,?,?,?)""",
            (
                event.event_id,
                run["run_id"],
                sequence,
                event.model_dump_json(),
                occurred_at.isoformat(),
            ),
        )
        return event

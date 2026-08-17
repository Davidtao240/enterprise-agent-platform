from __future__ import annotations

import json
import tempfile
import unittest
import uuid
from datetime import timedelta

import httpx

from app.runtime.events import RuntimeEventDispatcher
from app.runtime.store import RuntimeStore, _utcnow


async def _seed_run(store: RuntimeStore, run_id: str, tenant_id: str = "tenant-a") -> None:
    """直接插入一条 running 的 run 行,绕过 Graph 执行。"""
    now = _utcnow().isoformat()
    await store._write(
        lambda conn: conn.execute(
            """INSERT INTO runtime_v2_runs
               (run_id, request_hash, request_json, thread_id, tenant_id,
                graph_key, graph_version, attempt, status, accepted_at, updated_at)
               VALUES (?,?,?,?,?,?,?,?,'running',?,?)""",
            (
                run_id,
                f"hash-{run_id}",
                "{}",
                f"thread-{run_id}",
                tenant_id,
                "finance_operating_report_graph",
                "1.0.0",
                1,
                now,
                now,
            ),
        )
    )


async def _seed_event(
    store: RuntimeStore,
    run_id: str,
    sequence: int,
    event_type: str = "run.started",
    next_attempt_at: str | None = None,
) -> str:
    """直接插入一条 pending 事件到 outbox,返回 event_id。"""
    event_id = str(uuid.uuid4())
    body = {
        "protocol_version": "2.0",
        "event_id": event_id,
        "tenant_id": "tenant-a",
        "run_id": run_id,
        "sequence": sequence,
        "attempt": 1,
        "type": event_type,
        "occurred_at": _utcnow().isoformat(),
        "payload": {},
    }
    await store._write(
        lambda conn: conn.execute(
            """INSERT INTO runtime_v2_event_outbox
               (event_id, run_id, sequence, body_json, next_attempt_at, created_at)
               VALUES (?,?,?,?,?,?)""",
            (
                event_id,
                run_id,
                sequence,
                json.dumps(body),
                next_attempt_at,
                _utcnow().isoformat(),
            ),
        )
    )
    return event_id


async def _outbox_rows(store: RuntimeStore, run_id: str) -> list[dict]:
    def read(conn):
        return [
            dict(row)
            for row in conn.execute(
                "SELECT * FROM runtime_v2_event_outbox WHERE run_id = ? ORDER BY sequence",
                (run_id,),
            ).fetchall()
        ]

    return await store._read(read)


class RuntimeStoreOutboxOrderingTest(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.store = RuntimeStore(f"{self.tmp.name}/runtime.sqlite3")
        await self.store.setup()

    async def asyncTearDown(self) -> None:
        self.tmp.cleanup()

    async def test_head_of_line_blocks_successors_during_backoff(self) -> None:
        run_id = str(uuid.uuid4())
        await _seed_run(self.store, run_id)
        await _seed_event(
            self.store, run_id, 1,
            next_attempt_at=(_utcnow() + timedelta(hours=1)).isoformat(),
        )
        await _seed_event(self.store, run_id, 2)

        heads = await self.store.pending_head_events()
        self.assertEqual([], heads, "backed-off head must block its successor")

        # 退避到期后,同一轮只选出 head,后继仍需等 head 送达。
        await self.store._write(
            lambda conn: conn.execute(
                "UPDATE runtime_v2_event_outbox SET next_attempt_at = NULL WHERE sequence = 1",
            )
        )
        heads = await self.store.pending_head_events()
        self.assertEqual([1], [event["sequence"] for event in heads])

    async def test_pending_events_returns_all_due_events_for_inspection(self) -> None:
        run_id = str(uuid.uuid4())
        await _seed_run(self.store, run_id)
        await _seed_event(self.store, run_id, 1)
        await _seed_event(self.store, run_id, 2)

        events = await self.store.pending_events()
        self.assertEqual([1, 2], [event["sequence"] for event in events])

    async def test_dead_letter_excludes_event_from_future_batches(self) -> None:
        run_id = str(uuid.uuid4())
        await _seed_run(self.store, run_id)
        event_id = await _seed_event(self.store, run_id, 1)

        await self.store.mark_event_dead(event_id, "HTTP 409: RUN_INVALID_STATE")
        rows = await _outbox_rows(self.store, run_id)
        self.assertEqual("dead", rows[0]["status"])
        self.assertEqual([], await self.store.pending_events())
        self.assertEqual([], await self.store.pending_head_events())


class RuntimeEventDispatcherTest(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.store = RuntimeStore(f"{self.tmp.name}/runtime.sqlite3")
        await self.store.setup()
        self.run_id = str(uuid.uuid4())
        await _seed_run(self.store, self.run_id)

    async def asyncTearDown(self) -> None:
        self.tmp.cleanup()

    def _dispatcher(self, status_codes: list[int]) -> RuntimeEventDispatcher:
        calls: list[int] = []

        def handler(request: httpx.Request) -> httpx.Response:
            status_code = status_codes[min(len(calls), len(status_codes) - 1)]
            calls.append(status_code)
            return httpx.Response(
                status_code,
                json={"code": "STALE_RUN_ATTEMPT", "message": "conflict"}
                if status_code == 409
                else {},
            )

        client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
        return RuntimeEventDispatcher(
            self.store,
            "http://go-backend:8080/internal/v2/runtime-events",
            "service-token",
            client=client,
        )

    async def test_4xx_events_are_dead_lettered_in_order(self) -> None:
        await _seed_event(self.store, self.run_id, 1, event_type="run.started")
        await _seed_event(self.store, self.run_id, 2, event_type="step.started")
        dispatcher = self._dispatcher([404, 404])

        # head-of-line:每轮只处理最靠前事件,4xx 死信后下一轮继续后继。
        delivered = await dispatcher.flush_once()
        self.assertEqual(0, delivered)
        rows = await _outbox_rows(self.store, self.run_id)
        self.assertEqual(["dead", "pending"], [row["status"] for row in rows])

        delivered = await dispatcher.flush_once()
        self.assertEqual(0, delivered)
        rows = await _outbox_rows(self.store, self.run_id)
        self.assertEqual(["dead", "dead"], [row["status"] for row in rows])
        self.assertEqual([], await self.store.pending_head_events())

    async def test_5xx_retries_with_head_of_line_ordering(self) -> None:
        seq1 = await _seed_event(self.store, self.run_id, 1, event_type="run.started")
        seq2 = await _seed_event(self.store, self.run_id, 2, event_type="step.started")
        dispatcher = self._dispatcher([500, 200, 200])

        self.assertEqual(0, await dispatcher.flush_once())
        rows = await _outbox_rows(self.store, self.run_id)
        by_event = {row["event_id"]: row for row in rows}
        self.assertEqual("pending", by_event[seq1]["status"])
        self.assertIsNotNone(by_event[seq1]["next_attempt_at"])
        # 退避期:head 阻塞后继,本轮不投出任何事件。
        self.assertEqual([], await self.store.pending_head_events())

        # 退避到期(模拟)后,head 先送达,后继下一轮送达。
        await self.store._write(
            lambda conn: conn.execute(
                "UPDATE runtime_v2_event_outbox SET next_attempt_at = NULL WHERE event_id = ?",
                (seq1,),
            )
        )
        self.assertEqual(1, await dispatcher.flush_once())
        self.assertEqual(1, await dispatcher.flush_once())

        rows = await _outbox_rows(self.store, self.run_id)
        self.assertEqual(["delivered", "delivered"], [row["status"] for row in rows])

    async def test_2xx_marks_delivered(self) -> None:
        await _seed_event(self.store, self.run_id, 1, event_type="run.started")
        dispatcher = self._dispatcher([200])

        self.assertEqual(1, await dispatcher.flush_once())
        rows = await _outbox_rows(self.store, self.run_id)
        self.assertEqual("delivered", rows[0]["status"])


if __name__ == "__main__":
    unittest.main()

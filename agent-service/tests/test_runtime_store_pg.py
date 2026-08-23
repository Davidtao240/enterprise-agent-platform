"""ADR-009: PostgreSQL RuntimeStore integration tests.

Skipped unless RUNTIME_DATABASE_URL (or TEST_DATABASE_URL) points at a live
PostgreSQL — same convention as the Go *_integration_test.go suite. Each test
drops and recreates schema ``agent_runtime`` for full isolation.

Run:
    RUNTIME_DATABASE_URL=postgresql://platform:platform_dev@localhost:5432/enterprise_agent_platform \
        python -m unittest tests.test_runtime_store_pg
"""

from __future__ import annotations

import os
import unittest
import uuid

from app.runtime.models import CancelRunRequest, ResumeRunRequest, StartRunRequest
from app.runtime.store import RuntimeStoreError

_DSN = os.getenv("RUNTIME_DATABASE_URL", "") or os.getenv("TEST_DATABASE_URL", "")


def _start_request(run_id: str) -> StartRunRequest:
    return StartRunRequest.model_validate(
        {
            "protocol_version": "2.0",
            "run_id": run_id,
            "thread_id": str(uuid.uuid4()),
            "trace_id": str(uuid.uuid4()),
            "workflow_instance_id": str(uuid.uuid4()),
            "node_instance_id": str(uuid.uuid4()),
            "business_app_code": "test",
            "graph": {"key": "pg_store_graph", "version": "1.0.0"},
            "configuration": {
                "agent_definition_version": "1.0.0",
                "profile_or_skill_version": "test@1.0.0",
                "model_config_version": "test@1.0.0",
            },
            "input": {},
            "trusted_context": {
                "user_id": str(uuid.uuid4()),
                "tenant_id": str(uuid.uuid4()),
            },
            "budget": {"max_steps": 30, "max_cost": 1.0},
            "attempt": 1,
        }
    )


@unittest.skipUnless(_DSN, "PostgreSQL runtime store test requires RUNTIME_DATABASE_URL")
class PostgresRuntimeStoreTest(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self) -> None:
        import psycopg

        from app.runtime.store_pg import SCHEMA, PostgresRuntimeStore

        with psycopg.connect(_DSN, autocommit=True) as conn:
            conn.execute(f"DROP SCHEMA IF EXISTS {SCHEMA} CASCADE")
        self.store = PostgresRuntimeStore(_DSN)
        await self.store.setup()

    async def asyncTearDown(self) -> None:
        # 关闭连接池,避免后台连接跨测试泄漏。
        if getattr(self, "store", None) is not None:
            await self.store.close()

    async def test_run_lifecycle_outbox_and_idempotency(self) -> None:
        run_id = str(uuid.uuid4())
        request = _start_request(run_id)

        accepted, created = await self.store.create_run(request)
        self.assertTrue(created)
        self.assertEqual("queued", accepted.status)

        # Start replay is idempotent.
        replay, created_again = await self.store.create_run(request)
        self.assertFalse(created_again)
        self.assertTrue(replay.replayed)

        # Conflicting Start on the same run_id is rejected.
        changed = request.model_copy(update={"input": {"changed": True}})
        with self.assertRaisesRegex(RuntimeStoreError, "different Start request"):
            await self.store.create_run(changed)

        self.assertTrue(await self.store.begin_execution(run_id, recovered=False))
        await self.store.record_result(
            run_id,
            checkpoint_ref="langgraph-postgres:test:cp-1",
            state_hash="hash-1",
            interrupt_data=None,
            envelope={"output": {"answer": 42}, "usage": {"tokens": 7}},
        )

        run = await self.store.get_run(run_id)
        self.assertEqual("succeeded", run["status"])
        self.assertEqual(1, run["checkpoint_version"])

        events = await self.store.pending_events()
        run_events = [event["body"] for event in events if event["run_id"] == run_id]
        self.assertEqual(
            ["run.started", "step.started", "checkpoint.saved", "step.completed", "run.succeeded"],
            [event["type"] for event in run_events],
        )
        checkpoint_events = [e for e in run_events if e["type"] == "checkpoint.saved"]
        self.assertEqual("postgres", checkpoint_events[0]["payload"]["backend"])
        succeeded = [e for e in run_events if e["type"] == "run.succeeded"]
        self.assertEqual({"answer": 42}, succeeded[0]["payload"]["output"])

        # Head-of-line delivery: exactly one event per batch per run.
        head = await self.store.pending_head_events(limit=10)
        self.assertEqual(1, len([e for e in head if e["run_id"] == run_id]))
        await self.store.mark_event_delivered(head[0]["event_id"])

        remaining = await self.store.pending_head_events(limit=10)
        next_for_run = [e for e in remaining if e["run_id"] == run_id]
        self.assertEqual(1, len(next_for_run))
        self.assertEqual(2, next_for_run[0]["sequence"])

    async def test_interrupt_resume_and_cancel_flows(self) -> None:
        run_id = str(uuid.uuid4())
        await self.store.create_run(_start_request(run_id))
        await self.store.begin_execution(run_id, recovered=False)

        interrupt_id = str(uuid.uuid4())
        await self.store.record_result(
            run_id,
            checkpoint_ref="langgraph-postgres:test:cp-2",
            state_hash="hash-2",
            interrupt_data={
                "interrupt_id": interrupt_id,
                "kind": "human",
                "resume_schema": {
                    "type": "object",
                    "required": ["decision"],
                    "properties": {"decision": {"type": "string"}},
                },
            },
        )
        run = await self.store.get_run(run_id)
        self.assertEqual("waiting_human", run["status"])
        self.assertEqual(interrupt_id, run["active_interrupt_id"])

        interrupt = await self.store.get_interrupt(run_id, interrupt_id)
        self.assertEqual("pending", interrupt["status"])
        self.assertIn("decision", interrupt["resume_schema"]["required"])

        resume = ResumeRunRequest.model_validate(
            {
                "protocol_version": "2.0",
                "run_id": run_id,
                "interrupt_id": interrupt_id,
                "expected_checkpoint_version": 1,
                "idempotency_key": "resume-pg-1",
                "resume_input": {"decision": "approved"},
            }
        )
        accepted = await self.store.accept_resume(resume)
        self.assertEqual("queued", accepted.status)

        pending = await self.store.get_pending_resume(run_id)
        self.assertEqual("approved", pending["resume_input"]["decision"])

        # Resume replay is idempotent.
        replay = await self.store.accept_resume(resume)
        self.assertTrue(replay.replayed)

        cancel = CancelRunRequest.model_validate(
            {
                "protocol_version": "2.0",
                "run_id": run_id,
                "reason": "pg-store-test",
                "requested_by": str(uuid.uuid4()),
                "idempotency_key": "cancel-pg-1",
            }
        )
        cancelled = await self.store.cancel(cancel)
        self.assertEqual("cancelled", cancelled.status)

        run = await self.store.get_run(run_id)
        self.assertEqual("cancelled", run["status"])

    async def test_recoverable_scan_and_enqueue_runtime_event(self) -> None:
        run_id = str(uuid.uuid4())
        await self.store.create_run(_start_request(run_id))

        recoverable = await self.store.list_recoverable_run_ids()
        self.assertIn(run_id, recoverable)

        await self.store.begin_execution(run_id, recovered=False)
        await self.store.enqueue_runtime_event(
            run_id, "tool.called", {"tool_code": "pg.tool.read"}
        )
        events = await self.store.pending_events()
        tool_events = [
            e["body"] for e in events
            if e["run_id"] == run_id and e["body"]["type"] == "tool.called"
        ]
        self.assertEqual(1, len(tool_events))
        self.assertEqual("pg.tool.read", tool_events[0]["payload"]["tool_code"])

    async def test_event_retry_backoff_and_dead_letter(self) -> None:
        run_id = str(uuid.uuid4())
        await self.store.create_run(_start_request(run_id))
        await self.store.begin_execution(run_id, recovered=False)

        events = await self.store.pending_events()
        first = events[0]

        await self.store.mark_event_failed(first["event_id"], "go-backend 503")
        # Backoff pushes the event into the future: no longer head-eligible.
        head = await self.store.pending_head_events(limit=10)
        self.assertEqual(
            [], [e for e in head if e["event_id"] == first["event_id"]]
        )
        # A second run's events are not blocked by this run's backoff.
        other_run = str(uuid.uuid4())
        await self.store.create_run(_start_request(other_run))
        await self.store.begin_execution(other_run, recovered=False)
        head = await self.store.pending_head_events(limit=10)
        self.assertTrue(any(e["run_id"] == other_run for e in head))

        await self.store.mark_event_dead(first["event_id"], "go-backend 400")
        pending = await self.store.pending_events()
        self.assertEqual(
            [], [e for e in pending if e["event_id"] == first["event_id"]]
        )


if __name__ == "__main__":
    unittest.main()

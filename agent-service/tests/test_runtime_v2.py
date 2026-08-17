from __future__ import annotations

import asyncio
import tempfile
import unittest
import uuid
from typing import Any, TypedDict

from langgraph.checkpoint.sqlite.aio import AsyncSqliteSaver
from langgraph.graph import END, START, StateGraph
from langgraph.types import interrupt

from app.runtime.models import CancelRunRequest, ResumeRunRequest, StartRunRequest
from app.runtime.service import RuntimeV2Service
from app.runtime.store import RuntimeStore, RuntimeStoreError


class SyntheticState(TypedDict, total=False):
    trace_id: str
    result: str
    decision: dict[str, Any]


def _success_graph(checkpointer: AsyncSqliteSaver) -> Any:
    graph = StateGraph(SyntheticState)
    graph.add_node("finish", lambda _: {"result": "ok"})
    graph.add_edge(START, "finish")
    graph.add_edge("finish", END)
    return graph.compile(checkpointer=checkpointer)


def _interrupt_graph(checkpointer: AsyncSqliteSaver) -> Any:
    def require_decision(_: SyntheticState) -> dict[str, Any]:
        value = interrupt(
            {
                "kind": "human",
                "resume_schema": {
                    "type": "object",
                    "required": ["decision"],
                    "additionalProperties": False,
                    "properties": {
                        "decision": {
                            "type": "string",
                            "enum": ["approved", "rejected"],
                        }
                    },
                },
            }
        )
        return {"decision": value, "result": "resumed"}

    graph = StateGraph(SyntheticState)
    graph.add_node("require_decision", require_decision)
    graph.add_edge(START, "require_decision")
    graph.add_edge("require_decision", END)
    return graph.compile(checkpointer=checkpointer)


def _slow_graph(
    checkpointer: AsyncSqliteSaver, release: asyncio.Event
) -> Any:
    async def wait_for_release(_: SyntheticState) -> dict[str, Any]:
        await release.wait()
        return {"result": "released"}

    graph = StateGraph(SyntheticState)
    graph.add_node("wait", wait_for_release)
    graph.add_edge(START, "wait")
    graph.add_edge("wait", END)
    return graph.compile(checkpointer=checkpointer)


def _interrupt_then_slow_graph(
    checkpointer: AsyncSqliteSaver,
    slow_started: asyncio.Event,
    release: asyncio.Event,
) -> Any:
    def require_decision(_: SyntheticState) -> dict[str, Any]:
        value = interrupt(
            {
                "kind": "human",
                "resume_schema": {
                    "type": "object",
                    "required": ["decision"],
                    "properties": {"decision": {"type": "string"}},
                },
            }
        )
        return {"decision": value}

    async def wait_after_resume(_: SyntheticState) -> dict[str, Any]:
        slow_started.set()
        await release.wait()
        return {"result": "resumed-once"}

    graph = StateGraph(SyntheticState)
    graph.add_node("require_decision", require_decision)
    graph.add_node("wait_after_resume", wait_after_resume)
    graph.add_edge(START, "require_decision")
    graph.add_edge("require_decision", "wait_after_resume")
    graph.add_edge("wait_after_resume", END)
    return graph.compile(checkpointer=checkpointer)


def _start_request(run_id: str, graph_key: str) -> StartRunRequest:
    return StartRunRequest.model_validate(
        {
            "protocol_version": "2.0",
            "run_id": run_id,
            "thread_id": str(uuid.uuid4()),
            "trace_id": str(uuid.uuid4()),
            "workflow_instance_id": str(uuid.uuid4()),
            "node_instance_id": str(uuid.uuid4()),
            "business_app_code": "test",
            "graph": {"key": graph_key, "version": "1.0.0"},
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


class RuntimeV2Test(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.db_path = f"{self.temp_dir.name}/runtime.sqlite3"
        self.checkpointer_context = AsyncSqliteSaver.from_conn_string(self.db_path)
        self.checkpointer = await self.checkpointer_context.__aenter__()
        await self.checkpointer.conn.execute("PRAGMA journal_mode = WAL")
        await self.checkpointer.setup()
        self.store = RuntimeStore(self.db_path)
        await self.store.setup()
        self.slow_release = asyncio.Event()
        self.resume_slow_started = asyncio.Event()
        self.resume_slow_release = asyncio.Event()
        self.graphs = {
            ("success_graph", "1.0.0"): _success_graph(self.checkpointer),
            ("interrupt_graph", "1.0.0"): _interrupt_graph(self.checkpointer),
            ("slow_graph", "1.0.0"): _slow_graph(
                self.checkpointer, self.slow_release
            ),
            ("interrupt_then_slow_graph", "1.0.0"): _interrupt_then_slow_graph(
                self.checkpointer,
                self.resume_slow_started,
                self.resume_slow_release,
            ),
        }
        self.service = RuntimeV2Service(
            self.store, self._get_graph, self._build_initial_state
        )

    async def asyncTearDown(self) -> None:
        await self.service.shutdown()
        await self.checkpointer_context.__aexit__(None, None, None)
        self.temp_dir.cleanup()

    def _get_graph(self, key: str, version: str) -> Any:
        graph = self.graphs.get((key, version))
        if graph is None:
            raise KeyError(f"{key}@{version}")
        return graph

    @staticmethod
    def _build_initial_state(
        _: str, __: str, request: dict[str, Any]
    ) -> dict[str, Any]:
        return {"trace_id": request["trace_id"]}

    async def _wait_for_status(self, run_id: str, expected: set[str]) -> dict[str, Any]:
        for _ in range(200):
            run = await self.store.get_run(run_id)
            if run["status"] in expected:
                return run
            await asyncio.sleep(0.01)
        self.fail(f"run {run_id} did not reach {expected}")

    async def test_start_persists_checkpoint_events_and_is_idempotent(self) -> None:
        run_id = str(uuid.uuid4())
        request = _start_request(run_id, "success_graph")
        accepted = await self.service.start(request)
        self.assertEqual("queued", accepted.status)

        run = await self._wait_for_status(run_id, {"succeeded"})
        self.assertEqual(1, run["checkpoint_version"])

        events = await self.store.pending_events()
        run_events = [event["body"] for event in events if event["run_id"] == run_id]
        self.assertEqual(
            ["run.started", "step.started", "checkpoint.saved", "step.completed", "run.succeeded"],
            [event["type"] for event in run_events],
        )
        self.assertEqual(list(range(1, 6)), [event["sequence"] for event in run_events])

        replay = await self.service.start(request)
        self.assertTrue(replay.replayed)
        self.assertEqual("succeeded", replay.status)

        changed = request.model_copy(update={"input": {"changed": True}})
        with self.assertRaisesRegex(RuntimeStoreError, "different Start request"):
            await self.service.start(changed)

    async def test_interrupt_resume_schema_version_and_duplicate_resume(self) -> None:
        run_id = str(uuid.uuid4())
        await self.service.start(_start_request(run_id, "interrupt_graph"))
        waiting = await self._wait_for_status(run_id, {"waiting_human"})
        self.assertEqual(1, waiting["checkpoint_version"])
        interrupt_id = waiting["active_interrupt_id"]
        self.assertTrue(interrupt_id)

        invalid = ResumeRunRequest(
            protocol_version="2.0",
            run_id=run_id,
            interrupt_id=interrupt_id,
            expected_checkpoint_version=1,
            idempotency_key="resume-invalid",
            resume_input={"decision": "maybe"},
        )
        with self.assertRaisesRegex(RuntimeStoreError, "outside its allowed values"):
            await self.service.resume(invalid)

        stale = invalid.model_copy(
            update={
                "expected_checkpoint_version": 2,
                "idempotency_key": "resume-stale",
                "resume_input": {"decision": "approved"},
            }
        )
        with self.assertRaisesRegex(RuntimeStoreError, "current is 1"):
            await self.service.resume(stale)

        resume = invalid.model_copy(
            update={
                "idempotency_key": "resume-approved",
                "resume_input": {"decision": "approved"},
            }
        )
        first = await self.service.resume(resume)
        replay = await self.service.resume(resume)
        self.assertEqual("queued", first.status)
        self.assertTrue(replay.replayed)

        completed = await self._wait_for_status(run_id, {"succeeded"})
        self.assertEqual(2, completed["checkpoint_version"])
        events = await self.store.pending_events()
        event_types = [event["body"]["type"] for event in events if event["run_id"] == run_id]
        self.assertEqual(1, event_types.count("run.resumed"))
        self.assertEqual(1, event_types.count("run.interrupted"))

    async def test_waiting_run_resumes_after_runtime_service_recreation(self) -> None:
        run_id = str(uuid.uuid4())
        await self.service.start(_start_request(run_id, "interrupt_graph"))
        waiting = await self._wait_for_status(run_id, {"waiting_human"})
        interrupt_id = waiting["active_interrupt_id"]

        await self.service.shutdown()
        self.service = RuntimeV2Service(
            self.store, self._get_graph, self._build_initial_state
        )
        await self.service.recover()
        # Waiting runs remain quiescent until an explicit, versioned Resume.
        self.assertEqual("waiting_human", (await self.store.get_run(run_id))["status"])

        await self.service.resume(
            ResumeRunRequest(
                protocol_version="2.0",
                run_id=run_id,
                interrupt_id=interrupt_id,
                expected_checkpoint_version=1,
                idempotency_key="resume-after-restart",
                resume_input={"decision": "approved"},
            )
        )
        completed = await self._wait_for_status(run_id, {"succeeded"})
        self.assertEqual(2, completed["checkpoint_version"])

    async def test_cancel_is_terminal_and_idempotent(self) -> None:
        run_id = str(uuid.uuid4())
        await self.service.start(_start_request(run_id, "interrupt_graph"))
        waiting = await self._wait_for_status(run_id, {"waiting_human"})

        request = CancelRunRequest(
            protocol_version="2.0",
            run_id=run_id,
            reason="cancelled_by_test",
            requested_by=str(uuid.uuid4()),
            idempotency_key="cancel-once",
        )
        first = await self.service.cancel(request)
        replay = await self.service.cancel(request)
        self.assertEqual("cancelled", first.status)
        self.assertTrue(replay.replayed)
        self.assertEqual("cancelled", (await self.store.get_run(run_id))["status"])
        interrupt = await self.store.get_interrupt(run_id, waiting["active_interrupt_id"])
        self.assertEqual("cancelled", interrupt["status"])

        events = await self.store.pending_events()
        event_types = [event["body"]["type"] for event in events if event["run_id"] == run_id]
        self.assertEqual(1, event_types.count("run.cancelled"))

    async def test_cancel_running_graph_closes_active_step(self) -> None:
        run_id = str(uuid.uuid4())
        await self.service.start(_start_request(run_id, "slow_graph"))
        running = await self._wait_for_status(run_id, {"running"})
        self.assertTrue(running["active_step_id"])

        await self.service.cancel(
            CancelRunRequest(
                protocol_version="2.0",
                run_id=run_id,
                reason="cancel_running",
                requested_by=str(uuid.uuid4()),
                idempotency_key="cancel-running-once",
            )
        )
        cancelled = await self.store.get_run(run_id)
        self.assertEqual("cancelled", cancelled["status"])
        self.assertIsNone(cancelled["active_step_id"])
        events = await self.store.pending_events()
        bodies = [event["body"] for event in events if event["run_id"] == run_id]
        self.assertEqual(
            ["run.started", "step.started", "step.failed", "run.cancelled"],
            [event["type"] for event in bodies],
        )
        self.assertEqual("cancelled", bodies[-2]["payload"]["status"])

    async def test_restart_after_resume_consumption_does_not_inject_resume_twice(self) -> None:
        run_id = str(uuid.uuid4())
        await self.service.start(_start_request(run_id, "interrupt_then_slow_graph"))
        waiting = await self._wait_for_status(run_id, {"waiting_human"})
        await self.service.resume(
            ResumeRunRequest(
                protocol_version="2.0",
                run_id=run_id,
                interrupt_id=waiting["active_interrupt_id"],
                expected_checkpoint_version=1,
                idempotency_key="resume-before-restart",
                resume_input={"decision": "approved"},
            )
        )
        await asyncio.wait_for(self.resume_slow_started.wait(), timeout=2)

        await self.service.shutdown()
        self.service = RuntimeV2Service(
            self.store, self._get_graph, self._build_initial_state
        )
        self.resume_slow_release.set()
        await self.service.recover()
        completed = await self._wait_for_status(run_id, {"succeeded"})
        self.assertEqual(2, completed["checkpoint_version"])
        events = await self.store.pending_events()
        event_types = [event["body"]["type"] for event in events if event["run_id"] == run_id]
        self.assertEqual(1, event_types.count("run.resumed"))


if __name__ == "__main__":
    unittest.main()

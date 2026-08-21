from __future__ import annotations

import asyncio
import hashlib
import json
import logging
from typing import Any, Callable

from langgraph.types import Command

from app.output_envelope import build_run_envelope
from app.runtime.event_hooks import _make_runtime_event_hook
from app.runtime.models import (
    AcceptedRunResponse,
    CancelRunRequest,
    ResumeRunRequest,
    StartRunRequest,
)
from app.runtime.store import RuntimeStore, RuntimeStoreError

logger = logging.getLogger(__name__)

GraphResolver = Callable[[str, str], Any]
InitialStateResolver = Callable[[str, str, dict[str, Any]], dict[str, Any]]


class RuntimeV2Service:
    def __init__(
        self,
        store: RuntimeStore,
        graph_resolver: GraphResolver,
        initial_state_resolver: InitialStateResolver,
        trace_poster: Any | None = None,
    ) -> None:
        self.store = store
        self.graph_resolver = graph_resolver
        self.initial_state_resolver = initial_state_resolver
        # M5-A: L3 Model Turn 追踪 (可选;未配置 TRACE_EVENT_URL 时为 None)
        self.trace_poster = trace_poster
        self._tasks: dict[str, asyncio.Task[None]] = {}
        self._tasks_lock = asyncio.Lock()

    async def start(self, request: StartRunRequest) -> AcceptedRunResponse:
        # Reject unsupported graph/version before persisting an unexecutable Run.
        self.graph_resolver(request.graph.key, request.graph.version)
        response, created = await self.store.create_run(request)
        if created or response.status in {"queued", "running"}:
            await self._schedule(request.run_id)
        return response

    async def resume(self, request: ResumeRunRequest) -> AcceptedRunResponse:
        interrupt = await self.store.get_interrupt(request.run_id, request.interrupt_id)
        _validate_resume_input(request.resume_input, interrupt["resume_schema"])
        response = await self.store.accept_resume(request)
        if not response.replayed:
            await self._schedule(request.run_id)
        return response

    async def cancel(self, request: CancelRunRequest) -> AcceptedRunResponse:
        response = await self.store.cancel(request)
        async with self._tasks_lock:
            task = self._tasks.get(request.run_id)
            if task is not None and not task.done():
                task.cancel()
        return response

    async def recover(self) -> None:
        for run_id in await self.store.list_recoverable_run_ids():
            await self._schedule(run_id)

    async def shutdown(self) -> None:
        async with self._tasks_lock:
            tasks = list(self._tasks.values())
            for task in tasks:
                if not task.done():
                    task.cancel()
        if tasks:
            await asyncio.gather(*tasks, return_exceptions=True)

    async def _schedule(self, run_id: str) -> None:
        async with self._tasks_lock:
            existing = self._tasks.get(run_id)
            if existing is not None and not existing.done():
                return
            task = asyncio.create_task(self._execute(run_id), name=f"runtime-v2:{run_id}")
            self._tasks[run_id] = task
            task.add_done_callback(lambda _: self._discard_task(run_id, task))

    def _discard_task(self, run_id: str, task: asyncio.Task[None]) -> None:
        if self._tasks.get(run_id) is task:
            self._tasks.pop(run_id, None)

    def _build_trace_handler(self, run: dict[str, Any]) -> Any | None:
        """M5-A: 为本次执行构造 L3 Trace 回调 (poster 未启用时返回 None)。"""
        poster = getattr(self, "trace_poster", None)
        if poster is None or not poster.enabled():
            return None
        try:
            request = json.loads(run.get("request_json") or "{}")
            trace_id = str(request.get("trace_id") or run.get("run_id") or "")
            if not trace_id:
                return None
            from app.core.trace_client import TraceCallbackHandler

            return TraceCallbackHandler(
                poster, str(run.get("tenant_id", "")), trace_id, str(run.get("run_id", ""))
            )
        except Exception:  # noqa: BLE001 - trace 构造失败不影响执行
            return None

    async def _execute(self, run_id: str) -> None:
        try:
            run = await self.store.get_run(run_id)
            graph = self.graph_resolver(run["graph_key"], run["graph_version"])
            config: dict[str, Any] = {"configurable": {"thread_id": run_id}}
            trace_handler = self._build_trace_handler(run)
            if trace_handler is not None:
                config["callbacks"] = [trace_handler]
            # M1-B: runtime event hooks — emit tool.called / artifact.ready /
            # step.* events to the Go control plane as LangGraph progresses.
            config["callbacks"] = list(config.get("callbacks") or []) + [
                _make_runtime_event_hook(self.store, run_id),
            ]
            pending_resume = await self.store.get_pending_resume(run_id)

            state_snapshot: Any | None = None
            try:
                state_snapshot = await graph.aget_state(config)
                has_checkpoint = bool(state_snapshot.values or state_snapshot.next)
            except Exception:
                # A new thread has no checkpoint yet. Invocation below will create it.
                has_checkpoint = False

            recovered = run["status"] == "running" and pending_resume is None and has_checkpoint
            if not await self.store.begin_execution(run_id, recovered=recovered):
                return

            if pending_resume is not None:
                pending_interrupt_id = pending_resume["interrupt_id"]
                checkpoint_interrupt_ids = {
                    item.id for item in (getattr(state_snapshot, "interrupts", ()) or ())
                }
                if pending_interrupt_id in checkpoint_interrupt_ids:
                    graph_input: Any = Command(resume=pending_resume["resume_input"])
                elif has_checkpoint:
                    # The Resume was already consumed and a later node checkpointed
                    # before the process stopped. Continue from that cursor without
                    # injecting the same Resume value twice.
                    graph_input = None
                else:
                    raise RuntimeStoreError(
                        "INTERRUPT_CONFLICT",
                        "pending Resume has no matching persistent interrupt checkpoint",
                    )
            elif has_checkpoint:
                graph_input = None
            else:
                graph_input = self.initial_state_resolver(
                    run["graph_key"], run["graph_version"], run["request"]
                )

            final_state = await graph.ainvoke(graph_input, config)
            snapshot = await graph.aget_state(config)
            checkpoint_id = snapshot.config.get("configurable", {}).get("checkpoint_id", "unknown")
            backend = getattr(self.store, "backend_name", "sqlite")
            checkpoint_ref = f"langgraph-{backend}:{run_id}:{checkpoint_id}"
            state_hash = _state_hash(final_state)
            interrupt_data = _extract_interrupt(final_state)
            # V1/V2 同形 envelope(M2-A):graph 正常返回但含 error/校验失败时,
            # 与 V1 契约一致地落为 run.failed;成功时 output/usage 随事件回传。
            envelope = (
                build_run_envelope(final_state)
                if isinstance(final_state, dict)
                else None
            )
            if envelope is not None and envelope["status"] == "failed" and interrupt_data is None:
                error = envelope.get("error") or {}
                await self.store.record_failure(
                    run_id,
                    error.get("code") or "GRAPH_EXECUTION_FAILED",
                    error.get("message") or "graph final state is failed",
                )
                return
            await self.store.record_result(
                run_id, checkpoint_ref, state_hash, interrupt_data, envelope
            )
        except asyncio.CancelledError:
            # Cancel is persisted before the task is interrupted. Graceful service
            # shutdown leaves the Run recoverable and does not fabricate failure.
            raise
        except Exception as exc:
            logger.exception("Runtime V2 execution failed for run %s", run_id)
            try:
                await self.store.record_failure(
                    run_id, "GRAPH_EXECUTION_FAILED", str(exc) or type(exc).__name__
                )
            except RuntimeStoreError:
                logger.exception("Could not persist Runtime V2 failure for run %s", run_id)


def _extract_interrupt(final_state: Any) -> dict[str, Any] | None:
    if not isinstance(final_state, dict):
        return None
    interrupts = final_state.get("__interrupt__") or []
    if not interrupts:
        return None
    interrupt = interrupts[0]
    value = interrupt.value if isinstance(interrupt.value, dict) else {}
    kind = value.get("kind", "input_required")
    if kind not in {"human", "external", "input_required"}:
        kind = "input_required"
    resume_schema = value.get("resume_schema")
    if not isinstance(resume_schema, dict):
        resume_schema = {"type": "object", "additionalProperties": True}
    return {
        "interrupt_id": interrupt.id,
        "kind": kind,
        "resume_schema": resume_schema,
    }


def _state_hash(state: Any) -> str:
    try:
        serialized = json.dumps(state, ensure_ascii=False, sort_keys=True, default=str)
    except (TypeError, ValueError):
        serialized = repr(type(state))
    return hashlib.sha256(serialized.encode("utf-8")).hexdigest()


def _validate_resume_input(value: dict[str, Any], schema: dict[str, Any]) -> None:
    if schema.get("type", "object") != "object":
        raise RuntimeStoreError("INVALID_RESUME_SCHEMA", "only object resume schemas are supported")
    required = schema.get("required", [])
    if not isinstance(required, list):
        raise RuntimeStoreError("INVALID_RESUME_SCHEMA", "resume schema required must be a list")
    missing = [key for key in required if key not in value]
    if missing:
        raise RuntimeStoreError(
            "RESUME_INPUT_INVALID", f"missing required resume fields: {', '.join(missing)}", 422
        )
    properties = schema.get("properties", {})
    if not isinstance(properties, dict):
        raise RuntimeStoreError("INVALID_RESUME_SCHEMA", "resume schema properties must be an object")
    if schema.get("additionalProperties") is False:
        unknown = sorted(set(value) - set(properties))
        if unknown:
            raise RuntimeStoreError(
                "RESUME_INPUT_INVALID", f"unknown resume fields: {', '.join(unknown)}", 422
            )
    for key, rule in properties.items():
        if key not in value or not isinstance(rule, dict):
            continue
        expected_type = rule.get("type")
        if expected_type and not _matches_json_type(value[key], expected_type):
            raise RuntimeStoreError(
                "RESUME_INPUT_INVALID", f"resume field {key!r} must be {expected_type}", 422
            )
        allowed = rule.get("enum")
        if isinstance(allowed, list) and value[key] not in allowed:
            raise RuntimeStoreError(
                "RESUME_INPUT_INVALID", f"resume field {key!r} is outside its allowed values", 422
            )


def _matches_json_type(value: Any, expected: str) -> bool:
    checks = {
        "string": lambda item: isinstance(item, str),
        "number": lambda item: isinstance(item, (int, float)) and not isinstance(item, bool),
        "integer": lambda item: isinstance(item, int) and not isinstance(item, bool),
        "boolean": lambda item: isinstance(item, bool),
        "object": lambda item: isinstance(item, dict),
        "array": lambda item: isinstance(item, list),
        "null": lambda item: item is None,
    }
    check = checks.get(expected)
    return check(value) if check is not None else False

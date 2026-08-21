"""LangGraph callback that translates node/tool progress into runtime events.

M1-B: the hook is attached at graph invocation time and emits
``step.started`` / ``step.completed`` / ``step.failed`` (generic node
transitions) plus the higher-granularity ``tool.called`` and
``artifact.ready`` events. All events are enqueued into the runtime outbox
so the RuntimeEventDispatcher can deliver them to Go and then to the
conversation SSE channel.

The hook deliberately swallows exceptions: tool execution must not break
because of an event-delivery failure.
"""

from __future__ import annotations

import json
import logging
from typing import Any

from langchain_core.callbacks import BaseCallbackHandler

from app.runtime.store import RuntimeStore

logger = logging.getLogger(__name__)


def _truncate(text: str, limit: int = 4000) -> str:
    if text is None:
        return ""
    return text if len(text) <= limit else text[:limit] + "...(truncated)"


def _safe_json(obj: Any) -> str:
    try:
        return json.dumps(obj, ensure_ascii=False, default=str)
    except Exception:
        return str(obj)


def _make_runtime_event_hook(store: RuntimeStore, run_id: str) -> BaseCallbackHandler:
    """Build a callback handler wired to the given run's outbox."""

    class RuntimeEventHook(BaseCallbackHandler):
        def __init__(self) -> None:
            super().__init__()
            self._node_depth = 0
            # run_ids of LangGraph node executions (metadata carries
            # ``langgraph_node``); only these emit node-level step events.
            self._node_run_ids: set[str] = set()

        async def on_chain_start(
            self, serialized: dict[str, Any], inputs: dict[str, Any], *, run_id: str,
            parent_run_id: str | None = None, tags: list[str] | None = None,
            metadata: dict[str, Any] | None = None, **kwargs: Any,
        ) -> None:
            if not (metadata or {}).get("langgraph_node"):
                return  # outer graph / non-node chain — no node-level event
            self._node_run_ids.add(run_id)
            self._node_depth += 1
            depth = self._node_depth
            name = (metadata or {}).get("langgraph_node") or (serialized or {}).get("name") or "node"
            await self._emit_guarded("step.started", {
                "step_id": f"node-{run_id}",
                "step_sequence": depth,
                "step_type": "node",
                "name": str(name),
                "input": _truncate(_safe_json(inputs)) if inputs else "",
            })

        async def on_chain_end(
            self, outputs: Any, *, run_id: str,
            parent_run_id: str | None = None, tags: list[str] | None = None,
            **kwargs: Any,
        ) -> None:
            if run_id not in self._node_run_ids:
                return
            self._node_run_ids.discard(run_id)
            if self._node_depth > 0:
                self._node_depth -= 1
            await self._emit_guarded("step.completed", {
                "step_id": f"node-{run_id}",
                "step_type": "node",
                "output": _truncate(_safe_json(outputs)) if outputs is not None else "",
            })
            # Emit artifact.ready when the node output looks like a deliverable.
            if outputs is not None:
                await self._maybe_emit_artifacts(outputs, run_id)

        async def on_chain_error(
            self, error: BaseException, *, run_id: str,
            parent_run_id: str | None = None, tags: list[str] | None = None,
            **kwargs: Any,
        ) -> None:
            if run_id not in self._node_run_ids:
                return
            self._node_run_ids.discard(run_id)
            if self._node_depth > 0:
                self._node_depth -= 1
            await self._emit_guarded("step.failed", {
                "step_id": f"node-{run_id}",
                "step_type": "node",
                "error": {"code": type(error).__name__, "message": _truncate(str(error), 800)},
            })

        async def on_tool_start(
            self, serialized: dict[str, Any], input_str: str, *, run_id: str,
            parent_run_id: str | None = None, tags: list[str] | None = None,
            metadata: dict[str, Any] | None = None, **kwargs: Any,
        ) -> None:
            tool_name = serialized.get("name") or serialized.get("tool") or "tool"
            await self._emit_guarded("tool.called", {
                "tool_call_id": run_id,
                "tool_name": str(tool_name),
                "input": _truncate(input_str or "", 1000),
                "status": "run",
            })

        async def on_tool_end(
            self, output: str, *, run_id: str,
            parent_run_id: str | None = None, tags: list[str] | None = None,
            **kwargs: Any,
        ) -> None:
            await self._emit_guarded("tool.called", {
                "tool_call_id": run_id,
                "status": "ok",
                "output": _truncate(output or "", 1500),
            })

        async def on_tool_error(
            self, error: BaseException, *, run_id: str,
            parent_run_id: str | None = None, tags: list[str] | None = None,
            **kwargs: Any,
        ) -> None:
            await self._emit_guarded("tool.called", {
                "tool_call_id": run_id,
                "status": "err",
                "error": {"code": type(error).__name__, "message": _truncate(str(error), 800)},
            })

        async def on_agent_finish(
            self, finish: Any, *, run_id: str,
            parent_run_id: str | None = None, tags: list[str] | None = None,
            **kwargs: Any,
        ) -> None:
            try:
                output = getattr(finish, "generations", None)
                if not output:
                    return
                last = output[-1][-1] if output and output[-1] else None
                if last is not None:
                    await self._maybe_emit_artifacts(getattr(last, "text", ""), run_id)
            except Exception:
                logger.exception("runtime hook on_agent_finish failed")

        async def _maybe_emit_artifacts(self, value: Any, source_id: str) -> None:
            """Best-effort extract artifacts from a string/list/dict value.

            Only strong deliverable signals count ("report" / "artifact" /
            "file" keys or markers) — generic keys like "output"/"result"
            appear in nearly every node state and would flood the panel.
            """
            blob = value if isinstance(value, str) else _safe_json(value)
            hits = []
            lower = blob.lower()
            for marker in ("report", "artifact", "file"):
                if marker in lower:
                    hits.append(marker)
                    break
            # Try to find JSON-encoded deliverables.
            try:
                obj = json.loads(blob) if isinstance(blob, str) and blob.strip().startswith(("{", "[")) else None
                if isinstance(obj, dict):
                    for key in ("artifact", "artifacts", "report", "file", "files", "document", "markdown"):
                        if key in obj:
                            hits.append(key)
                            break
            except Exception:
                obj = None
            if not hits:
                return
            await self._emit_guarded("artifact.ready", {
                "artifact_id": source_id,
                "sources": hits,
                "preview": _truncate(blob, 800),
            })

        async def _emit_guarded(self, event_type: str, payload: dict[str, Any]) -> None:
            try:
                await store.enqueue_runtime_event(run_id, event_type, payload)
            except Exception:
                logger.exception("runtime hook enqueue failed for %s on run %s", event_type, run_id)

    return RuntimeEventHook()


# Helpers imported by graph authors that want to emit an artifact explicitly
# from inside a tool or node (not only via callback inference).
async def emit_artifact(store: RuntimeStore, run_id: str, name: str, preview: str) -> None:
    await store.enqueue_runtime_event(run_id, "artifact.ready", {
        "artifact_id": name,
        "sources": ["explicit"],
        "preview": _truncate(preview, 800),
    })


__all__ = ["_make_runtime_event_hook", "emit_artifact"]

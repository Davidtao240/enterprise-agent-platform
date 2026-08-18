"""M5-A L3 Model Turn 追踪客户端。

以 LangChain 回调方式在全图唯一执行路径 (RuntimeV2Service -> graph.ainvoke)
捕获每次 LLM 调用的 start/end/error,fire-and-forget 上报至 Go 平台
POST /internal/v1/trace/events (InternalServiceToken + X-Tenant-ID)。

设计约束 (TRACE_AND_EVAL.md §2.3):
- best-effort:上报失败仅记 debug 日志,绝不影响 Agent 执行;
- 异步非阻塞:使用 loop.create_task,不 await。
"""

from __future__ import annotations

import asyncio
import logging
import time
from datetime import datetime, timezone
from typing import Any

import httpx
from langchain_core.callbacks import BaseCallbackHandler

logger = logging.getLogger(__name__)

LAYER_MODEL_TURN = "L3"


class TraceEventPoster:
    """共享的 trace 事件上报器 (进程内单例使用)。"""

    def __init__(self, url: str, service_token: str) -> None:
        self.url = (url or "").strip()
        self.service_token = (service_token or "").strip()
        self._client: httpx.AsyncClient | None = None

    def enabled(self) -> bool:
        return bool(self.url) and bool(self.service_token)

    def post(self, events: list[dict[str, Any]]) -> None:
        """调度一次异步上报;无事件或未启用时静默跳过。"""
        if not events or not self.enabled():
            return
        try:
            loop = asyncio.get_running_loop()
            loop.create_task(self._post(events))
        except RuntimeError:
            # 无事件循环(同步上下文)时丢弃,trace 是 best-effort 观测面。
            pass

    async def _post(self, events: list[dict[str, Any]]) -> None:
        try:
            if self._client is None:
                self._client = httpx.AsyncClient(timeout=5.0)
            await self._client.post(
                self.url,
                json={"events": events},
                headers={
                    "X-Internal-Service-Token": self.service_token,
                    "X-Tenant-ID": str(events[0].get("tenant_id", "")),
                },
            )
        except Exception as exc:  # noqa: BLE001 - trace 上报失败不影响主链路
            logger.debug("trace event post failed: %s", exc)

    async def aclose(self) -> None:
        if self._client is not None:
            await self._client.aclose()
            self._client = None


class TraceCallbackHandler(BaseCallbackHandler):
    """L3 Model Turn 回调:LLM 调用 start/end -> trace_events。"""

    def __init__(
        self,
        poster: TraceEventPoster,
        tenant_id: str,
        trace_id: str,
        run_id: str,
    ) -> None:
        self.poster = poster
        self.tenant_id = tenant_id
        self.trace_id = trace_id
        self.run_id = run_id
        self._starts: dict[str, float] = {}

    def _event(self, event_type: str, **extra: Any) -> dict[str, Any]:
        event: dict[str, Any] = {
            "trace_id": self.trace_id,
            "layer": LAYER_MODEL_TURN,
            "event_type": event_type,
            "tenant_id": self.tenant_id,
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "metadata": {"run_id": self.run_id},
        }
        event.update({k: v for k, v in extra.items() if v is not None})
        return event

    def on_llm_start(self, serialized: dict[str, Any], prompts: list[str], *, run_id: Any, **kwargs: Any) -> None:
        self._starts[str(run_id)] = time.monotonic()
        model = (serialized or {}).get("name") or (serialized or {}).get("id", "")
        self.poster.post([self._event("start", payload={"model": model})])

    def on_llm_end(self, response: Any, *, run_id: Any, **kwargs: Any) -> None:
        duration_ms = None
        start = self._starts.pop(str(run_id), None)
        if start is not None:
            duration_ms = int((time.monotonic() - start) * 1000)
        usage: dict[str, Any] = {}
        llm_output = getattr(response, "llm_output", None) or {}
        if isinstance(llm_output, dict):
            token_usage = llm_output.get("token_usage") or llm_output.get("usage")
            if isinstance(token_usage, dict):
                usage = {
                    "prompt_tokens": token_usage.get("prompt_tokens"),
                    "completion_tokens": token_usage.get("completion_tokens"),
                    "total_tokens": token_usage.get("total_tokens"),
                }
        payload: dict[str, Any] = {"usage": usage} if usage else {}
        self.poster.post([self._event("end", duration_ms=duration_ms, payload=payload or None)])

    def on_llm_error(self, error: BaseException, *, run_id: Any, **kwargs: Any) -> None:
        self._starts.pop(str(run_id), None)
        self.poster.post([self._event("error", payload={"error": str(error)})])

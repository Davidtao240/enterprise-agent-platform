from __future__ import annotations

import asyncio
import logging

import httpx

from app.runtime.store import RuntimeStore

logger = logging.getLogger(__name__)


class RuntimeEventDispatcher:
    """按 run 顺序至少一次投递持久 Outbox 事件,直到 Go 确认。

    投递规则:
    - 每轮只取每个 run 最靠前、退避已到期的待投递事件(head-of-line),
      失败事件在退避期内会阻塞同 run 的后续事件,跨批次不乱序。
    - Go 返回 2xx:事件标记 delivered。
    - Go 返回 4xx:终态拒绝(如 RUN_NOT_FOUND / 状态冲突),标记 dead
      并告警,不再无限重试。
    - Go 返回 5xx 或网络错误:指数退避(上限 60s)后重试。
    """

    def __init__(
        self,
        store: RuntimeStore,
        event_url: str,
        service_token: str,
        poll_interval_seconds: float = 0.25,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        self.store = store
        self.event_url = event_url.strip()
        self.service_token = service_token
        self.poll_interval_seconds = poll_interval_seconds
        self._client = client if client is not None else httpx.AsyncClient(timeout=10.0)
        self._task: asyncio.Task[None] | None = None
        self._stopping = asyncio.Event()

    async def start(self) -> None:
        if not self.event_url or self._task is not None:
            return
        self._stopping.clear()
        self._task = asyncio.create_task(self._run(), name="runtime-event-dispatcher")

    async def stop(self) -> None:
        self._stopping.set()
        if self._task is not None:
            self._task.cancel()
            await asyncio.gather(self._task, return_exceptions=True)
            self._task = None
        await self._client.aclose()

    async def flush_once(self) -> int:
        if not self.event_url:
            return 0
        delivered = 0
        events = await self.store.pending_head_events()
        for event in events:
            try:
                response = await self._client.post(
                    self.event_url,
                    json=event["body"],
                    headers={"X-Internal-Service-Token": self.service_token},
                )
                if 200 <= response.status_code < 300:
                    await self.store.mark_event_delivered(event["event_id"])
                    delivered += 1
                elif response.status_code < 500:
                    # 终态拒绝:重试无法改变 Go 的决定,直接死信并告警。
                    await self.store.mark_event_dead(
                        event["event_id"],
                        f"HTTP {response.status_code}: {response.text[:300]}",
                    )
                    logger.error(
                        "Runtime event %s dead-lettered: HTTP %s: %s",
                        event["event_id"],
                        response.status_code,
                        response.text[:300],
                    )
                else:
                    await self.store.mark_event_failed(
                        event["event_id"], f"HTTP {response.status_code}"
                    )
            except (httpx.HTTPError, OSError) as exc:
                await self.store.mark_event_failed(event["event_id"], type(exc).__name__)
        return delivered

    async def _run(self) -> None:
        while not self._stopping.is_set():
            try:
                await self.flush_once()
            except Exception:
                logger.exception("Runtime event outbox dispatch failed")
            try:
                await asyncio.wait_for(
                    self._stopping.wait(), timeout=self.poll_interval_seconds
                )
            except TimeoutError:
                pass

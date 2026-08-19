"""Token and cost accumulator for tracking LLM usage across graph execution."""

from __future__ import annotations

import time
from typing import Any


# Approximate cost per 1K tokens (USD) for common models
_COST_PER_1K: dict[str, tuple[float, float]] = {
    "qwen-plus": (0.0008, 0.002),       # prompt, completion
    "qwen-max": (0.0028, 0.0084),
    "deepseek-chat": (0.00014, 0.00028),
    "deepseek-reasoner": (0.00055, 0.00219),
}


class UsageTracker:
    """Accumulates token counts and estimated cost across multiple LLM calls."""

    def __init__(self, model: str = "") -> None:
        self.model = model
        self.prompt_tokens = 0
        self.completion_tokens = 0
        self.total_tokens = 0
        self.call_count = 0
        self.human_interaction_ms: int = 0
        self.clarification_count: int = 0
        self.rework_count: int = 0
        self.conversation_started_at: float | None = None
        self.conversation_ended_at: float | None = None
        self.active_periods: list[tuple[float, float]] = []
        self._last_activity_at: float | None = None
        self._human_wait_start: float | None = None

    def add(self, usage: dict[str, Any] | None) -> None:
        """Accumulate token counts from a LangChain response_metadata usage dict."""
        if not usage:
            return
        prompt = usage.get("prompt_tokens", 0)
        completion = usage.get("completion_tokens", 0)
        total = usage.get("total_tokens", prompt + completion)

        self.prompt_tokens += prompt
        self.completion_tokens += completion
        self.total_tokens += total
        self.call_count += 1

        if not self.model:
            self.model = usage.get("model", "")

    def _estimate_cost(self) -> float:
        """Estimate cost from token counts and model pricing."""
        prompt_cost, completion_cost = _COST_PER_1K.get(self.model, (0.001, 0.003))
        return round(
            (self.prompt_tokens / 1000) * prompt_cost
            + (self.completion_tokens / 1000) * completion_cost,
            6,
        )

    def start_conversation(self) -> None:
        self.conversation_started_at = time.monotonic()
        self._last_activity_at = self.conversation_started_at

    def end_conversation(self) -> None:
        now = time.monotonic()
        self.conversation_ended_at = now
        if self._last_activity_at is not None:
            self.active_periods.append((self._last_activity_at, now))

    def record_human_interaction_start(self) -> None:
        if self._last_activity_at is not None:
            self.active_periods.append((self._last_activity_at, time.monotonic()))
        self._human_wait_start = time.monotonic()

    def record_human_interaction_end(self) -> None:
        if self._human_wait_start is not None:
            self.human_interaction_ms += int((time.monotonic() - self._human_wait_start) * 1000)
            self.clarification_count += 1
            self._human_wait_start = None
        self._last_activity_at = time.monotonic()

    def record_rework(self) -> None:
        self.rework_count += 1

    @property
    def conversation_total_ms(self) -> int:
        if self.conversation_started_at and self.conversation_ended_at:
            return int((self.conversation_ended_at - self.conversation_started_at) * 1000)
        return 0

    @property
    def conversation_active_ms(self) -> int:
        total = 0.0
        for start, end in self.active_periods:
            total += end - start
        return int(total * 1000)

    @property
    def agent_independent_ms(self) -> int:
        return max(0, self.total_duration_ms - self.human_interaction_ms)

    @property
    def total_duration_ms(self) -> int:
        return self.call_count * 1000

    def to_avr_dict(self) -> dict[str, Any]:
        return {
            "agent_independent_ms": self.agent_independent_ms,
            "conversation_total_ms": self.conversation_total_ms,
            "conversation_active_ms": self.conversation_active_ms,
            "human_interaction_ms": self.human_interaction_ms,
            "clarification_count": self.clarification_count,
            "rework_count": self.rework_count,
            "total_tokens": self.total_tokens,
            "cost": self._estimate_cost(),
        }

    def to_dict(self) -> dict[str, Any]:
        result = {
            "model": self.model,
            "prompt_tokens": self.prompt_tokens,
            "completion_tokens": self.completion_tokens,
            "total_tokens": self.total_tokens,
            "cost": self._estimate_cost(),
            "call_count": self.call_count,
        }
        result.update(self.to_avr_dict())
        return result

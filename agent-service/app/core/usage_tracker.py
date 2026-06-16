"""Token and cost accumulator for tracking LLM usage across graph execution."""

from __future__ import annotations

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

    def to_dict(self) -> dict[str, Any]:
        return {
            "model": self.model,
            "prompt_tokens": self.prompt_tokens,
            "completion_tokens": self.completion_tokens,
            "total_tokens": self.total_tokens,
            "cost": self._estimate_cost(),
            "call_count": self.call_count,
        }

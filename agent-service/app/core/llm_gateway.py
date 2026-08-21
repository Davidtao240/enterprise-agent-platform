"""LLM Gateway — centralized multi-model routing, metering, budget, and caching.

Design principles (per M7-M9 design doc):
- Lightweight (~500 lines), no LiteLLM dependency
- Multi-model routing with configurable strategies
- Token metering per tenant / department / agent
- Budget control with circuit-breaker on overrun
- Prefix cache for repeated prompt prefixes
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import logging
import os
import time
from collections import OrderedDict
from dataclasses import dataclass, field
from typing import Any, Literal, Optional

logger = logging.getLogger(__name__)


@dataclass
class ModelConfig:
    """Configuration for an LLM model."""
    name: str
    provider: str
    base_url: str = ""
    api_key: str = ""
    model_id: str = ""
    input_price_per_1k: float = 0.008
    output_price_per_1k: float = 0.020
    max_tokens: int = 8192
    enabled: bool = True

    def __repr__(self) -> str:
        masked_key = self.api_key[:4] + "***" if len(self.api_key) > 4 else "***"
        return f"ModelConfig(name={self.name!r}, provider={self.provider!r}, model_id={self.model_id!r}, api_key={masked_key!r}, ...)"


@dataclass
class BudgetConfig:
    """Budget configuration for a tenant/department/agent."""
    daily_token_limit: int = 1_000_000
    daily_cost_limit: float = 100.0
    monthly_token_limit: int = 30_000_000
    monthly_cost_limit: float = 3000.0
    alert_threshold: float = 0.8
    enforce: bool = True


@dataclass
class UsageRecord:
    """Record of a single LLM call for metering."""
    id: str
    tenant_id: str
    agent_id: str
    model_name: str
    input_tokens: int
    output_tokens: int
    cost_usd: float
    timestamp: float
    trace_id: str = ""


class PrefixCache:
    """LRU cache for prompt prefixes to reduce redundant computation.

    Key: hash of the prompt prefix
    Value: cached completion/embedding result
    TTL-based eviction with max_size limit
    """

    def __init__(self, max_size: int = 1000, ttl_seconds: int = 300) -> None:
        self._cache: OrderedDict[str, tuple[Any, float]] = OrderedDict()
        self._max_size = max_size
        self._ttl = ttl_seconds
        self._hits = 0
        self._misses = 0

    def get(self, key: str) -> Optional[Any]:
        if key not in self._cache:
            self._misses += 1
            return None
        value, expires_at = self._cache[key]
        if time.monotonic() > expires_at:
            del self._cache[key]
            self._misses += 1
            return None
        self._cache.move_to_end(key)
        self._hits += 1
        return value

    def put(self, key: str, value: Any) -> None:
        if key in self._cache:
            self._cache.move_to_end(key)
        self._cache[key] = (value, time.monotonic() + self._ttl)
        while len(self._cache) > self._max_size:
            self._cache.popitem(last=False)

    def invalidate(self, prefix: str) -> int:
        keys_to_remove = [k for k in self._cache if k.startswith(prefix)]
        for k in keys_to_remove:
            del self._cache[k]
        return len(keys_to_remove)

    def stats(self) -> dict[str, Any]:
        total = self._hits + self._misses
        return {
            "size": len(self._cache),
            "max_size": self._max_size,
            "hits": self._hits,
            "misses": self._misses,
            "hit_rate": self._hits / total if total > 0 else 0.0,
        }


class TokenMeter:
    """Track token usage per tenant/department/agent with budget enforcement."""

    def __init__(self) -> None:
        self._lock = asyncio.Lock()
        self._daily_usage: dict[str, dict[str, float]] = {}
        self._monthly_usage: dict[str, dict[str, float]] = {}
        self._budgets: dict[str, BudgetConfig] = {}
        self._records: list[UsageRecord] = []
        self._record_limit = 10000

    def set_budget(self, scope_key: str, budget: BudgetConfig) -> None:
        self._budgets[scope_key] = budget

    async def record_usage(self, record: UsageRecord) -> None:
        async with self._lock:
            from datetime import datetime, timezone

            now = datetime.fromtimestamp(record.timestamp, tz=timezone.utc)
            date_key = now.strftime("%Y-%m-%d")
            month_key = now.strftime("%Y-%m")

            for scope_key in self._iter_scope_keys(record):
                if scope_key not in self._daily_usage:
                    self._daily_usage[scope_key] = {}
                if date_key not in self._daily_usage[scope_key]:
                    self._daily_usage[scope_key][date_key] = 0.0
                self._daily_usage[scope_key][date_key] += record.cost_usd

                if scope_key not in self._monthly_usage:
                    self._monthly_usage[scope_key] = {}
                if month_key not in self._monthly_usage[scope_key]:
                    self._monthly_usage[scope_key][month_key] = 0.0
                self._monthly_usage[scope_key][month_key] += record.cost_usd

            self._records.append(record)
            if len(self._records) > self._record_limit:
                self._records = self._records[-self._record_limit:]

    async def check_budget(
        self,
        tenant_id: str,
        agent_id: str = "",
    ) -> dict[str, Any]:
        async with self._lock:
            today = time.strftime("%Y-%m-%d", time.gmtime())
            scope_key = tenant_id

            budget = self._budgets.get(scope_key)
            if budget is None or not budget.enforce:
                return {"ok": True, "reason": "no_budget_set", "usage_ratio": 0.0}

            current_cost = self._daily_usage.get(scope_key, {}).get(today, 0.0)
            usage_ratio = current_cost / budget.daily_cost_limit if budget.daily_cost_limit > 0 else 0.0

            if usage_ratio >= 1.0:
                return {
                    "ok": False,
                    "reason": "daily_budget_exceeded",
                    "current_daily_cost": current_cost,
                    "daily_limit": budget.daily_cost_limit,
                    "usage_ratio": usage_ratio,
                }
            elif usage_ratio >= budget.alert_threshold:
                return {
                    "ok": True,
                    "reason": "approaching_limit",
                    "current_daily_cost": current_cost,
                    "daily_limit": budget.daily_cost_limit,
                    "usage_ratio": usage_ratio,
                    "alert": True,
                }
            else:
                return {
                    "ok": True,
                    "reason": "within_budget",
                    "current_daily_cost": current_cost,
                    "daily_limit": budget.daily_cost_limit,
                    "usage_ratio": usage_ratio,
                }

    def get_usage_report(
        self,
        tenant_id: str,
        days: int = 7,
    ) -> dict[str, Any]:
        today = time.strftime("%Y-%m-%d", time.gmtime())
        report = {"tenant_id": tenant_id, "period_days": days, "daily_breakdown": {}}

        scope_key = tenant_id
        budget = self._budgets.get(scope_key)
        report["budget"] = {
            "daily_cost_limit": budget.daily_cost_limit if budget else None,
            "monthly_cost_limit": budget.monthly_cost_limit if budget else None,
        }

        daily = self._daily_usage.get(scope_key, {})
        report["daily_breakdown"] = {
            date: {"cost_usd": round(cost, 4)}
            for date, cost in sorted(daily.items())
        }
        report["total_cost_usd"] = round(sum(daily.values()), 4)
        return report

    def _iter_scope_keys(self, record: UsageRecord) -> list[str]:
        keys = {record.tenant_id}
        if record.agent_id:
            keys.add(f"{record.tenant_id}:{record.agent_id}")
        return list(keys)


class LLMGateway:
    """Central LLM gateway with routing, metering, and caching.

    Usage:
        gateway = LLMGateway()
        await gateway.complete(prompt="Hello", model="qwen-max", tenant_id="t1")
    """

    def __init__(
        self,
        models: Optional[dict[str, ModelConfig]] = None,
        default_model: str = "qwen_max",
    ) -> None:
        self._models = models or self._load_default_models()
        self._default_model = default_model
        self._meter = TokenMeter()
        self._cache = PrefixCache(max_size=2000, ttl_seconds=600)
        self._routing_strategies: dict[str, Any] = {
            "round_robin": self._route_round_robin,
            "cost_optimized": self._route_cost_optimized,
            "quality_first": self._route_quality_first,
        }
        self._rr_index = 0

    def _load_default_models(self) -> dict[str, ModelConfig]:
        return {
            "qwen_max": ModelConfig(
                name="qwen_max",
                provider="qwen",
                base_url=os.getenv("QWEN_API_BASE", "https://dashscope.aliyuncs.com/compatible-mode/v1"),
                api_key=os.getenv("QWEN_API_KEY", ""),
                model_id="qwen-max",
                input_price_per_1k=0.008,
                output_price_per_1k=0.020,
            ),
            "qwen_plus": ModelConfig(
                name="qwen_plus",
                provider="qwen",
                base_url=os.getenv("QWEN_API_BASE", ""),
                api_key=os.getenv("QWEN_API_KEY", ""),
                model_id="qwen-plus",
                input_price_per_1k=0.004,
                output_price_per_1k=0.012,
            ),
            "deepseek": ModelConfig(
                name="deepseek",
                provider="deepseek",
                base_url=os.getenv("DEEPSEEK_API_BASE", "https://api.deepseek.com/v1"),
                api_key=os.getenv("DEEPSEEK_API_KEY", ""),
                model_id="deepseek-chat",
                input_price_per_1k=0.002,
                output_price_per_1k=0.008,
            ),
        }

    def set_budget(self, tenant_id: str, budget: BudgetConfig) -> None:
        self._meter.set_budget(tenant_id, budget)

    async def complete(
        self,
        prompt: str,
        model: Optional[str] = None,
        tenant_id: str = "",
        agent_id: str = "",
        trace_id: str = "",
        routing_strategy: str = "round_robin",
        skip_cache: bool = False,
    ) -> dict[str, Any]:
        if tenant_id:
            budget_check = await self._meter.check_budget(tenant_id, agent_id)
            if not budget_check["ok"]:
                logger.warning(
                    "LLM budget exceeded: tenant_id=%s agent_id=%s reason=%s current_cost=%.4f daily_limit=%.4f usage_ratio=%.2f",
                    tenant_id,
                    agent_id,
                    budget_check.get("reason", "unknown"),
                    budget_check.get("current_daily_cost", 0.0),
                    budget_check.get("daily_limit", 0.0),
                    budget_check.get("usage_ratio", 0.0),
                )
                return {
                    "content": "",
                    "model_used": "",
                    "usage": {},
                    "cost_usd": 0.0,
                    "error": budget_check["reason"],
                    "budget_exceeded": True,
                }

        model_name = model or self._route(routing_strategy, agent_id)
        model_config = self._models.get(model_name)
        if model_config is None or not model_config.enabled:
            return {
                "content": "",
                "model_used": model_name,
                "usage": {},
                "error": f"Model '{model_name}' not found or disabled",
            }

        cache_key = self._compute_cache_key(prompt, model_name)
        if not skip_cache:
            cached = self._cache.get(cache_key)
            if cached is not None:
                cached["cached"] = True
                return cached

        estimated_input_tokens = max(1, len(prompt) // 4)

        result = await self._generate_completion(prompt, model_config)

        input_tokens = result.get("usage", {}).get("prompt_tokens", estimated_input_tokens)
        output_tokens = result.get("usage", {}).get("completion_tokens", len(result.get("content", "")) // 4)
        cost = self._calculate_cost(input_tokens, output_tokens, model_config)

        record = UsageRecord(
            id=f"usage_{trace_id or time.time():.0f}",
            tenant_id=tenant_id or "default",
            agent_id=agent_id,
            model_name=model_name,
            input_tokens=input_tokens,
            output_tokens=output_tokens,
            cost_usd=cost,
            timestamp=time.time(),
            trace_id=trace_id,
        )
        await self._meter.record_usage(record)

        result["model_used"] = model_name
        result["cost_usd"] = cost
        result["cached"] = False
        self._cache.put(cache_key, result)

        return result

    def _route(self, strategy: str, agent_id: str) -> str:
        if strategy in self._routing_strategies:
            return self._routing_strategies[strategy](agent_id)
        return self._route_round_robin(agent_id)

    def _route_round_robin(self, agent_id: str) -> str:
        enabled = [name for name, cfg in self._models.items() if cfg.enabled]
        if not enabled:
            return self._default_model
        idx = self._rr_index % len(enabled)
        self._rr_index += 1
        return enabled[idx]

    def _route_cost_optimized(self, agent_id: str) -> str:
        enabled = [(name, cfg) for name, cfg in self._models.items() if cfg.enabled]
        if not enabled:
            return self._default_model
        enabled.sort(key=lambda x: x[1].input_price_per_1k + x[1].output_price_per_1k)
        return enabled[0][0]

    def _route_quality_first(self, agent_id: str) -> str:
        enabled = [(name, cfg) for name, cfg in self._models.items() if cfg.enabled]
        if not enabled:
            return self._default_model
        enabled.sort(key=lambda x: x[1].input_price_per_1k + x[1].output_price_per_1k, reverse=True)
        return enabled[0][0]

    async def _generate_completion(self, prompt: str, model_config: ModelConfig) -> dict[str, Any]:
        import httpx

        url = f"{model_config.base_url.rstrip('/')}/chat/completions"

        payload = {
            "model": model_config.model_id,
            "messages": [
                {"role": "system", "content": "You are a helpful assistant."},
                {"role": "user", "content": prompt},
            ],
            "max_tokens": min(model_config.max_tokens, 4096),
            "temperature": 0.7,
        }

        headers = {
            "Authorization": f"Bearer {model_config.api_key}",
            "Content-Type": "application/json",
        }

        try:
            async with httpx.AsyncClient(timeout=30.0) as client:
                resp = await client.post(url, json=payload, headers=headers)
                resp.raise_for_status()
                data = resp.json()

            choices = data.get("choices", [])
            content = choices[0]["message"]["content"] if choices else ""
            usage_data = data.get("usage", {})

            return {
                "content": content,
                "usage": {
                    "prompt_tokens": usage_data.get("prompt_tokens", 0),
                    "completion_tokens": usage_data.get("completion_tokens", 0),
                    "total_tokens": usage_data.get("total_tokens", 0),
                },
                "model_response": data,
            }
        except Exception as e:
            logger.error(f"LLM Gateway API call failed: {e}, model={model_config.model_id}")
            content = f"[FALLBACK-{model_config.model_id}] {prompt[:100]}..."
            return {
                "content": content,
                "usage": {
                    "prompt_tokens": len(prompt) // 4,
                    "completion_tokens": len(content) // 4,
                    "total_tokens": (len(prompt) + len(content)) // 4,
                },
                "fallback": True,
                "error": str(e),
            }

    def _calculate_cost(self, input_tokens: int, output_tokens: int, config: ModelConfig) -> float:
        input_cost = (input_tokens / 1000) * config.input_price_per_1k
        output_cost = (output_tokens / 1000) * config.output_price_per_1k
        return round(input_cost + output_cost, 6)

    def _compute_cache_key(self, prompt: str, model: str) -> str:
        prefix = prompt[:200]
        key_material = f"{model}:{prefix}"
        return hashlib.sha256(key_material.encode()).hexdigest()

    def get_model_stats(self) -> dict[str, Any]:
        return {
            name: {
                "enabled": cfg.enabled,
                "provider": cfg.provider,
                "input_price_per_1k": cfg.input_price_per_1k,
                "output_price_per_1k": cfg.output_price_per_1k,
            }
            for name, cfg in self._models.items()
        }

    def get_cache_stats(self) -> dict[str, Any]:
        return self._cache.stats()

    def clear_cache(self) -> int:
        """Clear all cached entries. Returns number cleared."""
        return self._cache.invalidate("")

    def get_usage_report(self, tenant_id: str, days: int = 7) -> dict[str, Any]:
        return self._meter.get_usage_report(tenant_id, days)
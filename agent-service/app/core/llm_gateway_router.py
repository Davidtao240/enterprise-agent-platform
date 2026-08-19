"""LLM Gateway Router — exposes LLM Gateway as HTTP endpoints.

Endpoints:
- POST /v1/llm/completions — Generate completion (with budget + cache)
- GET /v1/llm/models — List available models
- GET /v1/llm/usage/{tenant_id} — Get usage report
- POST /v1/llm/budgets/{tenant_id} — Set budget
- GET /v1/llm/cache/stats — Get cache statistics
- DELETE /v1/llm/cache — Clear cache
"""

from __future__ import annotations

import logging
import os
import secrets

from fastapi import APIRouter, Depends, HTTPException, Request, status
from pydantic import BaseModel, Field
from typing import Any, Optional

from app.core.llm_gateway import BudgetConfig, LLMGateway

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/v1/llm", tags=["llm-gateway"])

_gateway: LLMGateway | None = None


def get_gateway() -> LLMGateway:
    global _gateway
    if _gateway is None:
        _gateway = LLMGateway()
    return _gateway


async def require_internal_service(request: Request) -> None:
    expected = os.getenv("INTERNAL_SERVICE_TOKEN", "")
    provided = request.headers.get("X-Internal-Service-Token", "")
    if not expected:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail={"code": "SERVICE_AUTH_NOT_CONFIGURED", "message": "internal service authentication is not configured"},
        )
    if not secrets.compare_digest(provided, expected):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail={"code": "SERVICE_AUTH_FAILED", "message": "invalid internal service identity"},
        )


class CompletionRequest(BaseModel):
    prompt: str
    model: Optional[str] = None
    tenant_id: str = ""
    agent_id: str = ""
    trace_id: str = ""
    routing_strategy: str = "round_robin"
    skip_cache: bool = False


class CompletionResponse(BaseModel):
    content: str
    model_used: str
    usage: dict[str, Any] = {}
    cost_usd: float = 0.0
    cached: bool = False
    error: Optional[str] = None
    budget_exceeded: bool = False


class BudgetRequest(BaseModel):
    daily_token_limit: int = 1_000_000
    daily_cost_limit: float = 100.0
    monthly_token_limit: int = 30_000_000
    monthly_cost_limit: float = 3000.0
    alert_threshold: float = 0.8
    enforce: bool = True


@router.post("/completions", response_model=CompletionResponse, dependencies=[Depends(require_internal_service)])
async def create_completion(req: CompletionRequest) -> CompletionResponse:
    gateway = get_gateway()
    result = await gateway.complete(
        prompt=req.prompt,
        model=req.model,
        tenant_id=req.tenant_id,
        agent_id=req.agent_id,
        trace_id=req.trace_id,
        routing_strategy=req.routing_strategy,
        skip_cache=req.skip_cache,
    )
    return CompletionResponse(**result)


@router.get("/models", dependencies=[Depends(require_internal_service)])
async def list_models() -> dict[str, Any]:
    gateway = get_gateway()
    return gateway.get_model_stats()


@router.get("/usage/{tenant_id}", dependencies=[Depends(require_internal_service)])
async def get_usage(tenant_id: str, days: int = 7) -> dict[str, Any]:
    gateway = get_gateway()
    return gateway.get_usage_report(tenant_id, days)


@router.post("/budgets/{tenant_id}", dependencies=[Depends(require_internal_service)])
async def set_budget(tenant_id: str, req: BudgetRequest) -> dict[str, Any]:
    gateway = get_gateway()
    gateway.set_budget(tenant_id, BudgetConfig(
        daily_token_limit=req.daily_token_limit,
        daily_cost_limit=req.daily_cost_limit,
        monthly_token_limit=req.monthly_token_limit,
        monthly_cost_limit=req.monthly_cost_limit,
        alert_threshold=req.alert_threshold,
        enforce=req.enforce,
    ))
    return {"status": "ok", "tenant_id": tenant_id, "budget": req.model_dump()}


@router.get("/cache/stats", dependencies=[Depends(require_internal_service)])
async def cache_stats() -> dict[str, Any]:
    gateway = get_gateway()
    return gateway.get_cache_stats()


@router.delete("/cache", dependencies=[Depends(require_internal_service)])
async def clear_cache() -> dict[str, Any]:
    gateway = get_gateway()
    count = gateway.clear_cache()
    return {"cleared_entries": count}
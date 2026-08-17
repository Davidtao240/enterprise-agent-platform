from __future__ import annotations

from datetime import datetime
from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field


class StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class GraphIdentity(StrictModel):
    key: str = Field(min_length=1, max_length=128)
    version: str = Field(min_length=1, max_length=32)


class RuntimeConfiguration(StrictModel):
    agent_definition_version: str = Field(min_length=1, max_length=128)
    profile_or_skill_version: str = Field(min_length=1, max_length=128)
    model_config_version: str = Field(min_length=1, max_length=128)


class TrustedContext(StrictModel):
    user_id: str = Field(min_length=1, max_length=128)
    tenant_id: str = Field(min_length=1, max_length=128)
    department_id: str | None = Field(default=None, max_length=128)


class RuntimeBudget(StrictModel):
    deadline_at: datetime | None = None
    max_steps: int = Field(default=30, ge=1, le=10000)
    max_cost: float = Field(default=1.0, ge=0)


class StartRunRequest(StrictModel):
    protocol_version: Literal["2.0"]
    run_id: str = Field(min_length=1, max_length=128)
    thread_id: str = Field(min_length=1, max_length=128)
    trace_id: str = Field(min_length=1, max_length=128)
    workflow_instance_id: str | None = Field(default=None, max_length=128)
    node_instance_id: str | None = Field(default=None, max_length=128)
    business_app_code: str = Field(min_length=1, max_length=64)
    graph: GraphIdentity
    configuration: RuntimeConfiguration
    input: dict[str, Any] = Field(default_factory=dict)
    trusted_context: TrustedContext
    budget: RuntimeBudget = Field(default_factory=RuntimeBudget)
    attempt: int = Field(default=1, ge=1)
    idempotency_key: str | None = Field(default=None, min_length=1, max_length=255)


class ResumeRunRequest(StrictModel):
    protocol_version: Literal["2.0"]
    run_id: str = Field(min_length=1, max_length=128)
    interrupt_id: str = Field(min_length=1, max_length=128)
    expected_checkpoint_version: int = Field(ge=1)
    idempotency_key: str = Field(min_length=1, max_length=255)
    resume_input: dict[str, Any]


class CancelRunRequest(StrictModel):
    protocol_version: Literal["2.0"]
    run_id: str = Field(min_length=1, max_length=128)
    reason: str = Field(min_length=1, max_length=255)
    requested_by: str = Field(min_length=1, max_length=128)
    idempotency_key: str = Field(min_length=1, max_length=255)


class AcceptedRunResponse(StrictModel):
    protocol_version: Literal["2.0"] = "2.0"
    run_id: str
    status: str
    accepted_at: datetime
    checkpoint_version: int = 0
    replayed: bool = False


class RuntimeEventEnvelope(StrictModel):
    protocol_version: Literal["2.0"] = "2.0"
    event_id: str
    tenant_id: str
    run_id: str
    sequence: int = Field(ge=1)
    attempt: int = Field(ge=1)
    type: str
    occurred_at: datetime
    payload: dict[str, Any] = Field(default_factory=dict)
    checkpoint_version: int | None = Field(default=None, ge=1)

"""SupervisorAgent: multi-agent orchestration using LangGraph supervisor pattern.

The supervisor decomposes a complex task into sub-tasks, dispatches them to
specialist agents, and aggregates their outputs. This enables:
- Task decomposition and delegation
- Parallel agent execution where possible
- Sequential dependency management
- Result aggregation and synthesis
"""

from __future__ import annotations

import json
import logging
from typing import Any, Literal, Optional

from app.agents.base import BaseAgent

logger = logging.getLogger(__name__)


class SupervisorAgent(BaseAgent):
    """Orchestrates multiple sub-agents to complete complex tasks.

    The supervisor uses an LLM to:
    1. Analyze the user's request
    2. Plan which sub-agents to invoke and in what order
    3. Dispatch tasks to sub-agents
    4. Synthesize results into a coherent output
    """

    agent_id = "supervisor_agent"
    domain = "shared"
    reusable_scope = "shared"

    def __init__(self, llm_client: Any = None) -> None:
        self.llm_client = llm_client

    async def run(self, state: dict[str, Any]) -> dict[str, Any]:
        """Run the supervisor orchestration."""
        sub_tasks = state.get("sub_tasks", [])
        if not sub_tasks:
            return self._plan_subtasks(state)
        return self._aggregate_results(state)

    def _plan_subtasks(self, state: dict[str, Any]) -> dict[str, Any]:
        """Use LLM to decompose the user request into sub-tasks."""
        user_request = state.get("user_request", "")
        available_agents = state.get("available_agents", [])

        # Default planning logic (when no LLM client available)
        plan = {
            "sub_tasks": [
                {"task": "data_collection", "agent": "data_collection_agent", "status": "pending"},
                {"task": "analysis", "agent": "analysis_agent", "status": "pending"},
                {"task": "compliance_check", "agent": "compliance_agent", "status": "pending"},
                {"task": "synthesis", "agent": "supervisor_agent", "status": "pending"},
            ],
            "plan_summary": "Collect data → Analyze → Check compliance → Synthesize report",
        }
        return plan

    def _aggregate_results(self, state: dict[str, Any]) -> dict[str, Any]:
        """Aggregate results from completed sub-tasks."""
        results = state.get("sub_results", {})
        errors = state.get("errors", [])

        if errors:
            return {
                "aggregated_result": {
                    "status": "partial",
                    "results": results,
                    "errors": errors,
                    "summary": f"Completed {len(results)} tasks with {len(errors)} errors",
                }
            }

        return {
            "aggregated_result": {
                "status": "complete",
                "results": results,
                "summary": "All tasks completed successfully",
            }
        }
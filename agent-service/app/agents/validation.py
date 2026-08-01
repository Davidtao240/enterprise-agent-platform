"""ValidationAgent: execute deterministic rules from a domain profile.

The shared agent owns orchestration and result shape; the versioned profile
owns field names, rules, and domain-facing messages.
"""

import logging
from typing import Any

from app.agents.base import BaseAgent
from app.profiles.contracts import ValidationProfile

logger = logging.getLogger(__name__)


class ValidationAgent(BaseAgent):
    agent_id = "validation_agent"
    domain = "shared"
    reusable_scope = "shared"

    def __init__(self, profile: ValidationProfile) -> None:
        self.profile = profile

    async def run(self, state: dict[str, Any]) -> dict[str, Any]:
        mapped_data = state.get("mapped_data") or {}
        mapped_rows = mapped_data.get("mapped_rows", [])
        errors, warnings = self.profile.validate_rows(mapped_rows)

        # Carry over data-loading warnings
        load_warnings = state.get("_load_warnings") or []
        warnings = load_warnings + warnings

        valid = len(errors) == 0
        state["validation_result"] = {
            "valid": valid,
            "errors": errors,
            "warnings": warnings,
        }

        logger.info("ValidationAgent: valid=%s, %d errors, %d warnings", valid, len(errors), len(warnings))
        return state

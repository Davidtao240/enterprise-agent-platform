"""SchemaMappingAgent: map uploaded columns using an injected domain profile.

Hybrid approach: rule-based alias matching first, LLM fallback for unmapped
columns. The agent is domain-neutral; the explicit graph selects the profile.
"""

from __future__ import annotations

import logging
from typing import Any

from langchain_core.messages import HumanMessage
from pydantic import BaseModel, Field

from app.agents.base import BaseAgent
from app.core.llm import get_llm
from app.core.prompt_safety import sanitize_user_input
from app.core.usage_tracker import UsageTracker
from app.profiles.contracts import SchemaMappingProfile

logger = logging.getLogger(__name__)


class SchemaMappingOutput(BaseModel):
    mappings: dict[str, str] = Field(description="Mapping of field names to canonical field names")


def _map_columns(
    columns: list[str],
    profile: SchemaMappingProfile,
) -> tuple[dict[str, str], list[str]]:
    """Map column names to canonical fields. Returns (mapping, unmapped_list)."""
    mapping: dict[str, str] = {}
    unmapped: list[str] = []
    canonical_fields = profile.canonical_fields
    canonical_by_lower = {field.lower(): field for field in canonical_fields}
    aliases_by_lower = {
        alias.lower().strip(): canonical
        for alias, canonical in profile.aliases.items()
    }

    for col in columns:
        col_stripped = col.strip()
        # 1. Exact match
        if col_stripped in canonical_fields:
            mapping[col_stripped] = col_stripped
            continue
        # 2. Lowercase match
        lowercase_match = canonical_by_lower.get(col_stripped.lower())
        if lowercase_match:
            mapping[col_stripped] = lowercase_match
            continue
        # 3. Alias lookup (case-insensitive)
        alias_lower = col_stripped.lower().strip()
        alias_match = aliases_by_lower.get(alias_lower)
        if alias_match:
            mapping[col_stripped] = alias_match
        else:
            unmapped.append(col_stripped)

    return mapping, unmapped


class SchemaMappingAgent(BaseAgent):
    agent_id = "schema_mapping_agent"
    domain = "shared"
    reusable_scope = "shared"

    def __init__(self, profile: SchemaMappingProfile) -> None:
        self.profile = profile

    async def run(self, state: dict[str, Any]) -> dict[str, Any]:
        raw_data = state.get("raw_data") or {}
        columns = raw_data.get("columns", [])
        rows = raw_data.get("rows", [])

        if not columns or not rows:
            state["error"] = {
                "code": "SCHEMA_MAPPING_FAILED",
                "message": "No data to map — raw_data is empty.",
                "retryable": False,
            }
            return state

        # Phase 1: rule-based mapping
        field_mapping, unmapped = _map_columns(columns, self.profile)
        warnings: list[dict[str, Any]] = []

        # Phase 2: LLM fallback for unmapped columns
        if unmapped:
            try:
                llm_mapping = await self._llm_map(unmapped)
                field_mapping.update(llm_mapping)
            except Exception as e:
                logger.warning("LLM schema mapping failed: %s, using rule-only mapping", e)
                for col in unmapped:
                    warnings.append({
                        "level": "low",
                        "message": f"Column '{col}' could not be mapped to a canonical field.",
                    })

        # Build mapped rows
        mapped_rows = []
        for row in rows:
            mapped_row: dict[str, Any] = {}
            for orig_col, value in row.items():
                canonical = field_mapping.get(orig_col.strip())
                if canonical:
                    mapped_row[canonical] = value
            mapped_rows.append(mapped_row)

        state["mapped_data"] = {
            "mapped_rows": mapped_rows,
            "field_mapping": field_mapping,
            "warnings": warnings,
        }

        logger.info("SchemaMappingAgent: mapped %d columns, %d unmapped", len(field_mapping), len(unmapped))
        return state

    async def _llm_map(self, unmapped: list[str]) -> dict[str, str]:
        """Use LLM to suggest canonical field mappings for unrecognized column names."""
        llm = get_llm(temperature=0.0)
        safe_unmapped = sanitize_user_input(str(unmapped))
        prompt = self.profile.build_prompt([safe_unmapped])
        structured_llm = llm.with_structured_output(SchemaMappingOutput)
        response = await structured_llm.ainvoke([HumanMessage(content=prompt)])

        return {
            key: value
            for key, value in response.mappings.items()
            if value is not None and value in self.profile.canonical_fields
        }

    def _track_usage(self, state: dict[str, Any] | None, response_metadata: dict[str, Any]) -> None:
        """Update the usage tracker from LLM response metadata."""
        # Usage tracking is handled at the graph level via state
        token_usage = response_metadata.get("token_usage", {})
        if token_usage:
            tracker = UsageTracker()
            tracker.add(token_usage)
            self._last_usage = tracker.to_dict()

"""SchemaMappingAgent: map uploaded column names to canonical finance schema.

Hybrid approach: rule-based alias matching first, LLM fallback for unmapped columns.
"""

from __future__ import annotations

import json
import logging
from typing import Any

from langchain_core.messages import HumanMessage

from app.agents.base import BaseAgent
from app.core.llm import get_llm
from app.core.usage_tracker import UsageTracker

logger = logging.getLogger(__name__)

CANONICAL_FIELDS = [
    "month", "department", "revenue", "cost",
    "gross_profit", "net_profit", "customer_count", "order_count",
]

ALIAS_MAP: dict[str, str] = {
    # Chinese → canonical
    "收入": "revenue", "营业收入": "revenue",
    "成本": "cost", "营业成本": "cost",
    "净利润": "net_profit", "净利": "net_profit",
    "毛利": "gross_profit", "毛利润": "gross_profit",
    "客户数": "customer_count", "客户数量": "customer_count",
    "订单数": "order_count", "订单数量": "order_count",
    "月份": "month", "日期": "month", "时间": "month",
    "部门": "department", "事业部": "department",
    # English → canonical
    "income": "revenue", "sales": "revenue", "turnover": "revenue",
    "expense": "cost", "expenses": "cost", "operating cost": "cost",
    "profit": "net_profit", "net income": "net_profit", "net": "net_profit",
    "gross income": "gross_profit", "gross": "gross_profit", "gross margin": "gross_profit",
    "dept": "department", "division": "department",
    "cust": "customer_count", "customers": "customer_count",
    "orders": "order_count", "order volume": "order_count",
}


def _map_columns(columns: list[str]) -> tuple[dict[str, str], list[str]]:
    """Map column names to canonical fields. Returns (mapping, unmapped_list)."""
    mapping: dict[str, str] = {}
    unmapped: list[str] = []

    for col in columns:
        col_stripped = col.strip()
        # 1. Exact match
        if col_stripped in CANONICAL_FIELDS:
            mapping[col_stripped] = col_stripped
            continue
        # 2. Lowercase match
        if col_stripped.lower() in [f.lower() for f in CANONICAL_FIELDS]:
            match = next(f for f in CANONICAL_FIELDS if f.lower() == col_stripped.lower())
            mapping[col_stripped] = match
            continue
        # 3. Alias lookup (case-insensitive)
        alias_lower = col_stripped.lower().strip()
        matched = False
        for alias, canonical in ALIAS_MAP.items():
            if alias_lower == alias.lower():
                mapping[col_stripped] = canonical
                matched = True
                break
        if not matched:
            unmapped.append(col_stripped)

    return mapping, unmapped


class SchemaMappingAgent(BaseAgent):
    agent_id = "schema_mapping_agent"
    domain = "finance"

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
        field_mapping, unmapped = _map_columns(columns)
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
        prompt = f"""Map these column names to the canonical finance schema fields.

Canonical fields: {json.dumps(CANONICAL_FIELDS)}

Unmapped columns: {json.dumps(unmapped)}

Return ONLY a JSON object mapping each column to a canonical field (or null if no match).
Example: {{"sales_revenue": "revenue", "op_cost": "cost", "notes": null}}
"""
        response = await llm.ainvoke([HumanMessage(content=prompt)])
        self._track_usage(state=None, response_metadata=response.response_metadata)

        text = response.content.strip()
        if isinstance(text, str):
            text = text.removeprefix("```json").removesuffix("```").strip()
        result = json.loads(text)

        return {k: v for k, v in result.items() if v is not None and v in CANONICAL_FIELDS}

    def _track_usage(self, state: dict[str, Any] | None, response_metadata: dict[str, Any]) -> None:
        """Update the usage tracker from LLM response metadata."""
        # Usage tracking is handled at the graph level via state
        token_usage = response_metadata.get("token_usage", {})
        if token_usage:
            tracker = UsageTracker()
            tracker.add(token_usage)
            self._last_usage = tracker.to_dict()

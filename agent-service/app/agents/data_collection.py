"""DataCollectionAgent: gathers financial data from various sources.

Specializes in:
- Reading financial spreadsheets
- Extracting data from ERP exports
- Standardizing data formats for analysis
"""

from __future__ import annotations

import logging
from typing import Any

from app.agents.base import BaseAgent

logger = logging.getLogger(__name__)


class DataCollectionAgent(BaseAgent):
    agent_id = "data_collection_agent"
    domain = "finance"
    reusable_scope = "finance"

    async def run(self, state: dict[str, Any]) -> dict[str, Any]:
        user_request = state.get("user_request", "")
        file_id = state.get("file_id")

        collected = self._collect_data(state)
        result = {
            "task": "data_collection",
            "status": "completed",
            "data": collected,
            "source": file_id or "inline",
            "record_count": len(collected),
        }
        return {"sub_results": {**state.get("sub_results", {}), "data_collection": result}}

    def _collect_data(self, state: dict[str, Any]) -> list[dict[str, Any]]:
        raw_data = state.get("raw_data", {})
        if not raw_data:
            return []

        records = raw_data.get("rows", [])
        return records[:50]
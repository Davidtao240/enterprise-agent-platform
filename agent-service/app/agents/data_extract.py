"""DataExtractAgent: parse CSV/Excel files into normalized tabular data.

Uses pandas for file parsing with three-tier fallback (MinIO → inline → sample data).
No LLM involved — purely rule-based.
"""

import logging
from typing import Any

from app.agents.base import BaseAgent
from app.core.file_utils import load_data

logger = logging.getLogger(__name__)


class DataExtractAgent(BaseAgent):
    agent_id = "data_extract_agent"
    domain = "finance"

    async def run(self, state: dict[str, Any]) -> dict[str, Any]:
        file_id = state.get("file_id")
        inline_data = state.get("raw_data")
        if isinstance(inline_data, dict):
            # Handle Go's {"workflow_input": ...} wrapping
            inline_data = inline_data.get("rows") or inline_data.get("workflow_input")

        try:
            columns, rows, warnings = await load_data(
                file_id=file_id,
                inline_data=inline_data if isinstance(inline_data, list) else None,
            )
        except Exception as e:
            logger.exception("DataExtractAgent failed")
            state["error"] = {
                "code": "FILE_PARSE_FAILED",
                "message": str(e),
                "retryable": True,
            }
            return state

        state["raw_data"] = {"columns": columns, "rows": rows}

        # Append file-loading warnings to any existing warnings
        if warnings:
            existing = state.get("validation_result", {}).get("warnings", []) if state.get("validation_result") else []
            state["_load_warnings"] = [
                {"level": "info", "message": w} for w in warnings
            ]

        logger.info("DataExtractAgent: extracted %d rows, %d columns", len(rows), len(columns))
        return state

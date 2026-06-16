"""ReportAgent: generate structured finance operating report JSON.

LLM-based with a template fallback for when the LLM is unavailable.
"""

import json
import logging
from datetime import datetime, timezone
from typing import Any

from langchain_core.messages import HumanMessage

from app.agents.base import BaseAgent
from app.core.llm import get_llm

logger = logging.getLogger(__name__)


def _build_fallback_report(
    mapped_data: dict[str, Any],
    metrics: dict[str, Any],
    warnings: list[dict[str, Any]],
) -> dict[str, Any]:
    """Generate a minimal report from calculated metrics when LLM is unavailable."""
    rows = mapped_data.get("mapped_rows", [])
    periods = sorted({str(r.get("month", "")) for r in rows if r.get("month")})
    departments = sorted({str(r.get("department", "")) for r in rows if r.get("department")})

    period_str = ", ".join(periods) if periods else "N/A"
    dept_str = ", ".join(departments) if departments else "N/A"

    m = metrics
    return {
        "title": f"Finance Operating Report — {period_str}",
        "period": period_str,
        "department": dept_str,
        "sections": [
            {
                "key": "executive_summary",
                "title": "Executive Summary",
                "content": (
                    f"This report covers {m.get('row_count', 0)} departments for period {period_str}. "
                    f"Total revenue: {m.get('revenue', 0):,.0f}. "
                    f"Net profit: {m.get('net_profit', 0):,.0f}."
                ),
            },
            {
                "key": "revenue_analysis",
                "title": "Revenue Analysis",
                "content": f"Total revenue: {m.get('revenue', 0):,.0f}.",
            },
            {
                "key": "cost_analysis",
                "title": "Cost Analysis",
                "content": f"Total cost: {m.get('cost', 0):,.0f}.",
            },
            {
                "key": "profit_analysis",
                "title": "Profit Analysis",
                "content": (
                    f"Gross profit: {m.get('gross_profit', 0):,.0f} "
                    f"(margin: {m.get('gross_margin', 0):.1%}). "
                    f"Net profit: {m.get('net_profit', 0):,.0f} "
                    f"(margin: {m.get('net_margin', 0):.1%})."
                ),
            },
        ],
        "warnings": warnings,
        "recommendations": ["Review cost trends and margin pressure."],
        "review": {"status": "pending", "reviewer": None, "comment": None, "reviewed_at": None},
    }


class ReportAgent(BaseAgent):
    agent_id = "report_agent"
    domain = "finance"

    async def run(self, state: dict[str, Any]) -> dict[str, Any]:
        analysis = state.get("analysis_result") or {}
        mapped_data = state.get("mapped_data") or {}
        validation = state.get("validation_result") or {}

        key_metrics = analysis.get("key_metrics", {})
        analysis_narrative = analysis.get("analysis", {})
        warnings: list[dict[str, Any]] = []
        warnings.extend(analysis.get("warnings", []))
        warnings.extend(validation.get("warnings", []))

        try:
            report = await self._llm_generate(key_metrics, analysis_narrative, mapped_data, warnings)
        except Exception as e:
            logger.warning("LLM report generation failed, using fallback: %s", e)
            report = _build_fallback_report(mapped_data, key_metrics, warnings)
            report["warnings"].append({
                "level": "info",
                "message": "AI-generated report unavailable — showing template-based report.",
            })

        state["report"] = {
            "report": report,
            "result_file_id": None,  # V1: file generation deferred
        }

        logger.info("ReportAgent: report title='%s'", report.get("title", ""))
        return state

    async def _llm_generate(
        self,
        metrics: dict[str, Any],
        narrative: dict[str, Any],
        mapped_data: dict[str, Any],
        warnings: list[dict[str, Any]],
    ) -> dict[str, Any]:
        llm = get_llm(temperature=0.3)
        rows = mapped_data.get("mapped_rows", [])
        periods = sorted({str(r.get("month", "")) for r in rows if r.get("month")})
        departments = sorted({str(r.get("department", "")) for r in rows if r.get("department")})

        prompt = f"""Generate a structured finance operating report as JSON.

Period: {', '.join(periods) if periods else 'N/A'}
Departments: {', '.join(departments) if departments else 'N/A'}
Key Metrics: {json.dumps(metrics)}
Analysis: {json.dumps(narrative)}
Warnings: {json.dumps(warnings, default=str)}

Return a JSON object with these keys:
- title: The report title (string)
- period: The reporting period (string)
- department: Department name(s) (string)
- sections: Array of {{key, title, content}} objects. Must include: executive_summary, revenue_analysis, cost_analysis, profit_analysis.
- warnings: Array of warning objects
- recommendations: Array of 2-4 actionable recommendation strings
- review: {{status: "pending", reviewer: null, comment: null, reviewed_at: null}}

Each content field should be 2-5 sentences in professional English. Return ONLY valid JSON.
"""
        response = await llm.ainvoke([HumanMessage(content=prompt)])
        text = response.content.strip()
        if isinstance(text, str):
            text = text.removeprefix("```json").removesuffix("```").strip()
        result = json.loads(text)
        result.setdefault("review", {"status": "pending", "reviewer": None, "comment": None, "reviewed_at": None})
        return result

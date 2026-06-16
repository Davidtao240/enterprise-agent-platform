"""ReviewSummaryAgent: prepare a reviewer-facing summary for human approval.

LLM-based with fallback to programmatic summary from key_metrics and warnings.
"""

import json
import logging
from typing import Any

from langchain_core.messages import HumanMessage

from app.agents.base import BaseAgent
from app.core.llm import get_llm

logger = logging.getLogger(__name__)


def _build_fallback_summary(
    key_metrics: dict[str, Any],
    analysis: dict[str, Any],
    warnings: list[dict[str, Any]],
) -> dict[str, Any]:
    """Generate a programmatic review summary when LLM is unavailable."""
    m = key_metrics
    revenue = m.get("revenue", 0)
    net_profit = m.get("net_profit", 0)
    net_margin = m.get("net_margin", 0)

    summary_parts = [f"Total revenue: {revenue:,.0f}. Net profit: {net_profit:,.0f} (margin: {net_margin:.1%})."]

    high_warnings = [w for w in warnings if w.get("level") == "high"]
    med_warnings = [w for w in warnings if w.get("level") == "medium"]

    warning_msgs = [w.get("message", "") for w in high_warnings + med_warnings]
    suggestions = [
        "Review high-priority warnings and verify data accuracy.",
        "Confirm all required approvals are in place.",
        "Check for unusual trends across departments.",
    ]

    return {
        "summary": " ".join(summary_parts),
        "warnings": warning_msgs,
        "review_suggestions": suggestions,
    }


class ReviewSummaryAgent(BaseAgent):
    agent_id = "review_summary_agent"
    domain = "finance"

    async def run(self, state: dict[str, Any]) -> dict[str, Any]:
        analysis = state.get("analysis_result") or {}
        validation = state.get("validation_result") or {}
        report = state.get("report") or {}

        key_metrics = analysis.get("key_metrics", {})
        analysis_narrative = analysis.get("analysis", {})
        warnings: list[dict[str, Any]] = []
        warnings.extend(validation.get("warnings", []))
        warnings.extend(analysis.get("warnings", []))
        warnings.extend((report.get("report") or {}).get("warnings", []))

        try:
            summary = await self._llm_summarize(key_metrics, analysis_narrative, warnings)
        except Exception as e:
            logger.warning("LLM review summary failed, using fallback: %s", e)
            summary = _build_fallback_summary(key_metrics, analysis_narrative, warnings)

        # Deduplicate warnings
        seen = set()
        deduped: list[str] = []
        for w in summary.get("warnings", []):
            msg = str(w) if isinstance(w, str) else w.get("message", str(w))
            if msg not in seen:
                seen.add(msg)
                deduped.append(msg)
        summary["warnings"] = deduped

        state["review_summary"] = summary
        logger.info("ReviewSummaryAgent: summary ready (%d warnings)", len(deduped))
        return state

    async def _llm_summarize(
        self,
        key_metrics: dict[str, Any],
        analysis: dict[str, Any],
        warnings: list[dict[str, Any]],
    ) -> dict[str, Any]:
        llm = get_llm(temperature=0.2)

        high_warnings = [w for w in warnings if w.get("level") in ("high", "medium")]

        prompt = f"""You are a finance reviewer. Summarize the following for a human finance manager who needs to approve this report.

Key Metrics: {json.dumps(key_metrics)}
Analysis: {json.dumps(analysis)}
Critical Warnings: {json.dumps(high_warnings, default=str)}

Return a JSON object with:
- summary: 2-4 sentence executive summary with key numbers
- warnings: Array of warning strings that need attention
- review_suggestions: Array of 2-4 specific questions or items the reviewer should check

Be concise and business-focused. Return ONLY valid JSON.
"""
        response = await llm.ainvoke([HumanMessage(content=prompt)])
        text = response.content.strip()
        if isinstance(text, str):
            text = text.removeprefix("```json").removesuffix("```").strip()
        return json.loads(text)

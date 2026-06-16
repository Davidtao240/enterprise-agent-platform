"""FinanceAnalysisAgent: calculate key financial metrics and generate narrative analysis.

Hybrid: rule-based metric calculation + LLM narrative generation with fallback template.
"""

import json
import logging
from typing import Any

from langchain_core.messages import HumanMessage

from app.agents.base import BaseAgent
from app.core.llm import get_llm

logger = logging.getLogger(__name__)


def _calculate_metrics(rows: list[dict[str, Any]]) -> dict[str, Any]:
    """Aggregate financial metrics from all rows. Pure calculation, no LLM."""
    total_revenue = 0.0
    total_cost = 0.0
    total_gross_profit = 0.0
    total_net_profit = 0.0

    for row in rows:
        try:
            total_revenue += float(row.get("revenue", 0) or 0)
            total_cost += float(row.get("cost", 0) or 0)
            total_gross_profit += float(row.get("gross_profit", 0) or 0)
            total_net_profit += float(row.get("net_profit", 0) or 0)
        except (ValueError, TypeError):
            continue

    gross_margin = round(total_gross_profit / total_revenue, 4) if total_revenue > 0 else 0.0
    net_margin = round(total_net_profit / total_revenue, 4) if total_revenue > 0 else 0.0

    return {
        "revenue": round(total_revenue, 2),
        "cost": round(total_cost, 2),
        "gross_profit": round(total_gross_profit, 2),
        "net_profit": round(total_net_profit, 2),
        "gross_margin": gross_margin,
        "net_margin": net_margin,
        "row_count": len(rows),
    }


def _build_fallback_analysis(metrics: dict[str, Any]) -> dict[str, Any]:
    """Generate a template-based analysis when LLM is unavailable."""
    r, c, gp, np_val = metrics["revenue"], metrics["cost"], metrics["gross_profit"], metrics["net_profit"]
    gm, nm = metrics["gross_margin"], metrics["net_margin"]

    return {
        "revenue_summary": f"Total revenue across {metrics['row_count']} departments is {r:,.0f}.",
        "cost_summary": f"Total cost is {c:,.0f}, representing {c/r*100:.1f}% of revenue." if r > 0 else "Cost data unavailable.",
        "profit_summary": f"Gross profit is {gp:,.0f} (margin {gm:.1%}). Net profit is {np_val:,.0f} (margin {nm:.1%}).",
        "risk_summary": (
            "Cost exceeds revenue — immediate review required."
            if c > r and r > 0
            else "Net profit is negative — margin pressure detected."
            if np_val < 0
            else "No critical risks detected from financial metrics."
        ),
    }


class FinanceAnalysisAgent(BaseAgent):
    agent_id = "finance_analysis_agent"
    domain = "finance"

    async def run(self, state: dict[str, Any]) -> dict[str, Any]:
        mapped_data = state.get("mapped_data") or {}
        mapped_rows = mapped_data.get("mapped_rows", [])
        validation = state.get("validation_result") or {}

        if not mapped_rows:
            state["error"] = {
                "code": "FINANCE_ANALYSIS_FAILED",
                "message": "No mapped data available for analysis.",
                "retryable": False,
            }
            return state

        # 1. Calculate metrics (rule-based)
        key_metrics = _calculate_metrics(mapped_rows)

        # 2. Generate narrative (LLM with fallback)
        analysis_warnings: list[dict[str, Any]] = list(validation.get("warnings", []))

        try:
            narrative = await self._llm_analyze(key_metrics, mapped_rows)
        except Exception as e:
            logger.warning("LLM analysis failed, using fallback template: %s", e)
            narrative = _build_fallback_analysis(key_metrics)
            analysis_warnings.append({
                "level": "info",
                "message": "AI analysis unavailable — showing calculated metrics only.",
            })

        state["analysis_result"] = {
            "key_metrics": key_metrics,
            "analysis": narrative,
            "warnings": analysis_warnings,
        }

        logger.info("FinanceAnalysisAgent: revenue=%s, net_profit=%s", key_metrics["revenue"], key_metrics["net_profit"])
        return state

    async def _llm_analyze(self, metrics: dict[str, Any], rows: list[dict[str, Any]]) -> dict[str, Any]:
        llm = get_llm(temperature=0.2)
        prompt = f"""Analyze the following financial data and produce a JSON object with these keys: revenue_summary, cost_summary, profit_summary, risk_summary.

Each value should be 2-4 sentences in English.

Key Metrics: {json.dumps(metrics)}

Department-level data: {json.dumps(rows, default=str)}

Return ONLY valid JSON, no markdown formatting.
"""
        response = await llm.ainvoke([HumanMessage(content=prompt)])
        text = response.content.strip()
        if isinstance(text, str):
            text = text.removeprefix("```json").removesuffix("```").strip()
        return json.loads(text)

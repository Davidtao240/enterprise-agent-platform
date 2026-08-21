"""FinanceAnalysisAgent: calculate key financial metrics and generate narrative analysis.

Hybrid: rule-based metric calculation + LLM narrative generation with fallback template.
"""

import json
import logging
from typing import Any

from langchain_core.messages import HumanMessage
from pydantic import BaseModel, Field

from app.agents.base import BaseAgent
from app.core.llm import get_llm
from app.core.prompt_safety import sanitize_user_input

logger = logging.getLogger(__name__)


class AnalysisNarrative(BaseModel):
    revenue_summary: str = Field(description="Summary of revenue analysis")
    cost_summary: str = Field(description="Summary of cost analysis")
    profit_summary: str = Field(description="Summary of profit analysis")
    risk_summary: str = Field(description="Summary of risk analysis")


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
        "revenue_summary": f"{metrics['row_count']} 个部门的营业收入合计为 {r:,.0f}。",
        "cost_summary": f"成本合计为 {c:,.0f}，占营业收入的 {c/r*100:.1f}%。" if r > 0 else "暂无可用的成本占比数据。",
        "profit_summary": f"毛利润为 {gp:,.0f}（毛利率 {gm:.1%}），净利润为 {np_val:,.0f}（净利率 {nm:.1%}）。",
        "risk_summary": (
            "成本超过营业收入，需要立即复核成本数据和业务原因。"
            if c > r and r > 0
            else "净利润为负，存在明显利润率压力。"
            if np_val < 0
            else "根据当前财务指标，暂未发现重大风险。"
        ),
    }


class FinanceAnalysisAgent(BaseAgent):
    agent_id = "finance_analysis_agent"
    domain = "finance"
    reusable_scope = "domain_only"

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
                "message": "AI 深度分析暂不可用，当前展示基于财务数据计算得到的指标。",
            })

        state["analysis_result"] = {
            "key_metrics": key_metrics,
            "analysis": narrative,
            "warnings": analysis_warnings,
        }

        logger.info("FinanceAnalysisAgent: revenue=%s, net_profit=%s", key_metrics["revenue"], key_metrics["net_profit"])
        return state

    async def _llm_analyze(self, metrics: dict[str, Any], rows: list[dict[str, Any]]) -> dict[str, Any]:
        llm = get_llm(temperature=0.0)
        safe_metrics = sanitize_user_input(json.dumps(metrics, ensure_ascii=False))
        safe_rows = sanitize_user_input(json.dumps(rows, default=str, ensure_ascii=False))
        prompt = f"""请分析以下财务数据，并生成专业的分析报告。

关键指标：{safe_metrics}

部门明细：{safe_rows}

请使用结构化方式返回分析结果。"""
        structured_llm = llm.with_structured_output(AnalysisNarrative)
        response = await structured_llm.ainvoke([HumanMessage(content=prompt)])
        return response.model_dump()

"""ReviewSummaryAgent: prepare a reviewer-facing summary for human approval.

LLM-based with fallback to programmatic summary from key_metrics and warnings.
"""

import json
import logging
from typing import Any

from langchain_core.messages import HumanMessage

from app.agents.base import BaseAgent
from app.core.llm import get_llm
from app.profiles.contracts import ReviewSummaryProfile

logger = logging.getLogger(__name__)


class ReviewSummaryAgent(BaseAgent):
    agent_id = "review_summary_agent"
    domain = "shared"
    reusable_scope = "shared"

    def __init__(self, profile: ReviewSummaryProfile) -> None:
        self.profile = profile

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
            summary = self.profile.build_fallback(
                key_metrics,
                analysis_narrative,
                warnings,
            )

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
        prompt = self.profile.build_prompt(key_metrics, analysis, warnings)
        response = await llm.ainvoke([HumanMessage(content=prompt)])
        text = response.content.strip()
        if isinstance(text, str):
            text = text.removeprefix("```json").removesuffix("```").strip()
        return json.loads(text)

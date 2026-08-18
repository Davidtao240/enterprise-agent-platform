"""ReportAgent: generate structured reports using an injected domain profile.

LLM-based with a template fallback for when the LLM is unavailable.
"""

import json
import logging
from typing import Any

from langchain_core.messages import HumanMessage

from app.agents.base import BaseAgent
from app.core.llm import get_llm
from app.profiles.contracts import ReportProfile

logger = logging.getLogger(__name__)


class ReportAgent(BaseAgent):
    agent_id = "report_agent"
    domain = "shared"
    reusable_scope = "shared"

    def __init__(self, profile: ReportProfile) -> None:
        self.profile = profile

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
            report = self.profile.build_fallback(mapped_data, key_metrics, warnings)
            report["warnings"].append(dict(self.profile.fallback_warning))

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
        prompt = self.profile.build_prompt(metrics, narrative, mapped_data, warnings)
        response = await llm.ainvoke([HumanMessage(content=prompt)])
        text = response.content.strip()
        result = self._parse_llm_json(text)
        result.setdefault("review", {"status": "pending", "reviewer": None, "comment": None, "reviewed_at": None})
        return result

    @staticmethod
    def _parse_llm_json(text: str) -> dict[str, Any]:
        """Parse LLM output into a JSON dict with robust fallback.

        LLM responses may arrive in three common formats:
        1. Plain JSON: ``{"key": "value"}``  — most reliable path.
        2. Markdown-fenced JSON: ```json ... ``` — strip the fences then parse.
        3. JSON wrapped in prose: ``Here is the result: {...}`` — extract the
           first ``{``…``}`` block and parse that.

        Raises ``json.JSONDecodeError`` if none of the strategies succeed so
        the caller can decide whether to retry or use the template fallback.
        """
        try:
            return json.loads(text)
        except json.JSONDecodeError:
            pass

        # Strategy 2: strip markdown code fences
        if text.startswith("```"):
            stripped = text.removeprefix("```json").removeprefix("```").removesuffix("```").strip()
            try:
                return json.loads(stripped)
            except json.JSONDecodeError:
                pass

        # Strategy 3: extract the first balanced { ... } block
        # Find the first '{' and attempt to match the closing '}'
        start = text.find("{")
        end = text.rfind("}")
        if start != -1 and end != -1 and end > start:
            candidate = text[start : end + 1]
            try:
                return json.loads(candidate)
            except json.JSONDecodeError:
                pass

        raise json.JSONDecodeError("LLM output is not valid JSON", text, 0)

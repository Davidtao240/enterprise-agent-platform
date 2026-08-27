"""Offline smoke tests that actually RUN each built-in graph's full lifecycle.

Coverage gap addressed: previously only ``finance_operating_report`` had a
graph smoke test. This adds true end-to-end graph execution for the other
built-in graphs:

- document_summary (LLM node patched with a fake structured LLM)
- finance_chat (LLM node patched with a fake structured LLM)
- meeting_minutes (LLM node patched with a fake structured LLM)
- finance_collaborative_report (all specialist agents are pure logic, so it
  runs fully offline without any LLM mocking)

Each test asserts the SUCCESS path (``error is None`` + expected output keys),
not just that a pydantic model validates.
"""

from __future__ import annotations

import asyncio
import unittest
from unittest.mock import patch
from uuid import uuid4

from app.graphs.document_summary import SummaryOutput
from app.graphs.finance_chat import AnalysisOutput, ChatResponse
from app.graphs.meeting_minutes import ActionItem, MinutesOutput


# ── Fake structured LLM ──
#
# The LLM-less graphs call ``get_llm().with_structured_output(Schema)`` and then
# ``await structured.ainvoke(prompt)``. We swap ``get_llm`` per-module for a fake
# that returns the schema-appropriate pydantic instance so the success path can
# run offline without any provider key.


class _StructuredFake:
    def __init__(self, output):
        self._output = output

    async def ainvoke(self, prompt):
        return self._output


def _make_doc_summary_output() -> SummaryOutput:
    return SummaryOutput(
        summary="项目整体进展符合预期。",
        key_findings=["收入同比上升"],
        action_items=["补齐季度成本数据"],
    )


def _make_analysis_output() -> AnalysisOutput:
    return AnalysisOutput(
        analysis_type="profitability",
        key_metrics=["revenue", "gross_margin"],
        approach="汇总财务报表核心科目",
        confidence="high",
    )


def _make_chat_response() -> ChatResponse:
    return ChatResponse(response="基于当前盈利能力指标，公司整体表现健康。")


def _make_minutes_output() -> MinutesOutput:
    return MinutesOutput(
        summary="季度经营会确认了重点方向。",
        decisions=["推进降本提质"],
        action_items=[
            ActionItem(action="输出下月预算", assignee="财务中心", deadline="2026-09-05")
        ],
        attendees=["财务中心"],
    )


class FakeLLM:
    """Duck-type ChatOpenAI: with_structured_output returns schema-aware fake."""

    _factories = {
        SummaryOutput: _make_doc_summary_output,
        AnalysisOutput: _make_analysis_output,
        ChatResponse: _make_chat_response,
        MinutesOutput: _make_minutes_output,
    }

    def with_structured_output(self, schema):
        factory = self._factories.get(schema)
        if factory is None:
            raise AssertionError(f"Unexpected structured output schema: {schema}")
        return _StructuredFake(factory())


def _thread(trace_id: str) -> dict[str, object]:
    return {"configurable": {"thread_id": trace_id}}


def _base_state(**extra) -> dict:
    trace_id = str(uuid4())
    return {
        "trace_id": trace_id,
        "workflow_instance_id": "smoke-wf",
        "node_instance_id": "smoke-node",
        **extra,
    }


class DocumentSummaryGraphSmokeTest(unittest.TestCase):
    def test_success_path_yields_summary_without_error(self):
        from app.graphs.document_summary import build_document_summary_graph

        graph = build_document_summary_graph()
        initial = _base_state(messages=[], document_content="本季度营收增长，成本可控。")

        with patch("app.graphs.document_summary.get_llm", return_value=FakeLLM()):
            final = asyncio.run(graph.ainvoke(initial, _thread(initial["trace_id"])))

        self.assertIsNone(final.get("error"))
        self.assertTrue(final.get("summary"))
        self.assertIsInstance(final.get("key_findings"), list)
        self.assertIsInstance(final.get("action_items"), list)


class FinanceChatGraphSmokeTest(unittest.TestCase):
    def test_success_path_yields_response_without_error(self):
        from app.graphs.finance_chat import build_finance_chat_graph

        graph = build_finance_chat_graph()
        initial = _base_state(messages=[], user_query="评估本季度盈利能力")

        with patch("app.graphs.finance_chat.get_llm", return_value=FakeLLM()):
            final = asyncio.run(graph.ainvoke(initial, _thread(initial["trace_id"])))

        self.assertIsNone(final.get("error"))
        self.assertTrue(final.get("response"))
        # analysis is a schema field on FinanceChatState; analysis_type lives inside it.
        self.assertEqual((final.get("analysis") or {}).get("analysis_type"), "profitability")

    def test_success_path_merges_usage_across_nodes(self):
        from app.graphs.finance_chat import build_finance_chat_graph

        graph = build_finance_chat_graph()
        initial = _base_state(messages=[], user_query="分析现金流风险")

        with patch("app.graphs.finance_chat.get_llm", return_value=FakeLLM()):
            final = asyncio.run(graph.ainvoke(initial, _thread(initial["trace_id"])))

        self.assertIsNotNone(final.get("usage"))


class MeetingMinutesGraphSmokeTest(unittest.TestCase):
    def test_success_path_yields_minutes_without_error(self):
        from app.graphs.meeting_minutes import build_meeting_minutes_graph

        graph = build_meeting_minutes_graph()
        initial = _base_state(messages=[], transcript="会议决定下月聚焦降本增效。")

        with patch("app.graphs.meeting_minutes.get_llm", return_value=FakeLLM()):
            final = asyncio.run(graph.ainvoke(initial, _thread(initial["trace_id"])))

        self.assertIsNone(final.get("error"))
        minutes = final.get("minutes") or {}
        self.assertTrue(minutes.get("summary"))
        self.assertIsInstance(minutes.get("action_items"), list)
        self.assertIsInstance(minutes.get("attendees"), list)


class CollaborativeReportGraphSmokeTest(unittest.TestCase):
    """All specialist agents are pure logic — the full multi-agent flow runs
    offline with no LLM mocking, exercising supervisor + 3 agents + synthesis."""

    def test_success_path_yields_final_report_without_error(self):
        from app.graphs.finance_collaborative_report import build_finance_collaborative_report_graph

        graph = build_finance_collaborative_report_graph()
        initial = _base_state(
            file_id=None,
            user_request="生成财务分析报告",
            raw_data={
                "rows": [
                    {"month": "2026-05", "department": "财务中心", "revenue": 1000, "cost": 600},
                    {"month": "2026-06", "department": "财务中心", "revenue": 1200, "cost": 700},
                ]
            },
        )

        final = asyncio.run(graph.ainvoke(initial, _thread(initial["trace_id"])))

        self.assertIsNone(final.get("error"))
        report = final.get("final_report") or {}
        self.assertEqual(report.get("status"), "complete")
        self.assertIn("data_collection", report)
        self.assertIn("analysis", report)
        self.assertIn("compliance", report)
        # Data-collection status gates routing to analysis; verify it completed.
        sub_results = final.get("sub_results", {})
        self.assertEqual(sub_results.get("data_collection", {}).get("status"), "completed")


if __name__ == "__main__":
    unittest.main()
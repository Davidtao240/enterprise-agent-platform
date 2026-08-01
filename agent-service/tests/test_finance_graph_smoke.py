import asyncio
import unittest
from unittest.mock import AsyncMock, patch
from uuid import uuid4

try:
    from app.agents.finance_analysis import FinanceAnalysisAgent, _build_fallback_analysis
    from app.agents.report import ReportAgent
    from app.agents.review_summary import ReviewSummaryAgent
    from app.graphs.finance_operating_report import build_finance_operating_report_graph
    from app.profiles.finance import (
        build_fallback_report,
        build_fallback_review_summary,
    )
except ModuleNotFoundError as exc:
    build_finance_operating_report_graph = None
    IMPORT_ERROR = exc
else:
    IMPORT_ERROR = None


class FinanceGraphSmokeTest(unittest.TestCase):
    def test_finance_fallback_content_is_chinese(self):
        if build_finance_operating_report_graph is None:
            self.skipTest(f"Agent dependencies unavailable: {IMPORT_ERROR}")

        metrics = {
            "revenue": 1000,
            "cost": 600,
            "gross_profit": 400,
            "net_profit": 250,
            "gross_margin": 0.4,
            "net_margin": 0.25,
            "row_count": 1,
        }
        analysis = _build_fallback_analysis(metrics)
        report = build_fallback_report(
            {"mapped_rows": [{"month": "2026-05", "department": "财务中心"}]},
            metrics,
            [],
        )
        summary = build_fallback_review_summary(metrics, analysis, [])

        self.assertRegex(analysis["revenue_summary"], r"[\u4e00-\u9fff]")
        self.assertRegex(report["title"], r"[\u4e00-\u9fff]")
        self.assertRegex(summary["summary"], r"[\u4e00-\u9fff]")

    def test_finance_graph_runs_with_sample_data_fallback(self):
        if build_finance_operating_report_graph is None:
            self.skipTest(f"LangGraph dependency unavailable: {IMPORT_ERROR}")

        graph = build_finance_operating_report_graph()
        trace_id = str(uuid4())
        initial_state = {
            "trace_id": trace_id,
            "workflow_instance_id": "wf-smoke",
            "node_instance_id": "node-smoke",
            "file_id": None,
            "raw_data": None,
            "mapped_data": None,
            "validation_result": None,
            "analysis_result": None,
            "report": None,
            "review_summary": None,
            "error": None,
            "usage": None,
            "_load_warnings": None,
        }

        with (
            patch.object(
                FinanceAnalysisAgent,
                "_llm_analyze",
                new=AsyncMock(side_effect=RuntimeError("offline smoke fixture")),
            ),
            patch.object(
                ReportAgent,
                "_llm_generate",
                new=AsyncMock(side_effect=RuntimeError("offline smoke fixture")),
            ),
            patch.object(
                ReviewSummaryAgent,
                "_llm_summarize",
                new=AsyncMock(side_effect=RuntimeError("offline smoke fixture")),
            ),
        ):
            final_state = asyncio.run(
                graph.ainvoke(
                    initial_state,
                    {"configurable": {"thread_id": trace_id}},
                )
            )

        self.assertIsNone(final_state.get("error"))
        self.assertTrue(final_state.get("validation_result", {}).get("valid"))
        self.assertIn("key_metrics", final_state.get("analysis_result", {}))
        self.assertIn("report", final_state.get("report", {}))
        self.assertIn("summary", final_state.get("review_summary", {}))


if __name__ == "__main__":
    unittest.main()

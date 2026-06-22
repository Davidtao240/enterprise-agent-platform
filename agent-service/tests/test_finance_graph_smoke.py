import asyncio
import unittest
from uuid import uuid4

try:
    from app.graphs.finance_operating_report import build_finance_operating_report_graph
except ModuleNotFoundError as exc:
    build_finance_operating_report_graph = None
    IMPORT_ERROR = exc
else:
    IMPORT_ERROR = None


class FinanceGraphSmokeTest(unittest.TestCase):
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

        final_state = asyncio.run(graph.ainvoke(initial_state, {"configurable": {"thread_id": trace_id}}))

        self.assertIsNone(final_state.get("error"))
        self.assertTrue(final_state.get("validation_result", {}).get("valid"))
        self.assertIn("key_metrics", final_state.get("analysis_result", {}))
        self.assertIn("report", final_state.get("report", {}))
        self.assertIn("summary", final_state.get("review_summary", {}))


if __name__ == "__main__":
    unittest.main()

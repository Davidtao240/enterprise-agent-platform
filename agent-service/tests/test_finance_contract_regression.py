import asyncio
import copy
import json
import unittest
from pathlib import Path
from unittest.mock import AsyncMock, patch

from app.agents.data_extract import DataExtractAgent
from app.agents.finance_analysis import FinanceAnalysisAgent, _calculate_metrics
from app.agents.report import ReportAgent
from app.agents.review_summary import ReviewSummaryAgent
from app.agents.schema_mapping import SchemaMappingAgent
from app.agents.validation import ValidationAgent
from app.graphs import finance_operating_report as finance_graph
from app.main import _build_agent_run_response
from app.profiles.finance import FINANCE_PROFILE

FIXTURE_PATH = Path(__file__).parent / "fixtures" / "finance_v1_contract.json"
REPOSITORY_ROOT = Path(__file__).resolve().parents[2]


def _initial_state() -> dict:
    return {
        "trace_id": "trace-finance-contract",
        "workflow_instance_id": "wf-finance-contract",
        "node_instance_id": "node-finance-contract",
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


class FinanceContractRegressionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.fixture = json.loads(FIXTURE_PATH.read_text(encoding="utf-8"))
        cls.graph = finance_graph.build_finance_operating_report_graph()

    def test_profile_identity_is_versioned_and_explicit(self):
        expected = self.fixture["profile"]
        self.assertEqual(FINANCE_PROFILE.profile_key, expected["profile_key"])
        self.assertEqual(FINANCE_PROFILE.profile_version, expected["profile_version"])
        self.assertEqual(
            FINANCE_PROFILE.business_app_code,
            expected["business_app_code"],
        )

    def test_mapping_and_validation_match_finance_v1_fixture(self):
        success = self.fixture["success"]
        mapping_agent = SchemaMappingAgent(FINANCE_PROFILE.schema_mapping)
        mapping_state = asyncio.run(mapping_agent.run({
            "raw_data": copy.deepcopy(success["raw_data"]),
        }))

        mapped_data = mapping_state["mapped_data"]
        self.assertEqual(mapped_data["field_mapping"], success["field_mapping"])
        self.assertEqual(mapped_data["mapped_rows"], success["mapped_rows"])

        validation_agent = ValidationAgent(FINANCE_PROFILE.validation)
        validation_state = asyncio.run(validation_agent.run({
            "mapped_data": mapped_data,
            "_load_warnings": [],
        }))
        result = validation_state["validation_result"]
        self.assertEqual(result["valid"], success["validation"]["valid"])
        self.assertEqual(
            [issue["code"] for issue in result["errors"]],
            success["validation"]["error_codes"],
        )
        self.assertEqual(
            [issue["code"] for issue in result["warnings"]],
            success["validation"]["warning_codes"],
        )

    def test_profile_fallback_preserves_finance_demo_baseline(self):
        expected = self.fixture["fallback_demo"]
        extraction_agent = DataExtractAgent(FINANCE_PROFILE.data_extraction)
        state = asyncio.run(extraction_agent.run({
            "file_id": None,
            "raw_data": None,
        }))

        self.assertEqual(state["raw_data"]["columns"], expected["columns"])
        self.assertEqual(len(state["raw_data"]["rows"]), expected["row_count"])
        self.assertEqual(
            state["_load_warnings"],
            [{"level": "info", "message": expected["warning"]}],
        )
        self.assertEqual(
            _calculate_metrics(state["raw_data"]["rows"]),
            expected["key_metrics"],
        )

    def test_success_graph_and_output_envelope_are_deterministic(self):
        success = self.fixture["success"]

        async def extract_fixture(state: dict) -> dict:
            state["raw_data"] = copy.deepcopy(success["raw_data"])
            return state

        with (
            patch.object(
                finance_graph._data_extract_agent,
                "run",
                new=AsyncMock(side_effect=extract_fixture),
            ),
            patch.object(
                finance_graph._finance_analysis_agent,
                "_llm_analyze",
                new=AsyncMock(side_effect=RuntimeError("offline fixture")),
            ),
            patch.object(
                finance_graph._report_agent,
                "_llm_generate",
                new=AsyncMock(side_effect=RuntimeError("offline fixture")),
            ),
            patch.object(
                finance_graph._review_summary_agent,
                "_llm_summarize",
                new=AsyncMock(side_effect=RuntimeError("offline fixture")),
            ),
        ):
            final_state = asyncio.run(self.graph.ainvoke(_initial_state()))

        self.assertIsNone(final_state.get("error"))
        self.assertEqual(
            final_state["mapped_data"]["field_mapping"],
            success["field_mapping"],
        )
        self.assertEqual(
            final_state["analysis_result"]["key_metrics"],
            success["key_metrics"],
        )

        response = _build_agent_run_response(
            final_state,
            "run-finance-contract",
            "finance_operating_report_graph",
        )
        self.assertEqual(response["status"], success["response_status"])
        self.assertEqual(sorted(response["output"]), success["output_keys"])
        self.assertIsNone(response["error"])
        json.dumps(response, ensure_ascii=False)

    def test_insufficient_information_stops_before_analysis(self):
        scenario = self.fixture["insufficient_information"]

        async def extract_fixture(state: dict) -> dict:
            state["raw_data"] = copy.deepcopy(scenario["raw_data"])
            return state

        analysis_run = AsyncMock()
        with (
            patch.object(
                finance_graph._data_extract_agent,
                "run",
                new=AsyncMock(side_effect=extract_fixture),
            ),
            patch.object(
                finance_graph._finance_analysis_agent,
                "run",
                new=analysis_run,
            ),
        ):
            final_state = asyncio.run(self.graph.ainvoke(_initial_state()))

        analysis_run.assert_not_awaited()
        validation = final_state["validation_result"]
        self.assertFalse(validation["valid"])
        self.assertEqual(
            [issue["code"] for issue in validation["errors"]],
            scenario["validation_error_codes"],
        )
        self.assertEqual(
            [issue["field"] for issue in validation["errors"]],
            scenario["missing_fields"],
        )

        response = _build_agent_run_response(
            final_state,
            "run-finance-insufficient",
            "finance_operating_report_graph",
        )
        self.assertEqual(response["status"], scenario["finance_v1_response_status"])
        self.assertEqual(
            response["error"]["code"],
            scenario["finance_v1_error_code"],
        )

    def test_retryable_failure_serializes_error_and_usage(self):
        scenario = self.fixture["retryable_failure"]
        with patch.object(
            finance_graph._data_extract_agent,
            "run",
            new=AsyncMock(side_effect=RuntimeError(scenario["error"]["message"])),
        ):
            final_state = asyncio.run(self.graph.ainvoke(_initial_state()))

        self.assertEqual(final_state["error"], scenario["error"])
        final_state["usage"] = copy.deepcopy(self.fixture["usage"])
        response = _build_agent_run_response(
            final_state,
            "run-finance-retryable",
            "finance_operating_report_graph",
        )
        self.assertEqual(response["status"], scenario["response_status"])
        self.assertEqual(response["error"], scenario["error"])
        self.assertEqual(response["usage"], self.fixture["usage"])
        json.dumps(response)

    def test_non_retryable_failure_stops_after_schema_mapping(self):
        scenario = self.fixture["non_retryable_failure"]

        async def extract_empty(state: dict) -> dict:
            state["raw_data"] = {"columns": [], "rows": []}
            return state

        with patch.object(
            finance_graph._data_extract_agent,
            "run",
            new=AsyncMock(side_effect=extract_empty),
        ):
            final_state = asyncio.run(self.graph.ainvoke(_initial_state()))

        self.assertEqual(final_state["error"], scenario["error"])
        response = _build_agent_run_response(
            final_state,
            "run-finance-non-retryable",
            "finance_operating_report_graph",
        )
        self.assertEqual(response["status"], scenario["response_status"])
        self.assertEqual(response["error"], scenario["error"])

    def test_python_metadata_matches_go_seed(self):
        agent_classes = (
            DataExtractAgent,
            SchemaMappingAgent,
            ValidationAgent,
            FinanceAnalysisAgent,
            ReportAgent,
            ReviewSummaryAgent,
        )
        expected = self.fixture["agent_metadata"]
        for agent_class in agent_classes:
            with self.subTest(agent_id=agent_class.agent_id):
                self.assertEqual(agent_class.domain, expected[agent_class.agent_id]["domain"])
                self.assertEqual(
                    agent_class.reusable_scope,
                    expected[agent_class.agent_id]["reusable_scope"],
                )

        seed_sql = (
            REPOSITORY_ROOT
            / "go-platform"
            / "migrations"
            / "004_seed_finance_v1.up.sql"
        ).read_text(encoding="utf-8")
        for agent_id, metadata in expected.items():
            self.assertIn(f"'{agent_id}'", seed_sql)
            self.assertRegex(
                seed_sql,
                rf"'{agent_id}'.*'{metadata['domain']}', "
                rf"'{metadata['reusable_scope']}'",
            )


if __name__ == "__main__":
    unittest.main()

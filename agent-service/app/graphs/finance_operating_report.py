"""Finance Operating Report Graph.
Agent flow: DataExtract → SchemaMapping → Validation → FinanceAnalysis → Report → ReviewSummary

Built with LangGraph StateGraph + SQLite checkpointer for state persistence.
Conditional edges stop execution on error or validation failure.
"""

from __future__ import annotations

import logging
from typing import Any, Literal, Optional

from langgraph.graph import END, StateGraph
from typing_extensions import TypedDict

from app.agents.data_extract import DataExtractAgent
from app.agents.schema_mapping import SchemaMappingAgent
from app.agents.validation import ValidationAgent
from app.agents.finance_analysis import FinanceAnalysisAgent
from app.agents.report import ReportAgent
from app.agents.review_summary import ReviewSummaryAgent
from app.core.usage_tracker import UsageTracker

logger = logging.getLogger(__name__)

# ── Agent singletons (stateless, safe to reuse across invocations) ──
_data_extract_agent = DataExtractAgent()
_schema_mapping_agent = SchemaMappingAgent()
_validation_agent = ValidationAgent()
_finance_analysis_agent = FinanceAnalysisAgent()
_report_agent = ReportAgent()
_review_summary_agent = ReviewSummaryAgent()


class FinanceGraphState(TypedDict):
    trace_id: str
    workflow_instance_id: str
    node_instance_id: str
    file_id: Optional[str]
    raw_data: Optional[dict[str, Any]]
    mapped_data: Optional[dict[str, Any]]
    validation_result: Optional[dict[str, Any]]
    analysis_result: Optional[dict[str, Any]]
    report: Optional[dict[str, Any]]
    review_summary: Optional[dict[str, Any]]
    error: Optional[dict[str, Any]]
    usage: Optional[dict[str, Any]]
    _load_warnings: Optional[list[dict[str, Any]]]


# ── Node functions ──

async def data_extract_node(state: FinanceGraphState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    agent = _data_extract_agent
    try:
        result = await agent.run(dict(state))
        return _diff_state(state, result)
    except Exception as e:
        logger.exception("data_extract_node failed")
        return {"error": {"code": "DATA_EXTRACT_FAILED", "message": str(e), "retryable": True}}


async def schema_mapping_node(state: FinanceGraphState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    agent = _schema_mapping_agent
    try:
        result = await agent.run(dict(state))
        return _diff_state(state, result)
    except Exception as e:
        logger.exception("schema_mapping_node failed")
        return {"error": {"code": "SCHEMA_MAPPING_FAILED", "message": str(e), "retryable": True}}


async def validation_node(state: FinanceGraphState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    agent = _validation_agent
    try:
        result = await agent.run(dict(state))
        return _diff_state(state, result)
    except Exception as e:
        logger.exception("validation_node failed")
        return {"error": {"code": "SCHEMA_VALIDATION_FAILED", "message": str(e), "retryable": False}}


async def finance_analysis_node(state: FinanceGraphState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    agent = _finance_analysis_agent
    try:
        result = await agent.run(dict(state))
        return _diff_state(state, result)
    except Exception as e:
        logger.exception("finance_analysis_node failed")
        return {"error": {"code": "FINANCE_ANALYSIS_FAILED", "message": str(e), "retryable": True}}


async def report_node(state: FinanceGraphState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    agent = _report_agent
    try:
        result = await agent.run(dict(state))
        return _diff_state(state, result)
    except Exception as e:
        logger.exception("report_node failed")
        return {"error": {"code": "REPORT_GENERATION_FAILED", "message": str(e), "retryable": True}}


async def review_summary_node(state: FinanceGraphState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    agent = _review_summary_agent
    try:
        result = await agent.run(dict(state))
        return _diff_state(state, result)
    except Exception as e:
        logger.exception("review_summary_node failed")
        return {"error": {"code": "GRAPH_EXECUTION_FAILED", "message": str(e), "retryable": True}}


# ── Helpers ──

def _diff_state(original: dict[str, Any], updated: dict[str, Any]) -> dict[str, Any]:
    """Return only the keys that changed between original and updated state."""
    diff = {}
    for k in ("raw_data", "mapped_data", "validation_result", "analysis_result",
              "report", "review_summary", "error", "_load_warnings"):
        if k in updated and updated.get(k) != original.get(k):
            diff[k] = updated[k]
    return diff


def _route_on_error(state: FinanceGraphState) -> Literal["next", "end"]:
    return "end" if state.get("error") else "next"


def _route_after_validation(state: FinanceGraphState) -> Literal["next", "end"]:
    if state.get("error"):
        return "end"
    validation = state.get("validation_result") or {}
    if not validation.get("valid", True):
        return "end"
    return "next"


# ── Graph construction ──

_finance_graph = None


def build_finance_operating_report_graph() -> StateGraph:
    global _finance_graph
    if _finance_graph is not None:
        return _finance_graph

    graph = StateGraph(FinanceGraphState)

    graph.add_node("data_extract", data_extract_node)
    graph.add_node("schema_mapping", schema_mapping_node)
    graph.add_node("validation", validation_node)
    graph.add_node("finance_analysis", finance_analysis_node)
    graph.add_node("report", report_node)
    graph.add_node("review_summary", review_summary_node)

    graph.set_entry_point("data_extract")

    graph.add_conditional_edges("data_extract", _route_on_error, {"next": "schema_mapping", "end": END})
    graph.add_conditional_edges("schema_mapping", _route_on_error, {"next": "validation", "end": END})
    graph.add_conditional_edges("validation", _route_after_validation, {"next": "finance_analysis", "end": END})
    graph.add_conditional_edges("finance_analysis", _route_on_error, {"next": "report", "end": END})
    graph.add_conditional_edges("report", _route_on_error, {"next": "review_summary", "end": END})
    graph.add_edge("review_summary", END)

    # Compile without checkpointer for now — state persistence across retries
    # is managed by the Go backend via trace_id. Checkpointer can be added
    # when async SQLite connection pooling is needed.
    _finance_graph = graph.compile()
    return _finance_graph

"""Finance Collaborative Report Graph — M9-A Multi-Agent Orchestration.

Uses LangGraph supervisor pattern to coordinate 3 specialist agents:
1. DataCollectionAgent — gathers and standardizes financial data
2. AnalysisAgent — computes metrics, detects anomalies
3. ComplianceAgent — validates against regulatory rules

The supervisor decomposes the task, dispatches to agents in sequence
(Data → Analysis → Compliance), then synthesizes the final report.
"""

from __future__ import annotations

import json
import logging
from typing import Any, Literal, Optional

from langgraph.graph import END, StateGraph
from typing_extensions import TypedDict

from app.agents.supervisor import SupervisorAgent
from app.agents.data_collection import DataCollectionAgent
from app.agents.analysis import AnalysisAgent
from app.agents.compliance import ComplianceAgent

logger = logging.getLogger(__name__)

# ── Agent singletons ──
_supervisor = SupervisorAgent()
_data_collection_agent = DataCollectionAgent()
_analysis_agent = AnalysisAgent()
_compliance_agent = ComplianceAgent()


class CollaborativeReportState(TypedDict):
    trace_id: str
    workflow_instance_id: str
    node_instance_id: str
    file_id: Optional[str]
    user_request: str
    raw_data: Optional[dict[str, Any]]
    sub_tasks: list[dict[str, Any]]
    sub_results: dict[str, Any]
    analysis_result: Optional[dict[str, Any]]
    compliance_result: Optional[dict[str, Any]]
    final_report: Optional[dict[str, Any]]
    error: Optional[dict[str, Any]]
    usage: Optional[dict[str, Any]]


def build_collaborative_report_state(body: dict[str, Any]) -> dict[str, Any]:
    raw_input = body.get("input", {})
    workflow_input = raw_input.get("workflow_input", {})
    if isinstance(workflow_input, str):
        try:
            workflow_input = json.loads(workflow_input)
        except (json.JSONDecodeError, TypeError):
            workflow_input = {}

    file_id = workflow_input.get("file_id") or raw_input.get("file_id")
    user_request = workflow_input.get("user_request") or raw_input.get("user_request") or "生成财务分析报告"

    raw_data = None
    rows = workflow_input.get("rows") or raw_input.get("rows")
    if rows:
        raw_data = {"rows": rows}

    return {
        "trace_id": body.get("trace_id", ""),
        "workflow_instance_id": body.get("workflow_instance_id", ""),
        "node_instance_id": body.get("node_instance_id", ""),
        "file_id": file_id,
        "user_request": user_request,
        "raw_data": raw_data,
        "sub_tasks": [
            {"task": "data_collection", "agent": "data_collection_agent", "status": "pending"},
            {"task": "analysis", "agent": "analysis_agent", "status": "pending"},
            {"task": "compliance_check", "agent": "compliance_agent", "status": "pending"},
            {"task": "synthesis", "agent": "supervisor_agent", "status": "pending"},
        ],
        "sub_results": {},
        "analysis_result": None,
        "compliance_result": None,
        "final_report": None,
        "error": None,
        "usage": None,
    }


# ── Node functions ──

async def supervisor_plan_node(state: CollaborativeReportState) -> dict[str, Any]:
    """Supervisor plans the task decomposition."""
    if state.get("error"):
        return {}
    try:
        plan = await _supervisor.run(dict(state))
        return {"sub_tasks": plan.get("sub_tasks", state.get("sub_tasks", []))}
    except Exception as e:
        logger.exception("supervisor_plan_node failed")
        return {"error": {"code": "SUPERVISOR_PLAN_FAILED", "message": str(e), "retryable": False}}


async def data_collection_node(state: CollaborativeReportState) -> dict[str, Any]:
    """Data Collection agent gathers financial data."""
    if state.get("error"):
        return {}
    try:
        result = await _data_collection_agent.run(dict(state))
        return {"sub_results": result.get("sub_results", {})}
    except Exception as e:
        logger.exception("data_collection_node failed")
        return {"error": {"code": "DATA_COLLECTION_FAILED", "message": str(e), "retryable": True}}


async def analysis_node(state: CollaborativeReportState) -> dict[str, Any]:
    """Analysis agent computes metrics and detects anomalies."""
    if state.get("error"):
        return {}
    try:
        result = await _analysis_agent.run(dict(state))
        sub_results = result.get("sub_results", {})
        analysis_result = sub_results.get("analysis", {})
        return {
            "sub_results": sub_results,
            "analysis_result": analysis_result.get("analysis"),
        }
    except Exception as e:
        logger.exception("analysis_node failed")
        return {"error": {"code": "ANALYSIS_FAILED", "message": str(e), "retryable": True}}


async def compliance_node(state: CollaborativeReportState) -> dict[str, Any]:
    """Compliance agent validates against regulatory rules."""
    if state.get("error"):
        return {}
    try:
        result = await _compliance_agent.run(dict(state))
        sub_results = result.get("sub_results", {})
        compliance_result = sub_results.get("compliance_check", {})
        return {
            "sub_results": sub_results,
            "compliance_result": compliance_result.get("compliance"),
        }
    except Exception as e:
        logger.exception("compliance_node failed")
        return {"error": {"code": "COMPLIANCE_CHECK_FAILED", "message": str(e), "retryable": True}}


async def synthesis_node(state: CollaborativeReportState) -> dict[str, Any]:
    """Supervisor synthesizes all results into final report."""
    if state.get("error"):
        return {}
    try:
        result = await _supervisor.run(dict(state))
        sub_results = state.get("sub_results", {})
        final_report = {
            "report_id": f"collaborative_report_{state.get('trace_id', '')[:8]}",
            "user_request": state.get("user_request", ""),
            "generated_at": "auto",
            "collaboration_summary": {
                "agents_participated": [
                    "data_collection_agent",
                    "analysis_agent",
                    "compliance_agent",
                    "supervisor_agent",
                ],
                "tasks_completed": len(sub_results),
                "plan": state.get("sub_tasks", []),
            },
            "data_collection": sub_results.get("data_collection", {}),
            "analysis": state.get("analysis_result", {}),
            "compliance": state.get("compliance_result", {}),
            "supervisor_summary": result.get("aggregated_result", {}),
            "status": "complete",
        }
        return {"final_report": final_report}
    except Exception as e:
        logger.exception("synthesis_node failed")
        return {"error": {"code": "SYNTHESIS_FAILED", "message": str(e), "retryable": True}}


# ── Routing ──

def _route_on_error(state: CollaborativeReportState) -> Literal["next", "end"]:
    return "end" if state.get("error") else "next"


def _route_after_collection(state: CollaborativeReportState) -> Literal["next", "end"]:
    sub_results = state.get("sub_results", {})
    if "data_collection" not in sub_results:
        return "end"
    collection_result = sub_results.get("data_collection", {})
    if collection_result.get("status") != "completed":
        return "end"
    return "next"


def _route_after_analysis(state: CollaborativeReportState) -> Literal["next", "end"]:
    if state.get("error"):
        return "end"
    if state.get("analysis_result") is None:
        return "end"
    return "next"


# ── Graph construction ──

def build_finance_collaborative_report_graph(checkpointer: Any | None = None) -> Any:
    """Build the multi-agent collaborative report graph.

    Flow:
    supervisor_plan → data_collection → analysis → compliance → synthesis → END
    """
    graph = StateGraph(CollaborativeReportState)

    graph.add_node("supervisor_plan", supervisor_plan_node)
    graph.add_node("data_collection", data_collection_node)
    graph.add_node("analysis", analysis_node)
    graph.add_node("compliance_check", compliance_node)
    graph.add_node("synthesis", synthesis_node)

    graph.set_entry_point("supervisor_plan")

    graph.add_conditional_edges("supervisor_plan", _route_on_error, {"next": "data_collection", "end": END})
    graph.add_conditional_edges("data_collection", _route_after_collection, {"next": "analysis", "end": END})
    graph.add_conditional_edges("analysis", _route_after_analysis, {"next": "compliance_check", "end": END})
    graph.add_conditional_edges("compliance_check", _route_on_error, {"next": "synthesis", "end": END})
    graph.add_edge("synthesis", END)

    return graph.compile(checkpointer=checkpointer)
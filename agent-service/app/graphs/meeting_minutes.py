"""Meeting Minutes Graph.
Conversational agent graph for meeting minutes generation.
Flow: process_input → generate_minutes → format_output → END

Built with LangGraph StateGraph.
"""

from __future__ import annotations

import json
import logging
from typing import Any, Optional

from langgraph.graph import END, StateGraph
from typing_extensions import TypedDict

from app.core.llm import get_llm

logger = logging.getLogger(__name__)


class MeetingMinutesState(TypedDict):
    trace_id: str
    workflow_instance_id: str
    node_instance_id: str
    messages: Optional[list[dict[str, Any]]]
    transcript: Optional[str]
    minutes: Optional[dict[str, Any]]
    decisions: Optional[list[str]]
    action_items: Optional[list[dict[str, Any]]]
    attendees: Optional[list[str]]
    error: Optional[dict[str, Any]]
    usage: Optional[dict[str, Any]]


def build_meeting_minutes_state(body: dict[str, Any]) -> dict[str, Any]:
    raw_input = body.get("input", {})
    workflow_input = raw_input.get("workflow_input", {})
    if isinstance(workflow_input, str):
        try:
            workflow_input = json.loads(workflow_input)
        except (json.JSONDecodeError, TypeError):
            workflow_input = {}

    content = workflow_input.get("content", "") or raw_input.get("content", "")

    return {
        "trace_id": body.get("trace_id", ""),
        "workflow_instance_id": body.get("workflow_instance_id", ""),
        "node_instance_id": body.get("node_instance_id", ""),
        "messages": workflow_input.get("messages") or [],
        "transcript": content,
        "minutes": None,
        "decisions": None,
        "action_items": None,
        "attendees": None,
        "error": None,
        "usage": None,
    }


async def process_input_node(state: MeetingMinutesState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    try:
        transcript = state.get("transcript", "") or ""
        messages = state.get("messages") or []
        if messages and not transcript:
            last_msg = messages[-1]
            transcript = last_msg.get("content", "") if isinstance(last_msg, dict) else str(last_msg)
        return {"transcript": transcript, "messages": messages}
    except Exception as e:
        logger.exception("process_input_node failed")
        return {"error": {"code": "PROCESS_INPUT_FAILED", "message": str(e), "retryable": True}}


async def generate_minutes_node(state: MeetingMinutesState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    try:
        llm = get_llm(temperature=0.3)
        transcript = state.get("transcript", "") or ""

        prompt = f"""You are an expert meeting minutes generator. Given the following meeting transcript or notes, produce structured meeting minutes including:
1. Summary of the meeting (key topics discussed)
2. Decisions made during the meeting
3. Action items with assignees and deadlines if mentioned
4. List of attendees mentioned

Meeting transcript:
{transcript[:8000]}

Respond in JSON format with these keys:
- "summary": string (brief overview of the meeting)
- "decisions": array of strings
- "action_items": array of objects with "action" and "assignee" and "deadline" keys
- "attendees": array of strings
"""

        response = await llm.ainvoke(prompt)
        response_text = response.content if hasattr(response, "content") else str(response)

        try:
            parsed = json.loads(response_text)
        except json.JSONDecodeError:
            parsed = {
                "summary": response_text,
                "decisions": [],
                "action_items": [],
                "attendees": [],
            }

        usage = {}
        if hasattr(response, "usage_metadata") and response.usage_metadata:
            usage = {
                "prompt_tokens": response.usage_metadata.get("input_tokens", 0),
                "completion_tokens": response.usage_metadata.get("output_tokens", 0),
                "total_tokens": response.usage_metadata.get("total_tokens", 0),
            }

        minutes = {
            "summary": parsed.get("summary", ""),
            "decisions": parsed.get("decisions", []),
            "action_items": parsed.get("action_items", []),
            "attendees": parsed.get("attendees", []),
        }

        return {
            "minutes": minutes,
            "decisions": minutes["decisions"],
            "action_items": minutes["action_items"],
            "attendees": minutes["attendees"],
            "usage": usage,
        }
    except Exception as e:
        logger.exception("generate_minutes_node failed")
        return {"error": {"code": "GENERATE_MINUTES_FAILED", "message": str(e), "retryable": True}}


async def format_output_node(state: MeetingMinutesState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    try:
        minutes = state.get("minutes") or {}
        output = {
            "minutes": {
                "summary": minutes.get("summary", ""),
                "decisions": minutes.get("decisions", []),
                "action_items": minutes.get("action_items", []),
                "attendees": minutes.get("attendees", []),
            }
        }
        return {"minutes": minutes}
    except Exception as e:
        logger.exception("format_output_node failed")
        return {"error": {"code": "FORMAT_OUTPUT_FAILED", "message": str(e), "retryable": False}}


def _diff_state(original: dict[str, Any], updated: dict[str, Any]) -> dict[str, Any]:
    diff = {}
    for k in ("transcript", "minutes", "decisions", "action_items", "attendees", "error", "usage"):
        if k in updated and updated.get(k) != original.get(k):
            diff[k] = updated[k]
    return diff


def build_meeting_minutes_graph(checkpointer: Any | None = None) -> Any:
    graph = StateGraph(MeetingMinutesState)

    graph.add_node("process_input", process_input_node)
    graph.add_node("generate_minutes", generate_minutes_node)
    graph.add_node("format_output", format_output_node)

    graph.set_entry_point("process_input")
    graph.add_edge("process_input", "generate_minutes")
    graph.add_edge("generate_minutes", "format_output")
    graph.add_edge("format_output", END)

    return graph.compile(checkpointer=checkpointer)
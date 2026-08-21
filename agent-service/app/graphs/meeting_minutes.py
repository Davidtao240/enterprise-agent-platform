"""Meeting Minutes Graph.
Conversational agent graph for meeting minutes generation.
Flow: process_input → generate_minutes → format_output → END

Built with LangGraph StateGraph.
"""

from __future__ import annotations

import asyncio
import json
import logging
from typing import Any, Optional

from langgraph.graph import END, StateGraph
from pydantic import BaseModel, Field
from typing_extensions import TypedDict

from app.core.llm import get_llm
from app.core.prompt_safety import sanitize_user_input

logger = logging.getLogger(__name__)


class ActionItem(BaseModel):
    action: str = Field(description="The action to be taken")
    assignee: str = Field(default="", description="Person responsible for the action")
    deadline: str = Field(default="", description="Deadline for completion")


class MinutesOutput(BaseModel):
    summary: str = Field(description="Brief overview of the meeting")
    decisions: list[str] = Field(description="Key decisions made during the meeting")
    action_items: list[ActionItem] = Field(description="Action items with assignees and deadlines")
    attendees: list[str] = Field(description="List of attendees mentioned")


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
        llm = get_llm(temperature=0.0)
        transcript = state.get("transcript", "") or ""
        safe_transcript = sanitize_user_input(transcript)

        prompt = f"""You are an expert meeting minutes generator. Given the following meeting transcript or notes, produce structured meeting minutes including:
1. Summary of the meeting (key topics discussed)
2. Decisions made during the meeting
3. Action items with assignees and deadlines if mentioned
4. List of attendees mentioned

Meeting transcript:
{safe_transcript}

You must respond using the structured output format provided. Do not include any text outside the structured response.
"""

        structured_llm = llm.with_structured_output(MinutesOutput)
        response = await asyncio.wait_for(structured_llm.ainvoke(prompt), timeout=30)

        summary = response.summary
        decisions = response.decisions
        action_items = [item.model_dump() for item in response.action_items]
        attendees = response.attendees

        usage = {}
        if hasattr(response, "usage_metadata") and response.usage_metadata:
            usage = {
                "prompt_tokens": response.usage_metadata.get("input_tokens", 0),
                "completion_tokens": response.usage_metadata.get("output_tokens", 0),
                "total_tokens": response.usage_metadata.get("total_tokens", 0),
            }

        minutes = {
            "summary": summary,
            "decisions": decisions,
            "action_items": action_items,
            "attendees": attendees,
        }

        return {
            "minutes": minutes,
            "decisions": decisions,
            "action_items": action_items,
            "attendees": attendees,
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
        return {
            "minutes": minutes,
            "decisions": state.get("decisions", []),
            "action_items": state.get("action_items", []),
            "attendees": state.get("attendees", []),
        }
    except Exception as e:
        logger.exception("format_output_node failed")
        return {"error": {"code": "FORMAT_OUTPUT_FAILED", "message": str(e), "retryable": False}}


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
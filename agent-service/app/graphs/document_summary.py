"""Document Summary Graph.
Conversational agent graph for document summarization.
Flow: process_input → generate_summary → format_output → END

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


class SummaryOutput(BaseModel):
    summary: str = Field(description="Concise summary of the document (3-5 sentences)")
    key_findings: list[str] = Field(description="Key findings — bullet points of the most important revelations")
    action_items: list[str] = Field(description="Action items — any tasks or to-dos mentioned")


class DocumentSummaryState(TypedDict):
    trace_id: str
    workflow_instance_id: str
    node_instance_id: str
    messages: Optional[list[dict[str, Any]]]
    document_content: Optional[str]
    summary: Optional[str]
    action_items: Optional[list[str]]
    key_findings: Optional[list[str]]
    error: Optional[dict[str, Any]]
    usage: Optional[dict[str, Any]]


def build_document_summary_state(body: dict[str, Any]) -> dict[str, Any]:
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
        "document_content": content,
        "summary": None,
        "action_items": None,
        "key_findings": None,
        "error": None,
        "usage": None,
    }


async def process_input_node(state: DocumentSummaryState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    try:
        content = state.get("document_content", "") or ""
        messages = state.get("messages") or []
        if messages and not content:
            last_msg = messages[-1]
            content = last_msg.get("content", "") if isinstance(last_msg, dict) else str(last_msg)
        return {"document_content": content, "messages": messages}
    except Exception as e:
        logger.exception("process_input_node failed")
        return {"error": {"code": "PROCESS_INPUT_FAILED", "message": str(e), "retryable": True}}


async def generate_summary_node(state: DocumentSummaryState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    try:
        llm = get_llm(temperature=0.0)
        content = state.get("document_content", "") or ""
        safe_content = sanitize_user_input(content)

        prompt = f"""You are an expert document summarizer. Given the following document text, produce:
1. A concise summary (3-5 sentences)
2. Key findings (bullet points of the most important revelations)
3. Action items (any tasks or to-dos mentioned)

Document text:
{safe_content}

You must respond using the structured output format provided. Do not include any text outside the structured response.
"""

        structured_llm = llm.with_structured_output(SummaryOutput)
        response = await asyncio.wait_for(structured_llm.ainvoke(prompt), timeout=30)

        summary = response.summary
        key_findings = response.key_findings
        action_items = response.action_items

        usage = {}
        if hasattr(response, "usage_metadata") and response.usage_metadata:
            usage = {
                "prompt_tokens": response.usage_metadata.get("input_tokens", 0),
                "completion_tokens": response.usage_metadata.get("output_tokens", 0),
                "total_tokens": response.usage_metadata.get("total_tokens", 0),
            }

        return {
            "summary": summary,
            "key_findings": key_findings,
            "action_items": action_items,
            "usage": usage,
        }
    except Exception as e:
        logger.exception("generate_summary_node failed")
        return {"error": {"code": "GENERATE_SUMMARY_FAILED", "message": str(e), "retryable": True}}


async def format_output_node(state: DocumentSummaryState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    try:
        return {
            "summary": state.get("summary", ""),
            "action_items": state.get("action_items", []),
            "key_findings": state.get("key_findings", []),
        }
    except Exception as e:
        logger.exception("format_output_node failed")
        return {"error": {"code": "FORMAT_OUTPUT_FAILED", "message": str(e), "retryable": False}}


def build_document_summary_graph(checkpointer: Any | None = None) -> Any:
    graph = StateGraph(DocumentSummaryState)

    graph.add_node("process_input", process_input_node)
    graph.add_node("generate_summary", generate_summary_node)
    graph.add_node("format_output", format_output_node)

    graph.set_entry_point("process_input")
    graph.add_edge("process_input", "generate_summary")
    graph.add_edge("generate_summary", "format_output")
    graph.add_edge("format_output", END)

    return graph.compile(checkpointer=checkpointer)
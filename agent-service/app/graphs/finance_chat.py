"""Finance Chat Graph.
Conversational finance chat agent that wraps/enhances the existing report flow.
Flow: process_input → analyze_query → generate_response → format_output → END

Built with LangGraph StateGraph.
"""

from __future__ import annotations

import asyncio
import json
import logging
from typing import Any, Optional

from langgraph.graph import END, StateGraph
from typing_extensions import TypedDict

from app.core.llm import get_llm

logger = logging.getLogger(__name__)


class FinanceChatState(TypedDict):
    trace_id: str
    workflow_instance_id: str
    node_instance_id: str
    messages: Optional[list[dict[str, Any]]]
    user_query: Optional[str]
    analysis: Optional[dict[str, Any]]
    response: Optional[str]
    error: Optional[dict[str, Any]]
    usage: Optional[dict[str, Any]]


def build_finance_chat_state(body: dict[str, Any]) -> dict[str, Any]:
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
        "user_query": content,
        "analysis": None,
        "response": None,
        "error": None,
        "usage": None,
    }


async def process_input_node(state: FinanceChatState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    try:
        query = state.get("user_query", "") or ""
        messages = state.get("messages") or []
        if messages and not query:
            last_msg = messages[-1]
            query = last_msg.get("content", "") if isinstance(last_msg, dict) else str(last_msg)
        return {"user_query": query, "messages": messages}
    except Exception as e:
        logger.exception("process_input_node failed")
        return {"error": {"code": "PROCESS_INPUT_FAILED", "message": str(e), "retryable": True}}


async def analyze_query_node(state: FinanceChatState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    try:
        llm = get_llm(temperature=0.1)
        query = state.get("user_query", "") or ""

        prompt = f"""You are a financial analysis expert. Analyze the following user query and identify:
1. The type of financial analysis needed (e.g., profitability, liquidity, risk assessment, valuation, trend analysis, etc.)
2. Key financial metrics or concepts relevant to the query
3. Suggested data sources or approaches to answer the query
4. Confidence level in the analysis (high/medium/low)

User query: "{query}"

Respond in JSON format with these keys:
- "analysis_type": string
- "key_metrics": array of strings
- "approach": string
- "confidence": string (high/medium/low)
"""

        response = await asyncio.wait_for(llm.ainvoke(prompt), timeout=30)
        response_text = response.content if hasattr(response, "content") else str(response)

        try:
            parsed = json.loads(response_text)
        except json.JSONDecodeError:
            parsed = {
                "analysis_type": "general",
                "key_metrics": [],
                "approach": response_text,
                "confidence": "low",
            }

        usage = {}
        if hasattr(response, "usage_metadata") and response.usage_metadata:
            usage = {
                "prompt_tokens": response.usage_metadata.get("input_tokens", 0),
                "completion_tokens": response.usage_metadata.get("output_tokens", 0),
                "total_tokens": response.usage_metadata.get("total_tokens", 0),
            }

        return {
            "analysis": parsed,
            "usage": usage,
        }
    except Exception as e:
        logger.exception("analyze_query_node failed")
        return {"error": {"code": "ANALYZE_QUERY_FAILED", "message": str(e), "retryable": True}}


async def generate_response_node(state: FinanceChatState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    try:
        llm = get_llm(temperature=0.3)
        query = state.get("user_query", "") or ""
        analysis = state.get("analysis") or {}

        analysis_type = analysis.get("analysis_type", "general")
        key_metrics = analysis.get("key_metrics", [])
        approach = analysis.get("approach", "")
        confidence = analysis.get("confidence", "low")

        prompt = f"""You are a helpful financial assistant. Based on the following analysis, generate a natural, conversational response to the user's query.

User query: "{query}"

Analysis:
- Type: {analysis_type}
- Key metrics: {', '.join(key_metrics) if key_metrics else 'N/A'}
- Approach: {approach}
- Confidence: {confidence}

Provide a clear, helpful response that:
1. Acknowledges the user's question
2. Provides relevant financial insights based on the analysis
3. Suggests next steps or data needed if appropriate
4. Is concise and professional

Response:"""

        response = await asyncio.wait_for(llm.ainvoke(prompt), timeout=30)
        response_text = response.content if hasattr(response, "content") else str(response)

        usage = {}
        if hasattr(response, "usage_metadata") and response.usage_metadata:
            usage = {
                "prompt_tokens": response.usage_metadata.get("input_tokens", 0),
                "completion_tokens": response.usage_metadata.get("output_tokens", 0),
                "total_tokens": response.usage_metadata.get("total_tokens", 0),
            }

        merged_usage = _merge_usage(state.get("usage"), usage)

        return {
            "response": response_text,
            "usage": merged_usage,
        }
    except Exception as e:
        logger.exception("generate_response_node failed")
        return {"error": {"code": "GENERATE_RESPONSE_FAILED", "message": str(e), "retryable": True}}


async def format_output_node(state: FinanceChatState) -> dict[str, Any]:
    if state.get("error"):
        return {}
    try:
        analysis = state.get("analysis") or {}
        return {
            "response": state.get("response", ""),
            "analysis_type": analysis.get("analysis_type", "general"),
            "confidence": analysis.get("confidence", "low"),
        }
    except Exception as e:
        logger.exception("format_output_node failed")
        return {"error": {"code": "FORMAT_OUTPUT_FAILED", "message": str(e), "retryable": False}}


def _merge_usage(existing: Optional[dict], current: dict) -> dict:
    if not existing:
        return current
    return {
        "prompt_tokens": existing.get("prompt_tokens", 0) + current.get("prompt_tokens", 0),
        "completion_tokens": existing.get("completion_tokens", 0) + current.get("completion_tokens", 0),
        "total_tokens": existing.get("total_tokens", 0) + current.get("total_tokens", 0),
    }


def build_finance_chat_graph(checkpointer: Any | None = None) -> Any:
    graph = StateGraph(FinanceChatState)

    graph.add_node("process_input", process_input_node)
    graph.add_node("analyze_query", analyze_query_node)
    graph.add_node("generate_response", generate_response_node)
    graph.add_node("format_output", format_output_node)

    graph.set_entry_point("process_input")
    graph.add_edge("process_input", "analyze_query")
    graph.add_edge("analyze_query", "generate_response")
    graph.add_edge("generate_response", "format_output")
    graph.add_edge("format_output", END)

    return graph.compile(checkpointer=checkpointer)
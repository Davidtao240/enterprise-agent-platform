import logging
import uuid
from contextlib import asynccontextmanager

from dotenv import load_dotenv

load_dotenv()

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from app.registry.graph_registry import get_graph, list_graphs

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(name)s] %(levelname)s: %(message)s")
logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(app: FastAPI):
    # TODO: Init MinIO client, Qdrant client on startup
    logger.info("Agent Service started. Registered graphs: %s", list_graphs())
    yield


app = FastAPI(title="Enterprise Agent Service", version="1.0.0", lifespan=lifespan)


@app.get("/health")
async def health():
    return {"status": "ok", "graphs": list_graphs()}


@app.post("/internal/v1/agent-runs")
async def run_agent_graph(request: Request):
    """Unified agent graph execution endpoint.
    Called by Go Agent Gateway (POST /internal/v1/agent-runs).

    Request body follows AGENT_IO_CONTRACT.md:
    {trace_id, business_app_code, workflow_template_key, graph_key,
     workflow_instance_id, node_instance_id, input, context}

    Response follows the same contract:
    {run_id, graph_key, status, output, usage, error}
    """
    body = await request.json()
    trace_id = body.get("trace_id", str(uuid.uuid4()))
    graph_key = body.get("graph_key")
    run_id = str(uuid.uuid4())

    logger.info("Agent run requested: graph_key=%s trace_id=%s run_id=%s", graph_key, trace_id, run_id)

    if not graph_key:
        return JSONResponse(
            status_code=400,
            content={
                "run_id": run_id,
                "graph_key": "",
                "status": "failed",
                "output": {},
                "usage": {},
                "error": {"code": "GRAPH_NOT_FOUND", "message": "graph_key is required"},
            },
        )

    # Lookup graph
    try:
        graph = get_graph(graph_key)
    except KeyError:
        logger.warning("Graph not found: %s", graph_key)
        return JSONResponse(
            status_code=404,
            content={
                "run_id": run_id,
                "graph_key": graph_key,
                "status": "failed",
                "output": {},
                "usage": {},
                "error": {"code": "GRAPH_NOT_FOUND", "message": f"Graph '{graph_key}' not found"},
            },
        )
    except Exception as e:
        logger.exception("Unexpected error getting graph %s", graph_key)
        return JSONResponse(
            status_code=500,
            content={
                "run_id": run_id,
                "graph_key": graph_key,
                "status": "failed",
                "output": {},
                "usage": {},
                "error": {"code": "GRAPH_EXECUTION_FAILED", "message": str(e)},
            },
        )

    # Build initial state from the Go AgentRunRequest
    raw_input = body.get("input", {})
    workflow_input = raw_input.get("workflow_input", {})
    if isinstance(workflow_input, str):
        import json
        try:
            workflow_input = json.loads(workflow_input)
        except (json.JSONDecodeError, TypeError):
            workflow_input = {}

    file_id = workflow_input.get("file_id") or raw_input.get("file_id")
    workflow_instance_id = body.get("workflow_instance_id", "")
    node_instance_id = body.get("node_instance_id", "")

    initial_state = {
        "trace_id": trace_id,
        "workflow_instance_id": workflow_instance_id,
        "node_instance_id": node_instance_id,
        "file_id": file_id,
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

    # Execute the graph
    try:
        config = {"configurable": {"thread_id": trace_id}}
        final_state = await graph.ainvoke(initial_state, config)
        logger.info("Graph execution completed: graph_key=%s run_id=%s", graph_key, run_id)
    except Exception as e:
        logger.exception("Graph execution failed for %s", graph_key)
        return JSONResponse(
            status_code=500,
            content={
                "run_id": run_id,
                "graph_key": graph_key,
                "status": "failed",
                "output": {},
                "usage": {},
                "error": {"code": "GRAPH_EXECUTION_FAILED", "message": str(e)},
            },
        )

    # Build the response from final state
    has_error = final_state.get("error") is not None
    validation_result = final_state.get("validation_result") or {}
    validation_failed = (
        bool(validation_result) and not validation_result.get("valid", True)
    )

    status = "failed" if (has_error or validation_failed) else "succeeded"

    output = {
        "summary": (final_state.get("review_summary") or {}).get("summary", ""),
        "key_metrics": (final_state.get("analysis_result") or {}).get("key_metrics", {}),
        "warnings": _collect_all_warnings(final_state),
        "report": (final_state.get("report") or {}).get("report", {}),
        "result_file_id": (final_state.get("report") or {}).get("result_file_id"),
        "review_suggestions": (final_state.get("review_summary") or {}).get("review_suggestions", []),
    }

    usage = final_state.get("usage") or {}

    error = final_state.get("error")
    if not error and validation_failed:
        error = {
            "code": "SCHEMA_VALIDATION_FAILED",
            "message": "Data validation failed. See warnings for details.",
        }

    return {
        "run_id": run_id,
        "graph_key": graph_key,
        "status": status,
        "output": output,
        "usage": usage,
        "error": error,
    }


def _collect_all_warnings(state: dict) -> list[dict]:
    """Collect warnings from all stages with deduplication by message."""
    seen: set[str] = set()
    warnings: list[dict] = []

    def add_w(w: dict) -> None:
        msg = w.get("message", "")
        if msg and msg not in seen:
            seen.add(msg)
            warnings.append(w)

    for w in (state.get("_load_warnings") or []):
        if isinstance(w, dict):
            add_w(w)

    # Only take warnings from the final analysis stage (already aggregates upstream)
    analysis = state.get("analysis_result") or {}
    for w in analysis.get("warnings", []):
        if isinstance(w, dict):
            add_w(w)
        elif isinstance(w, str):
            add_w({"level": "info", "message": w})

    review = state.get("review_summary") or {}
    for w in review.get("warnings", []):
        if isinstance(w, str):
            add_w({"level": "medium", "message": w})
        elif isinstance(w, dict):
            add_w(w)

    return warnings

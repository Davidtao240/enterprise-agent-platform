import logging
import os
import uuid
from contextlib import asynccontextmanager
from pathlib import Path

from dotenv import load_dotenv

load_dotenv()

from fastapi import Depends, FastAPI, HTTPException, Request, status
from fastapi.responses import JSONResponse
from langgraph.checkpoint.sqlite.aio import AsyncSqliteSaver

from app.output_envelope import build_run_envelope
from app.registry.graph_registry import (
    build_graph_initial_state,
    configure_graphs,
    get_graph,
    list_graphs,
)
from app.runtime.events import RuntimeEventDispatcher
from app.runtime.models import (
    AcceptedRunResponse,
    CancelRunRequest,
    ResumeRunRequest,
    StartRunRequest,
)
from app.runtime.service import RuntimeV2Service
from app.runtime.store import RuntimeStore, RuntimeStoreError
from app.core.trace_client import TraceEventPoster
from app.core.embedding_endpoint import router as embedding_router
from app.core.internal_auth import require_internal_service
from app.core.llm_gateway_router import router as llm_gateway_router

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(name)s] %(levelname)s: %(message)s")
logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(app: FastAPI):
    checkpoint_path = os.getenv(
        "CHECKPOINT_DB_PATH", "/tmp/enterprise-agent-platform/agent-checkpoints.sqlite3"
    )
    Path(checkpoint_path).parent.mkdir(parents=True, exist_ok=True)
    runtime_store = RuntimeStore(checkpoint_path)
    await runtime_store.setup()

    async with AsyncSqliteSaver.from_conn_string(checkpoint_path) as checkpointer:
        await checkpointer.conn.execute("PRAGMA journal_mode = WAL")
        await checkpointer.conn.execute("PRAGMA busy_timeout = 30000")
        await checkpointer.setup()
        configure_graphs(checkpointer)

        dispatcher = RuntimeEventDispatcher(
            runtime_store,
            os.getenv("RUNTIME_EVENT_URL", ""),
            os.getenv("INTERNAL_SERVICE_TOKEN", ""),
        )
        # M5-A: L3 Model Turn trace 上报 (未配置 TRACE_EVENT_URL 时自动禁用)
        trace_poster = TraceEventPoster(
            os.getenv("TRACE_EVENT_URL", ""),
            os.getenv("INTERNAL_SERVICE_TOKEN", ""),
        )
        runtime = RuntimeV2Service(runtime_store, get_graph, build_graph_initial_state, trace_poster)
        app.state.runtime_v2 = runtime
        app.state.runtime_store = runtime_store
        app.state.runtime_event_dispatcher = dispatcher
        await dispatcher.start()
        await runtime.recover()
        logger.info(
            "Agent Service started. Registered graphs: %s; checkpoint=%s; trace_events=%s",
            list_graphs(),
            checkpoint_path,
            trace_poster.enabled(),
        )
        try:
            yield
        finally:
            await runtime.shutdown()
            await dispatcher.stop()
            await trace_poster.aclose()
            configure_graphs()


app = FastAPI(title="Enterprise Agent Service", version="1.0.0", lifespan=lifespan)

app.include_router(embedding_router)
app.include_router(llm_gateway_router)


@app.get("/health")
async def health():
    return {"status": "ok", "graphs": list_graphs()}


def _runtime_service(request: Request) -> RuntimeV2Service:
    return request.app.state.runtime_v2


def _raise_runtime_error(exc: Exception) -> None:
    if isinstance(exc, RuntimeStoreError):
        raise HTTPException(
            status_code=exc.status_code,
            detail={"code": exc.code, "message": exc.message},
        ) from exc
    if isinstance(exc, KeyError):
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail={"code": "GRAPH_NOT_FOUND", "message": str(exc)},
        ) from exc
    raise exc


@app.post(
    "/internal/v2/agent-runs",
    response_model=AcceptedRunResponse,
    status_code=status.HTTP_202_ACCEPTED,
    dependencies=[Depends(require_internal_service)],
)
async def start_durable_run(
    body: StartRunRequest, request: Request
) -> AcceptedRunResponse:
    try:
        return await _runtime_service(request).start(body)
    except (RuntimeStoreError, KeyError) as exc:
        _raise_runtime_error(exc)
        raise AssertionError("unreachable")


@app.post(
    "/internal/v2/agent-runs/{run_id}/resume",
    response_model=AcceptedRunResponse,
    status_code=status.HTTP_202_ACCEPTED,
    dependencies=[Depends(require_internal_service)],
)
async def resume_durable_run(
    run_id: str, body: ResumeRunRequest, request: Request
) -> AcceptedRunResponse:
    if body.run_id != run_id:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail={"code": "RUN_ID_CONFLICT", "message": "path and body run_id differ"},
        )
    try:
        return await _runtime_service(request).resume(body)
    except RuntimeStoreError as exc:
        _raise_runtime_error(exc)
        raise AssertionError("unreachable")


@app.post(
    "/internal/v2/agent-runs/{run_id}/cancel",
    response_model=AcceptedRunResponse,
    status_code=status.HTTP_202_ACCEPTED,
    dependencies=[Depends(require_internal_service)],
)
async def cancel_durable_run(
    run_id: str, body: CancelRunRequest, request: Request
) -> AcceptedRunResponse:
    if body.run_id != run_id:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail={"code": "RUN_ID_CONFLICT", "message": "path and body run_id differ"},
        )
    try:
        return await _runtime_service(request).cancel(body)
    except RuntimeStoreError as exc:
        _raise_runtime_error(exc)
        raise AssertionError("unreachable")


@app.post("/internal/v1/agent-runs", dependencies=[Depends(require_internal_service)])
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
    run_id = _resolve_run_id(body)

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

    # Build initial state from the unchanged Go AgentRunRequest.
    initial_state = build_graph_initial_state(graph_key, None, body)

    # Execute the graph
    try:
        # Each V1 durable attempt has an isolated checkpoint thread. Reusing a
        # Workflow trace here could make a later node retry inherit a completed
        # checkpoint from an earlier Run.
        config = {"configurable": {"thread_id": run_id}}
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

    return _build_agent_run_response(final_state, run_id, graph_key)


def _resolve_run_id(body: dict) -> str:
    """Echo the optional Go control-plane Run ID, preserving legacy callers."""
    requested_run_id = body.get("run_id")
    if isinstance(requested_run_id, str) and requested_run_id.strip():
        return requested_run_id
    return str(uuid.uuid4())


def _build_agent_run_response(
    final_state: dict,
    run_id: str,
    graph_key: str,
) -> dict:
    """Serialize graph state into the stable Agent Run envelope.

    envelope 构造复用 app.output_envelope(V1/V2 同形契约的唯一构造点)。
    """
    envelope = build_run_envelope(final_state)
    return {
        "run_id": run_id,
        "graph_key": graph_key,
        "status": envelope["status"],
        "output": envelope["output"],
        "usage": envelope["usage"],
        "error": envelope["error"],
    }

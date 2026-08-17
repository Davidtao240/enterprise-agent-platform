"""Graph Registry — maps graph_key to compiled LangGraph graphs.
New business scenarios register their graph here. No LLM-based routing.

Usage:
    from app.registry.graph_registry import get_graph

    graph = get_graph("finance_operating_report_graph")
"""

from typing import Any

from app.graphs.finance_operating_report import (
    build_finance_operating_report_graph,
    build_finance_operating_report_state,
)


FINANCE_GRAPH_KEY = "finance_operating_report_graph"
FINANCE_GRAPH_VERSION = "1.0.0"

_GRAPHS: dict[tuple[str, str], Any] = {}
_DEFAULT_VERSIONS = {
    FINANCE_GRAPH_KEY: FINANCE_GRAPH_VERSION,
}
_STATE_BUILDERS = {
    (FINANCE_GRAPH_KEY, FINANCE_GRAPH_VERSION): build_finance_operating_report_state,
}


def configure_graphs(checkpointer: Any | None = None) -> None:
    """Compile immutable graph versions against the active checkpoint backend."""
    global _GRAPHS
    _GRAPHS = {
        (FINANCE_GRAPH_KEY, FINANCE_GRAPH_VERSION):
            build_finance_operating_report_graph(checkpointer=checkpointer),
    }


def get_graph(graph_key: str, graph_version: str | None = None) -> Any:
    version = graph_version or _DEFAULT_VERSIONS.get(graph_key)
    if version is None:
        raise KeyError(f"Graph '{graph_key}' not found in registry.")
    graph = _GRAPHS.get((graph_key, version))
    if graph is None:
        raise KeyError(f"Graph '{graph_key}@{version}' not found in registry.")
    return graph


def list_graphs() -> list[str]:
    return sorted({key for key, _ in _GRAPHS})


def build_graph_initial_state(
    graph_key: str, graph_version: str | None, body: dict[str, Any]
) -> dict[str, Any]:
    version = graph_version or _DEFAULT_VERSIONS.get(graph_key)
    builder = _STATE_BUILDERS.get((graph_key, version)) if version is not None else None
    if builder is None:
        requested = version or graph_version or "default"
        raise KeyError(f"State builder '{graph_key}@{requested}' not found in registry.")
    return builder(body)


# Keep direct imports and unit tests usable outside FastAPI lifespan. Startup
# replaces these instances with checkpointer-backed compiled graphs.
configure_graphs()

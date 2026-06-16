"""Graph Registry — maps graph_key to compiled LangGraph graphs.
New business scenarios register their graph here. No LLM-based routing.

Usage:
    from app.registry.graph_registry import get_graph

    graph = get_graph("finance_operating_report_graph")
"""

from typing import Any

from app.graphs.finance_operating_report import build_finance_operating_report_graph


_finance_graph = build_finance_operating_report_graph()


_GRAPHS: dict[str, Any] = {
    "finance_operating_report_graph": _finance_graph,
}


def get_graph(graph_key: str) -> Any:
    graph = _GRAPHS.get(graph_key)
    if graph is None:
        raise KeyError(f"Graph '{graph_key}' not found in registry.")
    return graph


def list_graphs() -> list[str]:
    return list(_GRAPHS.keys())

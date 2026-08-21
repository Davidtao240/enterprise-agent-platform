"""Graph Registry — maps graph_key to compiled LangGraph graphs.

Two registration layers (ADR-010):
1. Built-in graphs: platform finance scenarios, declared in code below.
2. Installed graphs: business-domain packages discovered from the whitelisted
   ``app.installed_graphs`` package at runtime. Adding/removing a module there
   plus ``reload_graphs()`` makes it available without a service restart.

Routing stays explicit: callers always resolve by (graph_key, graph_version).
No LLM-based routing. New business scenarios register their graph either as a
built-in import or as an installed module — never by editing platform code.

Usage:
    from app.registry.graph_registry import get_graph

    graph = get_graph("finance_operating_report_graph")
"""

from __future__ import annotations

import importlib
import logging
import pkgutil
from dataclasses import dataclass
from typing import Any, Callable

from app.graphs.finance_operating_report import (
    build_finance_operating_report_graph,
    build_finance_operating_report_state,
)
from app.graphs.document_summary import (
    build_document_summary_graph,
    build_document_summary_state,
)
from app.graphs.meeting_minutes import (
    build_meeting_minutes_graph,
    build_meeting_minutes_state,
)
from app.graphs.finance_chat import (
    build_finance_chat_graph,
    build_finance_chat_state,
)
from app.graphs.finance_collaborative_report import (
    build_finance_collaborative_report_graph,
    build_collaborative_report_state,
)

logger = logging.getLogger(__name__)

FINANCE_GRAPH_KEY = "finance_operating_report_graph"
FINANCE_GRAPH_VERSION = "1.0.0"

DOCUMENT_SUMMARY_GRAPH_KEY = "document_summary_graph"
DOCUMENT_SUMMARY_GRAPH_VERSION = "1.0.0"

MEETING_MINUTES_GRAPH_KEY = "meeting_minutes_graph"
MEETING_MINUTES_GRAPH_VERSION = "1.0.0"

FINANCE_CHAT_GRAPH_KEY = "finance_chat_graph"
FINANCE_CHAT_GRAPH_VERSION = "1.0.0"

FINANCE_COLLABORATIVE_REPORT_GRAPH_KEY = "finance_collaborative_report_graph"
FINANCE_COLLABORATIVE_REPORT_GRAPH_VERSION = "1.0.0"

# Whitelisted package for dynamically installed business graphs (ADR-010).
INSTALLED_GRAPHS_PACKAGE = "app.installed_graphs"


@dataclass(frozen=True)
class GraphSpec:
    """One registrable graph: an immutable (key, version) compile unit."""

    key: str
    version: str
    build_graph: Callable[[Any | None], Any]
    build_state: Callable[[dict[str, Any]], dict[str, Any]] | None = None
    source: str = "builtin"


_BUILTIN_SPECS: tuple[GraphSpec, ...] = (
    GraphSpec(
        key=FINANCE_GRAPH_KEY,
        version=FINANCE_GRAPH_VERSION,
        build_graph=build_finance_operating_report_graph,
        build_state=build_finance_operating_report_state,
    ),
    GraphSpec(
        key=DOCUMENT_SUMMARY_GRAPH_KEY,
        version=DOCUMENT_SUMMARY_GRAPH_VERSION,
        build_graph=build_document_summary_graph,
        build_state=build_document_summary_state,
    ),
    GraphSpec(
        key=MEETING_MINUTES_GRAPH_KEY,
        version=MEETING_MINUTES_GRAPH_VERSION,
        build_graph=build_meeting_minutes_graph,
        build_state=build_meeting_minutes_state,
    ),
    GraphSpec(
        key=FINANCE_CHAT_GRAPH_KEY,
        version=FINANCE_CHAT_GRAPH_VERSION,
        build_graph=build_finance_chat_graph,
        build_state=build_finance_chat_state,
    ),
    GraphSpec(
        key=FINANCE_COLLABORATIVE_REPORT_GRAPH_KEY,
        version=FINANCE_COLLABORATIVE_REPORT_GRAPH_VERSION,
        build_graph=build_finance_collaborative_report_graph,
        build_state=build_collaborative_report_state,
    ),
)

_GRAPHS: dict[tuple[str, str], Any] = {}
_DEFAULT_VERSIONS: dict[str, str] = {}
_STATE_BUILDERS: dict[tuple[str, str], Callable[[dict[str, Any]], dict[str, Any]]] = {}
_INSTALLED_SPECS: dict[str, GraphSpec] = {}


def _compile_specs(specs: tuple[GraphSpec, ...] | list[GraphSpec], checkpointer: Any | None) -> dict[tuple[str, str], Any]:
    return {
        (spec.key, spec.version): spec.build_graph(checkpointer=checkpointer)
        for spec in specs
    }


def _register(specs: tuple[GraphSpec, ...] | list[GraphSpec], checkpointer: Any | None) -> None:
    for spec in specs:
        _DEFAULT_VERSIONS[spec.key] = spec.version
        if spec.build_state is not None:
            _STATE_BUILDERS[(spec.key, spec.version)] = spec.build_state


def discover_installed_graphs(
    package_name: str = INSTALLED_GRAPHS_PACKAGE,
) -> tuple[list[GraphSpec], list[dict[str, str]]]:
    """Scan the whitelisted installed-graphs package and import valid modules.

    Returns (valid_specs, invalid_modules). A module that violates the graph
    contract is skipped and reported — one broken third-party package must not
    block the rest of the marketplace (ADR-010 §2).
    """
    valid: list[GraphSpec] = []
    invalid: list[dict[str, str]] = []
    try:
        package = importlib.import_module(package_name)
    except ModuleNotFoundError:
        return [], []

    for module_info in pkgutil.iter_modules(getattr(package, "__path__", [])):
        module_name = f"{package_name}.{module_info.name}"
        try:
            module = importlib.import_module(module_name)
            # Pick up edited module files on reload (install-then-edit flows).
            module = importlib.reload(module)
            graph_key = getattr(module, "GRAPH_KEY", None)
            graph_version = getattr(module, "GRAPH_VERSION", None)
            build_graph = getattr(module, "build_graph", None)
            if not isinstance(graph_key, str) or not graph_key.strip():
                raise TypeError("GRAPH_KEY must be a non-empty string")
            if not isinstance(graph_version, str) or not graph_version.strip():
                raise TypeError("GRAPH_VERSION must be a non-empty string")
            if not callable(build_graph):
                raise TypeError("build_graph must be callable")
            build_state = getattr(module, "build_state", None)
            if build_state is not None and not callable(build_state):
                raise TypeError("build_state must be callable when present")
            valid.append(
                GraphSpec(
                    key=graph_key,
                    version=graph_version,
                    build_graph=build_graph,
                    build_state=build_state,
                    source=module_name,
                )
            )
        except Exception as exc:  # noqa: BLE001 — skip-and-report per ADR-010
            logger.warning("Installed graph module %s rejected: %s", module_name, exc)
            invalid.append(
                {"module": module_name, "error": f"{type(exc).__name__}: {exc}"}
            )
    return valid, invalid


def configure_graphs(
    checkpointer: Any | None = None,
    package_name: str = INSTALLED_GRAPHS_PACKAGE,
) -> None:
    """Compile immutable graph versions against the active checkpoint backend.

    Idempotent: safe to call at startup and on every reload.
    """
    global _GRAPHS, _last_discovery_invalid
    installed, _last_discovery_invalid = discover_installed_graphs(package_name)
    _INSTALLED_SPECS.clear()
    for spec in installed:
        # Later modules win on duplicate keys; builtins are re-registered last
        # so platform graphs can never be shadowed by installed packages.
        _INSTALLED_SPECS[spec.key] = spec

    specs = list(_INSTALLED_SPECS.values()) + list(_BUILTIN_SPECS)
    _GRAPHS = _compile_specs(specs, checkpointer)
    _DEFAULT_VERSIONS.clear()
    _STATE_BUILDERS.clear()
    _register(specs, checkpointer)


def reload_graphs(
    checkpointer: Any | None = None,
    package_name: str = INSTALLED_GRAPHS_PACKAGE,
) -> dict[str, list[str]]:
    """Re-discover installed graphs and recompile the registry (ADR-010 §3).

    New runs resolve graphs after the swap; in-flight runs keep their already
    resolved compiled graph. Returns a before/after diff for the admin caller.
    """
    before = list_graphs()
    configure_graphs(checkpointer=checkpointer, package_name=package_name)
    after = list_graphs()
    return {
        "before": before,
        "after": after,
        "added": sorted(set(after) - set(before)),
        "removed": sorted(set(before) - set(after)),
        "invalid": [entry["module"] for entry in _last_discovery_invalid],
    }


# Last discovery's invalid-module report, surfaced through reload_graphs().
_last_discovery_invalid: list[dict[str, str]] = []


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

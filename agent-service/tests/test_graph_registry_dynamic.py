"""ADR-010: Graph Registry dynamic loading tests.

Covers:
- valid installed module discovered, compiled and resolvable via get_graph;
- contract-violating module skipped and reported without blocking siblings;
- module removal unregisters the graph (new runs get GRAPH_NOT_FOUND);
- builtin finance graphs always survive a reload;
- HTTP contract of POST /internal/admin/graphs/reload (auth + diff shape).
"""

from __future__ import annotations

import os
import sys
import tempfile
import textwrap
import unittest
import uuid

PACKAGE = "dyn_test_graphs"

VALID_GRAPH_MODULE = textwrap.dedent(
    """
    from typing import Any, TypedDict

    GRAPH_KEY = "echo_test_graph"
    GRAPH_VERSION = "1.0.0"


    class EchoState(TypedDict, total=False):
        echo: str


    def build_graph(checkpointer=None) -> Any:
        from langgraph.graph import END, START, StateGraph

        graph = StateGraph(EchoState)
        graph.add_node("echo", lambda state: {"echo": state.get("echo", "silent")})
        graph.add_edge(START, "echo")
        graph.add_edge("echo", END)
        return graph.compile(checkpointer=checkpointer)


    def build_state(body: dict) -> dict:
        return {"echo": (body.get("input") or {}).get("message", "")}
    """
)

INVALID_GRAPH_MODULE = textwrap.dedent(
    """
    # Missing GRAPH_KEY on purpose — must be skipped and reported (ADR-010 §2).
    GRAPH_VERSION = "1.0.0"


    def build_graph(checkpointer=None):
        raise AssertionError("must never be called")
    """
)


class GraphRegistryDynamicTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.package_dir = os.path.join(self.temp_dir.name, PACKAGE)
        os.makedirs(self.package_dir)
        with open(os.path.join(self.package_dir, "__init__.py"), "w") as fh:
            fh.write("")
        sys.path.insert(0, self.temp_dir.name)

    def tearDown(self) -> None:
        sys.path.remove(self.temp_dir.name)
        for name in list(sys.modules):
            if name == PACKAGE or name.startswith(f"{PACKAGE}."):
                sys.modules.pop(name, None)
        # Restore the real registry state for other test modules.
        from app.registry.graph_registry import configure_graphs

        configure_graphs()
        self.temp_dir.cleanup()

    def _write_module(self, name: str, source: str) -> None:
        with open(os.path.join(self.package_dir, f"{name}.py"), "w") as fh:
            fh.write(source)

    def _remove_module(self, name: str) -> None:
        os.remove(os.path.join(self.package_dir, f"{name}.py"))
        sys.modules.pop(f"{PACKAGE}.{name}", None)

    def test_valid_module_is_discovered_and_resolvable(self) -> None:
        from app.registry.graph_registry import (
            build_graph_initial_state,
            get_graph,
            reload_graphs,
        )

        self._write_module("echo_graph", VALID_GRAPH_MODULE)
        result = reload_graphs(package_name=PACKAGE)

        self.assertIn("echo_test_graph", result["added"])
        self.assertIn("echo_test_graph", result["after"])
        self.assertEqual([], result["invalid"])

        graph = get_graph("echo_test_graph", "1.0.0")
        self.assertIsNotNone(graph)
        state = build_graph_initial_state("echo_test_graph", None, {"input": {"message": "hi"}})
        self.assertEqual({"echo": "hi"}, state)

    def test_invalid_module_is_skipped_without_blocking_siblings(self) -> None:
        from app.registry.graph_registry import reload_graphs

        self._write_module("echo_graph", VALID_GRAPH_MODULE)
        self._write_module("broken_graph", INVALID_GRAPH_MODULE)
        result = reload_graphs(package_name=PACKAGE)

        self.assertIn("echo_test_graph", result["added"])
        self.assertEqual([f"{PACKAGE}.broken_graph"], result["invalid"])
        self.assertNotIn("broken_graph", result["after"])

    def test_module_removal_unregisters_graph(self) -> None:
        from app.registry.graph_registry import get_graph, reload_graphs

        self._write_module("echo_graph", VALID_GRAPH_MODULE)
        reload_graphs(package_name=PACKAGE)
        self.assertIsNotNone(get_graph("echo_test_graph"))

        self._remove_module("echo_graph")
        result = reload_graphs(package_name=PACKAGE)

        self.assertIn("echo_test_graph", result["removed"])
        self.assertNotIn("echo_test_graph", result["after"])
        with self.assertRaises(KeyError):
            get_graph("echo_test_graph")

    def test_builtin_graphs_survive_reload(self) -> None:
        from app.registry.graph_registry import (
            FINANCE_GRAPH_KEY,
            get_graph,
            reload_graphs,
        )

        result = reload_graphs(package_name=PACKAGE)
        self.assertIn(FINANCE_GRAPH_KEY, result["after"])
        self.assertIsNotNone(get_graph(FINANCE_GRAPH_KEY))


class GraphReloadHTTPContractTest(unittest.TestCase):
    def test_reload_endpoint_requires_internal_token(self) -> None:
        from fastapi.testclient import TestClient
        from unittest.mock import patch

        from app.main import app

        with tempfile.TemporaryDirectory() as temp_dir, patch.dict(
            os.environ,
            {
                "CHECKPOINT_DB_PATH": f"{temp_dir}/runtime.sqlite3",
                "INTERNAL_SERVICE_TOKEN": "reload-contract-token",
                "RUNTIME_EVENT_URL": "",
                "RUNTIME_DATABASE_URL": "",
                "RUNTIME_STORE_BACKEND": "",
            },
            clear=False,
        ):
            with TestClient(app) as client:
                unauthorized = client.post("/internal/admin/graphs/reload")
                self.assertEqual(401, unauthorized.status_code)
                self.assertEqual(
                    "SERVICE_AUTH_FAILED", unauthorized.json()["detail"]["code"]
                )

                headers = {"X-Internal-Service-Token": "reload-contract-token"}
                response = client.post("/internal/admin/graphs/reload", headers=headers)
                self.assertEqual(200, response.status_code)
                body = response.json()
                for field in ("before", "after", "added", "removed", "invalid"):
                    self.assertIn(field, body)
                # Built-in finance graphs must be listed after reload.
                self.assertIn("finance_operating_report_graph", body["after"])


if __name__ == "__main__":
    unittest.main()

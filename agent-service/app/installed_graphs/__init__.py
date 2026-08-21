"""Whitelisted package for dynamically installed business graphs (ADR-010).

Each module placed in this package is auto-discovered by the Graph Registry
and must declare the module-level contract:

    GRAPH_KEY: str            # e.g. "procurement_quote_review_graph"
    GRAPH_VERSION: str        # e.g. "1.0.0"
    def build_graph(checkpointer=None) -> compiled LangGraph graph
    def build_state(body: dict) -> dict   # optional initial-state builder

Contract-violating modules are skipped and reported via
POST /internal/admin/graphs/reload (they never block other modules).

Installing a new business-domain graph:
1. Drop the module file into this package (Go Agent Gallery install flow).
2. Call POST /internal/admin/graphs/reload with X-Internal-Service-Token.
3. Start runs against the new graph_key — no service restart, no platform
   code changes (M8 zero-code onboarding, Gate Fail-6).
"""

"""Shared Agent Run output envelope.

V1 (`/internal/v1/agent-runs` response) 与 Runtime V2 (`run.succeeded` 事件
payload) 必须产出同形 output envelope,保证 Go 侧与前端对两条执行路径看到
一致的最终输出契约。本模块是唯一构造点,V1 HTTP 响应与 V2 事件均复用。
"""


def build_run_envelope(final_state: dict) -> dict:
    """Serialize graph state into the stable Agent Run envelope.

    Returns a dict with keys: status, output, usage, error.
    """
    has_error = final_state.get("error") is not None
    validation_result = final_state.get("validation_result") or {}
    validation_failed = (
        bool(validation_result) and not validation_result.get("valid", True)
    )

    status = "failed" if (has_error or validation_failed) else "succeeded"

    output = build_run_output(final_state)
    usage = final_state.get("usage") or {}

    error = final_state.get("error")
    if not error and validation_failed:
        error = {
            "code": "SCHEMA_VALIDATION_FAILED",
            "message": "Data validation failed. See warnings for details.",
        }

    return {"status": status, "output": output, "usage": usage, "error": error}


def build_run_output(final_state: dict) -> dict:
    """Build the output summary payload from graph final state."""
    output = {
        "summary": (final_state.get("review_summary") or {}).get("summary", ""),
        "key_metrics": (final_state.get("analysis_result") or {}).get("key_metrics", {}),
        "warnings": collect_all_warnings(final_state),
        "report": (final_state.get("report") or {}).get("report", {}),
        "result_file_id": (final_state.get("report") or {}).get("result_file_id"),
        "review_suggestions": (final_state.get("review_summary") or {})
        .get("review_suggestions", []),
    }

    final_report = final_state.get("final_report")
    if final_report:
        output["collaborative_report"] = final_report

    return output


def collect_all_warnings(state: dict) -> list[dict]:
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

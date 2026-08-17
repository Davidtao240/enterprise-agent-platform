from __future__ import annotations

import os
import tempfile
import unittest
import uuid
from unittest.mock import patch

from fastapi.testclient import TestClient

from app.main import app


def _start_body(run_id: str) -> dict:
    return {
        "protocol_version": "2.0",
        "run_id": run_id,
        "thread_id": str(uuid.uuid4()),
        "trace_id": str(uuid.uuid4()),
        "business_app_code": "test",
        "graph": {"key": "missing_graph", "version": "1.0.0"},
        "configuration": {
            "agent_definition_version": "1.0.0",
            "profile_or_skill_version": "test@1.0.0",
            "model_config_version": "test@1.0.0",
        },
        "input": {},
        "trusted_context": {
            "user_id": str(uuid.uuid4()),
            "tenant_id": str(uuid.uuid4()),
        },
        "attempt": 1,
    }


class RuntimeV2HTTPContractTest(unittest.TestCase):
    def test_routes_require_service_auth_and_return_structured_errors(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir, patch.dict(
            os.environ,
            {
                "CHECKPOINT_DB_PATH": f"{temp_dir}/runtime.sqlite3",
                "INTERNAL_SERVICE_TOKEN": "http-contract-token",
                "RUNTIME_EVENT_URL": "",
            },
            clear=False,
        ):
            with TestClient(app) as client:
                run_id = str(uuid.uuid4())
                body = _start_body(run_id)
                unauthorized = client.post("/internal/v2/agent-runs", json=body)
                self.assertEqual(401, unauthorized.status_code)
                self.assertEqual("SERVICE_AUTH_FAILED", unauthorized.json()["detail"]["code"])

                headers = {"X-Internal-Service-Token": "http-contract-token"}
                missing_graph = client.post(
                    "/internal/v2/agent-runs", json=body, headers=headers
                )
                self.assertEqual(404, missing_graph.status_code)
                self.assertEqual("GRAPH_NOT_FOUND", missing_graph.json()["detail"]["code"])

                resume = client.post(
                    f"/internal/v2/agent-runs/{run_id}/resume",
                    headers=headers,
                    json={
                        "protocol_version": "2.0",
                        "run_id": run_id,
                        "interrupt_id": "missing-interrupt",
                        "expected_checkpoint_version": 1,
                        "idempotency_key": "resume-missing",
                        "resume_input": {"decision": "approved"},
                    },
                )
                self.assertEqual(404, resume.status_code)
                self.assertEqual("INTERRUPT_NOT_FOUND", resume.json()["detail"]["code"])

                cancel = client.post(
                    f"/internal/v2/agent-runs/{run_id}/cancel",
                    headers=headers,
                    json={
                        "protocol_version": "2.0",
                        "run_id": run_id,
                        "reason": "contract_test",
                        "requested_by": str(uuid.uuid4()),
                        "idempotency_key": "cancel-missing",
                    },
                )
                self.assertEqual(404, cancel.status_code)
                self.assertEqual("RUN_NOT_FOUND", cancel.json()["detail"]["code"])


if __name__ == "__main__":
    unittest.main()

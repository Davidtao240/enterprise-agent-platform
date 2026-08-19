"""
M8-C: Sample Sidecar Connector

A minimal sidecar implementing the Connector Protocol HTTP contract:
  POST /execute    → execute a capability
  POST /verify     → verify execution result
  GET  /health     → health check
  GET  /manifest   → connector manifest

Run: python sample_sidecar.py --port 8787
"""
import argparse
import json
import sys
import time
import uuid
from http.server import HTTPServer, BaseHTTPRequestHandler
from typing import Any, Dict, Optional


class SidecarHandler(BaseHTTPRequestHandler):
    connector_code = "sample_connector"
    version = "1.0.0"
    capabilities = [
        {"name": "echo", "kind": "read", "description": "Echo input back"},
        {"name": "timestamp", "kind": "read", "description": "Return current timestamp"},
        {"name": "create_ticket", "kind": "write", "description": "Create a mock ticket"},
    ]

    def _send_json(self, status: int, data: Dict[str, Any]):
        body = json.dumps(data).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _read_body(self) -> Dict[str, Any]:
        length = int(self.headers.get("Content-Length", 0))
        if length > 0:
            return json.loads(self.rfile.read(length))
        return {}

    # ── Endpoints ──────────────────────────────────────────

    def do_GET(self):
        if self.path == "/health":
            self._send_json(200, {
                "healthy": True,
                "detail": "ok",
                "version": self.version,
                "timestamp": time.isoformat(time.now()),
            })
        elif self.path == "/manifest":
            self._send_json(200, {
                "connector_code": self.connector_code,
                "version": self.version,
                "connector_type": "http_sidecar",
                "capabilities": self.capabilities,
                "auth_type": "none",
                "release_stage": "production",
            })
        else:
            self._send_json(404, {"error": "not found"})

    def do_POST(self):
        body = self._read_body()

        if self.path == "/execute":
            self._handle_execute(body)
        elif self.path == "/verify":
            self._handle_verify(body)
        else:
            self._send_json(404, {"error": "not found"})

    def _handle_execute(self, body: Dict[str, Any]):
        capability = body.get("capability", "")
        input_data = body.get("input", {})

        if capability == "echo":
            self._send_json(200, {
                "status": "succeeded",
                "output": {"echoed": input_data},
            })
        elif capability == "timestamp":
            self._send_json(200, {
                "status": "succeeded",
                "output": {"timestamp": time.isoformat(time.now())},
            })
        elif capability == "create_ticket":
            ext_id = str(uuid.uuid4())
            self._send_json(200, {
                "status": "succeeded",
                "output": {"ticket": input_data, "ticket_id": ext_id},
                "external_request_id": ext_id,
                "external_object_id": f"TICKET-{ext_id[:8].upper()}",
            })
        else:
            self._send_json(400, {
                "status": "failed",
                "error": {
                    "code": "CAPABILITY_NOT_FOUND",
                    "message": f"Unknown capability: {capability}",
                    "retryable": False,
                },
            })

    def _handle_verify(self, body: Dict[str, Any]):
        ext_id = body.get("external_request_id", "")
        self._send_json(200, {
            "confirmed": True,
            "detail": f"Ticket {ext_id} confirmed in external system",
            "observed": {"status": "completed", "external_id": ext_id},
        })

    def log_message(self, format, *args):
        print(f"[{time.isoformat(time.now())}] {args}")


def main():
    parser = argparse.ArgumentParser(description="Sample Sidecar Connector")
    parser.add_argument("--port", type=int, default=8787)
    parser.add_argument("--host", default="0.0.0.0")
    args = parser.parse_args()

    server = HTTPServer((args.host, args.port), SidecarHandler)
    print(f"Sidecar '{SidecarHandler.connector_code}@{SidecarHandler.version}' "
          f"listening on {args.host}:{args.port}")
    print(f"Capabilities: {[c['name'] for c in SidecarHandler.capabilities]}")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        server.shutdown()
        print("Sidecar stopped.")


if __name__ == "__main__":
    main()
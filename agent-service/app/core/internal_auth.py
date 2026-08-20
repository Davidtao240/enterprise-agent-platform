from __future__ import annotations

import os
import secrets

from fastapi import HTTPException, Request, status


def require_internal_service(request: Request) -> None:
    expected = os.getenv("INTERNAL_SERVICE_TOKEN", "")
    provided = request.headers.get("X-Internal-Service-Token", "")
    if not expected:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail={
                "code": "SERVICE_AUTH_NOT_CONFIGURED",
                "message": "internal service authentication is not configured",
            },
        )
    if not secrets.compare_digest(provided, expected):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail={"code": "SERVICE_AUTH_FAILED", "message": "invalid internal service identity"},
        )
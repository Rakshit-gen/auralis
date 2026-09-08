"""Platform error envelope for the Python services.

Every failure response is:

    {"error": {"code": "...", "message": "...", "request_id": "..."}}

Stack traces are never exposed to clients.
"""

from __future__ import annotations

from typing import Any

import structlog
from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

log = structlog.get_logger()


class ApiError(Exception):
    """An application error with an HTTP status and a stable machine code."""

    def __init__(self, status: int, code: str, message: str, fields: dict[str, str] | None = None):
        super().__init__(message)
        self.status = status
        self.code = code
        self.message = message
        self.fields = fields

    @classmethod
    def bad_request(cls, message: str, **fields: str) -> ApiError:
        return cls(400, "request.validation_failed", message, fields or None)

    @classmethod
    def unauthorized(cls, message: str = "authentication required") -> ApiError:
        return cls(401, "auth.unauthorized", message)

    @classmethod
    def forbidden(cls, message: str = "insufficient role") -> ApiError:
        return cls(403, "auth.forbidden", message)

    @classmethod
    def not_found(cls, message: str = "resource not found") -> ApiError:
        return cls(404, "request.not_found", message)

    @classmethod
    def conflict(cls, message: str) -> ApiError:
        return cls(409, "request.conflict", message)

    @classmethod
    def unavailable(cls, message: str) -> ApiError:
        return cls(502, "dependency.unavailable", message)

    @classmethod
    def internal(cls, message: str = "an unexpected error occurred") -> ApiError:
        return cls(500, "internal.unexpected", message)


def error_body(code: str, message: str, request_id: str, fields: dict[str, Any] | None = None) -> dict:
    body: dict[str, Any] = {"code": code, "message": message, "request_id": request_id}
    if fields:
        body["fields"] = fields
    return {"error": body}


def _request_id(request: Request) -> str:
    return request.headers.get("x-auralis-request-id") or request.headers.get("x-request-id") or ""


def install_exception_handlers(app: FastAPI) -> None:
    @app.exception_handler(ApiError)
    async def _api_error(request: Request, exc: ApiError) -> JSONResponse:
        return JSONResponse(
            status_code=exc.status,
            content=error_body(exc.code, exc.message, _request_id(request), exc.fields),
        )

    @app.exception_handler(RequestValidationError)
    async def _validation(request: Request, exc: RequestValidationError) -> JSONResponse:
        fields = {".".join(str(p) for p in e["loc"][1:]): e["msg"] for e in exc.errors()}
        return JSONResponse(
            status_code=400,
            content=error_body(
                "request.validation_failed",
                "request validation failed",
                _request_id(request),
                fields,
            ),
        )

    @app.exception_handler(StarletteHTTPException)
    async def _http(request: Request, exc: StarletteHTTPException) -> JSONResponse:
        code = {
            401: "auth.unauthorized",
            403: "auth.forbidden",
            404: "request.not_found",
        }.get(exc.status_code, "request.error")
        return JSONResponse(
            status_code=exc.status_code,
            content=error_body(code, str(exc.detail), _request_id(request)),
        )

    @app.exception_handler(Exception)
    async def _unhandled(request: Request, exc: Exception) -> JSONResponse:
        log.error("unhandled exception", path=request.url.path, error=str(exc), exc_info=exc)
        return JSONResponse(
            status_code=500,
            content=error_body(
                "internal.unexpected",
                "an unexpected error occurred",
                _request_id(request),
            ),
        )

"""FastAPI app factory with the standard middleware, health endpoints, and
metrics wiring, so each Python service's entrypoint stays small."""

from __future__ import annotations

import time
import uuid
from collections.abc import Awaitable, Callable

import structlog
from fastapi import FastAPI, Request, Response
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest
from starlette.middleware.cors import CORSMiddleware

from auralis_common.errors import install_exception_handlers
from auralis_common.logging import bind_request_context
from auralis_common.telemetry import observe_http

log = structlog.get_logger()

HealthCheck = Callable[[], Awaitable[None]]


def create_app(
    service: str,
    version: str,
    *,
    cors_origins: list[str] | None = None,
    readiness: dict[str, HealthCheck] | None = None,
) -> FastAPI:
    app = FastAPI(
        title=f"auralis-{service}",
        version=version,
        docs_url="/docs",
        openapi_url="/openapi.json",
    )
    readiness = readiness or {}

    if cors_origins:
        app.add_middleware(
            CORSMiddleware,
            allow_origins=cors_origins,
            allow_credentials=True,
            allow_methods=["*"],
            allow_headers=["*"],
        )

    @app.middleware("http")
    async def _context(request: Request, call_next):
        start = time.perf_counter()
        request_id = (
            request.headers.get("x-auralis-request-id") or request.headers.get("x-request-id") or str(uuid.uuid4())
        )
        corr = request.headers.get("x-correlation-id") or request_id
        bind_request_context(request_id=request_id, correlation_id=corr, endpoint=request.url.path)
        try:
            response: Response = await call_next(request)
        except Exception:
            observe_http(
                service,
                request.method,
                request.url.path,
                500,
                time.perf_counter() - start,
            )
            raise
        response.headers["x-request-id"] = request_id
        response.headers["x-correlation-id"] = corr
        response.headers["x-content-type-options"] = "nosniff"
        response.headers["referrer-policy"] = "no-referrer"
        dur = time.perf_counter() - start
        matched = request.scope.get("route").path if request.scope.get("route") else request.url.path
        observe_http(service, request.method, matched, response.status_code, dur)
        log.info(
            "request",
            method=request.method,
            path=request.url.path,
            status=response.status_code,
            duration_ms=round(dur * 1000, 2),
        )
        return response

    @app.get("/health", include_in_schema=False)
    async def health() -> dict:
        return {"status": "ok", "service": service, "version": version}

    @app.get("/ready", include_in_schema=False)
    async def ready(response: Response) -> dict:
        checks: dict[str, str] = {}
        ok = True
        for name, check in readiness.items():
            try:
                await check()
                checks[name] = "ok"
            except Exception as exc:
                checks[name] = str(exc)
                ok = False
        response.status_code = 200 if ok else 503
        return {"status": "ready" if ok else "not_ready", "checks": checks}

    @app.get("/metrics", include_in_schema=False)
    async def metrics() -> Response:
        return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)

    install_exception_handlers(app)
    return app

"""Small async JSON client for service-to-service calls."""

from __future__ import annotations

from typing import Any

import httpx
import structlog

from auralis_common.errors import ApiError

log = structlog.get_logger()


class ServiceClient:
    def __init__(
        self, base_url: str, service_token: str, caller: str, timeout: float = 8.0
    ):
        self._base = base_url.rstrip("/")
        self._token = service_token
        self._caller = caller
        self._client = httpx.AsyncClient(timeout=timeout)

    async def aclose(self) -> None:
        await self._client.aclose()

    def _headers(self, correlation_id: str | None) -> dict[str, str]:
        h = {"x-auralis-service-token": self._token, "x-auralis-service": self._caller}
        if correlation_id:
            h["x-correlation-id"] = correlation_id
        return h

    async def get(self, path: str, correlation_id: str | None = None) -> Any:
        return await self._request("GET", path, None, correlation_id)

    async def post(
        self, path: str, body: dict, correlation_id: str | None = None
    ) -> Any:
        return await self._request("POST", path, body, correlation_id)

    async def patch(
        self, path: str, body: dict, correlation_id: str | None = None
    ) -> Any:
        return await self._request("PATCH", path, body, correlation_id)

    async def _request(
        self, method: str, path: str, body: dict | None, correlation_id: str | None
    ) -> Any:
        try:
            resp = await self._client.request(
                method,
                f"{self._base}{path}",
                json=body,
                headers=self._headers(correlation_id),
            )
        except httpx.HTTPError as exc:
            raise ApiError.unavailable(f"{self._base} is unavailable") from exc
        if resp.status_code == 404:
            raise ApiError.not_found("downstream resource not found")
        if resp.status_code >= 300:
            log.warning(
                "downstream call failed",
                url=f"{self._base}{path}",
                status=resp.status_code,
                body=resp.text[:400],
            )
            raise ApiError.unavailable(f"downstream returned {resp.status_code}")
        if resp.content:
            return resp.json()
        return None

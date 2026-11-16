"""Identity header parity with authn.IdentityHeaders in libs/go-platform."""

from __future__ import annotations

import time

import pytest
from starlette.requests import Request

from auralis_common.errors import ApiError
from auralis_common.identity import _sign, optional_identity

SECRET = "test-identity-secret"
USER = "7a1f4c2e-9b3d-4e5f-8a6b-1c2d3e4f5a6b"


def _request(issued_at: str, roles: str = "USER,CREATOR") -> Request:
    headers = {
        "x-auralis-user": USER,
        "x-auralis-roles": roles,
        "x-auralis-request-id": "req-1",
        "x-auralis-issued-at": issued_at,
        "x-auralis-identity-sig": _sign(SECRET, USER, roles, "req-1", issued_at),
    }
    raw = [(k.encode(), v.encode()) for k, v in headers.items()]
    return Request({"type": "http", "headers": raw})


@pytest.fixture(autouse=True)
def _secret(monkeypatch):
    monkeypatch.setenv("IDENTITY_SECRET", SECRET)


def test_matches_the_go_signature():
    want = "f711cf87fb58463af6c3e4a6b8a3270b4fe5633cfee2a16c7c8e6b40c1e2ff1e"
    assert _sign(SECRET, USER, "USER,CREATOR", "req-1", "1767225600") == want


def test_fresh_headers_are_accepted():
    identity = optional_identity(_request(str(int(time.time()))))
    assert identity is not None and identity.has_role("CREATOR")


def test_old_headers_are_rejected():
    with pytest.raises(ApiError):
        optional_identity(_request(str(int(time.time()) - 600)))

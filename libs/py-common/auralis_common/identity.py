"""Identity verification for the Python services.

The gateway forwards the authenticated caller as HMAC-signed headers. Services
verify the signature rather than re-parsing the JWT. Service-to-service calls
use a shared token instead.
"""

from __future__ import annotations

import hashlib
import hmac
import os
from dataclasses import dataclass, field

from fastapi import Request

from auralis_common.errors import ApiError

USER = "USER"
CREATOR = "CREATOR"
ADMIN = "ADMIN"


@dataclass
class Identity:
    user_id: str
    roles: list[str] = field(default_factory=list)

    def has_role(self, role: str) -> bool:
        return role in self.roles

    def has_any(self, *roles: str) -> bool:
        return any(r in self.roles for r in roles)


def _sign(secret: str, user: str, roles: str, request_id: str) -> str:
    mac = hmac.new(secret.encode(), f"{user}\n{roles}\n{request_id}".encode(), hashlib.sha256)
    return mac.hexdigest()


def _identity_secret() -> str:
    return os.environ["IDENTITY_SECRET"]


def _service_token() -> str:
    return os.environ["SERVICE_SHARED_TOKEN"]


def optional_identity(request: Request) -> Identity | None:
    user = request.headers.get("x-auralis-user")
    if not user:
        return None
    roles = request.headers.get("x-auralis-roles", "")
    request_id = request.headers.get("x-auralis-request-id", "")
    sig = request.headers.get("x-auralis-identity-sig", "")
    expected = _sign(_identity_secret(), user, roles, request_id)
    if not hmac.compare_digest(expected, sig):
        raise ApiError.unauthorized("identity signature mismatch")
    return Identity(user_id=user, roles=[r for r in roles.split(",") if r])


def require_identity(request: Request) -> Identity:
    identity = optional_identity(request)
    if identity is None:
        raise ApiError.unauthorized()
    return identity


def require_roles(*roles: str):
    def _dep(request: Request) -> Identity:
        identity = require_identity(request)
        if roles and not identity.has_any(*roles):
            raise ApiError.forbidden()
        return identity

    return _dep


def require_service_token(request: Request) -> None:
    got = request.headers.get("x-auralis-service-token", "")
    if not got or not hmac.compare_digest(got, _service_token()):
        raise ApiError.unauthorized("valid service token required")

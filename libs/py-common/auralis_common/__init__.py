"""Shared infrastructure for the Auralis Python services."""

from auralis_common.envelope import Envelope, new_envelope
from auralis_common.errors import ApiError, error_body, install_exception_handlers
from auralis_common.identity import (
    Identity,
    optional_identity,
    require_identity,
    require_roles,
    require_service_token,
)
from auralis_common.logging import bind_request_context, setup_logging
from auralis_common.service import create_app
from auralis_common.settings import BaseServiceSettings
from auralis_common.svcclient import ServiceClient

__all__ = [
    "Envelope",
    "new_envelope",
    "ApiError",
    "error_body",
    "install_exception_handlers",
    "Identity",
    "optional_identity",
    "require_identity",
    "require_roles",
    "require_service_token",
    "setup_logging",
    "bind_request_context",
    "create_app",
    "BaseServiceSettings",
    "ServiceClient",
]

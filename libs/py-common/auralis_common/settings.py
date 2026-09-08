"""Base settings shared by the Python services."""

from __future__ import annotations

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class BaseServiceSettings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env", extra="ignore", case_sensitive=False
    )

    log_level: str = "info"
    identity_secret: str
    service_shared_token: str
    kafka_brokers: str
    otel_exporter_otlp_endpoint: str = ""
    cors_allowed_origins: str = ""

    @property
    def cors_origins(self) -> list[str]:
        return [o.strip() for o in self.cors_allowed_origins.split(",") if o.strip()]

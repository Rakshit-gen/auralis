"""Base settings shared by the Python services."""

from __future__ import annotations

from pydantic import model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

# Same rule as the Go services' config.RequireSecret.
MIN_SECRET_LEN = 32


class BaseServiceSettings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore", case_sensitive=False)

    log_level: str = "info"
    identity_secret: str
    service_shared_token: str
    kafka_brokers: str
    otel_exporter_otlp_endpoint: str = ""
    cors_allowed_origins: str = ""
    # Lets the placeholder secrets from .env.example through; local compose only.
    allow_dev_secrets: bool = False

    @model_validator(mode="after")
    def _check_secrets(self):
        for name in ("identity_secret", "service_shared_token"):
            value = getattr(self, name)
            if len(value) < MIN_SECRET_LEN:
                raise ValueError(f"{name.upper()} is shorter than {MIN_SECRET_LEN} characters")
            if "change-me" in value.lower() and not self.allow_dev_secrets:
                raise ValueError(f"{name.upper()} is still the example value; set a real secret")
        return self

    @property
    def cors_origins(self) -> list[str]:
        return [o.strip() for o in self.cors_allowed_origins.split(",") if o.strip()]

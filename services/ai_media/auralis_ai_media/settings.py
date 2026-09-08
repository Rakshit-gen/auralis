from __future__ import annotations

from auralis_common.settings import BaseServiceSettings


class Settings(BaseServiceSettings):
    ai_media_database_url: str
    ai_media_http_addr: str = "0.0.0.0:8085"

    content_service_url: str
    user_service_url: str = ""

    s3_endpoint: str
    s3_access_key: str
    s3_secret_key: str
    s3_bucket: str = "auralis-media"
    s3_region: str = "us-east-1"
    s3_use_ssl: bool = False

    # AI providers. Groq is never required.
    groq_api_key: str = ""
    groq_model: str = "openai/gpt-oss-20b"
    ai_default_provider: str = "auto"  # auto | local | groq

    # TTS. Piper is used when a voice model is present, else espeak-ng.
    piper_voice_path: str = ""

    ai_media_run_worker: bool = True
    ai_media_work_dir: str = "/tmp/auralis-media"
    ai_media_max_attempts: int = 3

    @property
    def port(self) -> int:
        return int(self.ai_media_http_addr.rsplit(":", 1)[-1])

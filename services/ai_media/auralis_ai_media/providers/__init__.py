"""Provider selection. Groq is used only when a key is configured and the
default provider allows it; everything else falls back to the local providers so
the service is always functional."""

from __future__ import annotations

import structlog

from auralis_ai_media.providers.base import GenerationError, LLMProvider, TTSProvider
from auralis_ai_media.providers.local_llm import LocalLLMProvider
from auralis_ai_media.providers.tts import LocalTTSProvider, PiperTTSProvider

log = structlog.get_logger()


def select_llm(default_provider: str, groq_api_key: str, groq_model: str) -> LLMProvider:
    if default_provider in ("auto", "groq") and groq_api_key:
        from auralis_ai_media.providers.groq_llm import GroqLLMProvider

        log.info("llm provider selected", provider="groq", model=groq_model)
        return GroqLLMProvider(groq_api_key, groq_model)
    log.info("llm provider selected", provider="local")
    return LocalLLMProvider()


def select_tts(piper_voice_path: str) -> TTSProvider:
    if piper_voice_path:
        try:
            provider = PiperTTSProvider(piper_voice_path)
            log.info("tts provider selected", provider="piper")
            return provider
        except GenerationError as exc:
            log.warning("piper unavailable, using espeak-ng", error=str(exc))
    provider = LocalTTSProvider()
    log.info("tts provider selected", provider="espeak")
    return provider


__all__ = ["LLMProvider", "TTSProvider", "select_llm", "select_tts"]

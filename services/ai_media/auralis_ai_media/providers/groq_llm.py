"""Groq adapter. OpenAI-compatible chat completions with JSON-schema structured
output. Groq is optional: it is only used when GROQ_API_KEY is set, and any
failure falls back to the local provider at the call site.

Docs: https://console.groq.com/docs/structured-outputs (json_schema, strict),
base URL https://api.groq.com/openai/v1.
"""

from __future__ import annotations

import json

import httpx
import structlog
from pydantic import BaseModel

from auralis_ai_media.languages import normalize, writing_directive
from auralis_ai_media.providers.base import GenerationError
from auralis_ai_media.providers.local_llm import LocalLLMProvider
from auralis_ai_media.schemas import (
    ContinuityContext,
    EpisodeOutline,
    EpisodeScript,
    GeneratedMetadata,
    StoryBible,
)

log = structlog.get_logger()

_BASE = "https://api.groq.com/openai/v1/chat/completions"
_SYSTEM = (
    "You are a story editor for an original serialized audio drama platform. "
    "Write grounded, character-driven fiction. Never reference real people, real brands, or copyrighted works. "
    "Respond only with JSON that matches the requested schema."
)


class _OutlineList(BaseModel):
    episodes: list[EpisodeOutline]


class GroqLLMProvider:
    name = "groq"

    def __init__(self, api_key: str, model: str = "openai/gpt-oss-20b", timeout: float = 90.0):
        self._key = api_key
        self._model = model
        self._timeout = timeout
        self._fallback = LocalLLMProvider()

    async def _complete(self, prompt: str, schema_model: type[BaseModel], schema_name: str) -> dict:
        payload = {
            "model": self._model,
            "messages": [
                {"role": "system", "content": _SYSTEM},
                {"role": "user", "content": prompt},
            ],
            "temperature": 0.9,
            "max_tokens": 8000,
            "response_format": {
                "type": "json_schema",
                "json_schema": {
                    "name": schema_name,
                    "strict": True,
                    "schema": schema_model.model_json_schema(),
                },
            },
        }
        async with httpx.AsyncClient(timeout=self._timeout) as client:
            resp = await client.post(_BASE, json=payload, headers={"Authorization": f"Bearer {self._key}"})
        if resp.status_code >= 300:
            raise GenerationError(f"groq returned {resp.status_code}: {resp.text[:300]}")
        try:
            content = resp.json()["choices"][0]["message"]["content"]
            return json.loads(content)
        except (KeyError, json.JSONDecodeError, IndexError) as exc:
            raise GenerationError(f"groq response was not valid JSON: {exc}") from exc

    async def generate_bible(self, brief: str, episode_count: int, seed: int, language: str = "en") -> StoryBible:
        language = normalize(language)
        prompt = (
            f"{writing_directive(language)}"
            f"Create a story bible for a {episode_count}-episode original audio series. "
            f"Creator brief: {brief!r}. Include a concept, 3 to 8 characters with voices and motivations, "
            f"world rules, key relationships, and a full-series arc. Keep it original and self-contained."
        )
        try:
            data = await self._complete(prompt, StoryBible, "story_bible")
            data.setdefault("episode_count", episode_count)
            bible = StoryBible.model_validate(data)
            bible.language = language
            return bible
        except (GenerationError, ValueError) as exc:
            log.warning("groq bible generation failed, using local provider", error=str(exc))
            return await self._fallback.generate_bible(brief, episode_count, seed, language)

    async def generate_outlines(self, bible: StoryBible, seed: int, language: str = "en") -> list[EpisodeOutline]:
        language = normalize(language or bible.language)
        prompt = (
            f"{writing_directive(language)}"
            f"Given this story bible, write one outline per episode for all {bible.episode_count} episodes. "
            f"Each outline needs a number, title, summary, 3 to 8 beats, and a cliffhanger. "
            f"Follow a three-act shape across the season.\n\nBIBLE:\n{bible.model_dump_json()}"
        )
        try:
            data = await self._complete(prompt, _OutlineList, "episode_outlines")
            outlines = _OutlineList.model_validate(data).episodes
            if len(outlines) != bible.episode_count:
                raise GenerationError("outline count mismatch")
            return outlines
        except (GenerationError, ValueError) as exc:
            log.warning("groq outline generation failed, using local provider", error=str(exc))
            return await self._fallback.generate_outlines(bible, seed, language)

    async def generate_script(self, ctx: ContinuityContext, seed: int) -> EpisodeScript:
        language = normalize(ctx.language)
        prompt = (
            f"{writing_directive(language)}"
            "Write the full script for this episode as a list of speaker/text lines. "
            "Use 'Narrator' for narration and character names for dialogue. 8 to 400 lines. "
            "Honour the continuity context exactly; do not contradict established facts.\n\n"
            f"CONTEXT:\n{ctx.model_dump_json()}"
        )
        try:
            data = await self._complete(prompt, EpisodeScript, "episode_script")
            data.setdefault("number", ctx.outline.number)
            data.setdefault("title", ctx.outline.title)
            return EpisodeScript.model_validate(data)
        except (GenerationError, ValueError) as exc:
            log.warning("groq script generation failed, using local provider", error=str(exc))
            return await self._fallback.generate_script(ctx, seed)

    async def generate_metadata(self, bible: StoryBible, seed: int, language: str = "en") -> GeneratedMetadata:
        language = normalize(language or bible.language)
        directive = writing_directive(language)
        if directive:
            directive = (
                f"{directive}Keep the tags as lowercase English slugs; write short_description in the "
                f"story's language. "
            )
        prompt = (
            f"{directive}"
            "Produce catalog metadata for this series: 2 to 12 lowercase tags, a maturity rating "
            "(general|teen|mature), a hex accent_color, and a short_description under 300 characters.\n\n"
            f"BIBLE:\n{bible.model_dump_json()}"
        )
        try:
            data = await self._complete(prompt, GeneratedMetadata, "metadata")
            return GeneratedMetadata.model_validate(data)
        except (GenerationError, ValueError) as exc:
            log.warning("groq metadata generation failed, using local provider", error=str(exc))
            return await self._fallback.generate_metadata(bible, seed, language)

"""Provider interfaces. External providers are never referenced outside this
package, so swapping one is a wiring change, not a code change."""

from __future__ import annotations

from typing import Protocol, TypeVar

from pydantic import BaseModel

from auralis_ai_media.schemas import (
    ContinuityContext,
    EpisodeOutline,
    EpisodeScript,
    GeneratedMetadata,
    StoryBible,
)

T = TypeVar("T", bound=BaseModel)


class GenerationError(RuntimeError):
    """Raised when a provider cannot produce valid output."""


class LLMProvider(Protocol):
    name: str

    async def generate_bible(self, brief: str, episode_count: int, seed: int, language: str = "en") -> StoryBible: ...

    async def generate_outlines(self, bible: StoryBible, seed: int, language: str = "en") -> list[EpisodeOutline]: ...

    async def generate_script(self, ctx: ContinuityContext, seed: int) -> EpisodeScript: ...

    async def generate_metadata(self, bible: StoryBible, seed: int, language: str = "en") -> GeneratedMetadata: ...


class TTSSegment(BaseModel):
    speaker: str
    text: str
    voice: str


class TTSResult(BaseModel):
    # WAV bytes are returned out of band via the file path to avoid holding large
    # payloads in memory maps; duration is in seconds.
    wav_path: str
    duration_sec: float
    sample_rate: int
    channels: int


class TTSProvider(Protocol):
    name: str

    async def synthesize(self, segments: list[TTSSegment], out_dir: str, language: str = "en") -> TTSResult: ...

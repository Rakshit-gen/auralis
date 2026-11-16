"""Provider interfaces. External providers are never referenced outside this
package, so swapping one is a wiring change, not a code change."""

from __future__ import annotations

import asyncio
import os
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


async def run_process(
    cmd: list[str], *, limit_sec: float, stdin: bytes | None = None, env: dict[str, str] | None = None
) -> bytes:
    """Run cmd and return its stdout. A process that fails or runs past limit_sec
    seconds is a GenerationError; one that hangs is killed rather than left
    holding the worker forever."""
    proc = await asyncio.create_subprocess_exec(
        *cmd,
        stdin=asyncio.subprocess.PIPE if stdin is not None else None,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        env=env,
    )
    name = os.path.basename(cmd[0])
    try:
        out, err = await asyncio.wait_for(proc.communicate(stdin), limit_sec)
    except TimeoutError:
        proc.kill()
        await proc.wait()
        raise GenerationError(f"{name} took longer than {limit_sec:.0f}s") from None
    if proc.returncode != 0:
        raise GenerationError(f"{name} failed: {err.decode(errors='replace')[:400]}")
    return out


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

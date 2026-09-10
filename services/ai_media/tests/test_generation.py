"""Local generation and media pipeline, end to end without external services."""

from __future__ import annotations

import os
import shutil

import httpx
import pytest

from auralis_ai_media.pipeline import media
from auralis_ai_media.providers.base import GenerationError, TTSSegment
from auralis_ai_media.providers.local_llm import LocalLLMProvider
from auralis_ai_media.providers.tts import LocalTTSProvider, PiperTTSProvider
from auralis_ai_media.schemas import ContinuityContext, EpisodeScript, StoryBible


@pytest.mark.asyncio
async def test_groq_network_failure_falls_back_to_local(monkeypatch):
    from auralis_ai_media.providers.groq_llm import GroqLLMProvider

    async def boom(*a, **kw):
        raise httpx.ConnectError("connection refused")

    monkeypatch.setattr(httpx.AsyncClient, "post", boom)
    groq = GroqLLMProvider("fake-key", "some-model")
    bible = await groq.generate_bible("a lighthouse keeper who hears the drowned", 6, seed=42)
    assert isinstance(bible, StoryBible)  # local fallback, not an uncaught ConnectError


@pytest.mark.asyncio
async def test_local_llm_produces_valid_and_continuous_story():
    llm = LocalLLMProvider()

    bible = await llm.generate_bible("a story about a lighthouse keeper who hears the drowned", 6, seed=42)
    assert isinstance(bible, StoryBible)
    assert 3 <= bible.episode_count <= 30
    assert len(bible.characters) >= 2

    # Deterministic for a fixed seed.
    bible2 = await llm.generate_bible("a story about a lighthouse keeper who hears the drowned", 6, seed=42)
    assert bible2.concept.title == bible.concept.title

    outlines = await llm.generate_outlines(bible, seed=42)
    assert len(outlines) == bible.episode_count
    assert [o.number for o in outlines] == list(range(1, bible.episode_count + 1))

    # Episode 3 script, with continuity from episodes 1 and 2.
    ctx = ContinuityContext(
        concept=bible.concept,
        characters=bible.characters,
        world_rules=bible.world_rules,
        recent_summaries=["Episode 1 set up the debt.", "Episode 2 revealed the antagonist's stake."],
        unresolved_threads=["who paid the first cost"],
        arc=bible.arc,
        outline=outlines[2],
    )
    script = await llm.generate_script(ctx, seed=42)
    assert isinstance(script, EpisodeScript)
    assert script.number == 3
    assert len(script.lines) >= 8
    assert script.recap.startswith("Previously:")
    # The script references a real character from the bible.
    speakers = {ln.speaker for ln in script.lines}
    assert speakers & {c.name for c in bible.characters}

    metadata = await llm.generate_metadata(bible, seed=42)
    assert 2 <= len(metadata.tags) <= 12
    assert metadata.maturity in ("general", "teen", "mature")


@pytest.mark.asyncio
async def test_tts_and_ffmpeg_packaging(tmp_path):
    if not shutil.which("espeak-ng") and not shutil.which("espeak"):
        pytest.skip("espeak-ng not installed")
    if not shutil.which("ffmpeg"):
        pytest.skip("ffmpeg not installed")

    tts = LocalTTSProvider()
    segments = [
        TTSSegment(speaker="Narrator", text="The harbor was quiet the night the signal came back.", voice="narrator"),
        TTSSegment(speaker="Marisol", text="You heard it too. Don't tell me you didn't.", voice="bright_quick"),
        TTSSegment(speaker="Idris", text="I heard something. That's not the same as knowing what.", voice="low_warm"),
    ] * 3

    result = await tts.synthesize(segments, str(tmp_path / "tts"))
    assert result.duration_sec > 3
    assert result.sample_rate > 0

    packaged = await media.package(result.wav_path, str(tmp_path / "work"))
    assert {v["bitrate_kbps"] for v in packaged.variants} == {64, 128, 256}
    assert packaged.duration_sec >= 3
    assert packaged.channels == 2
    assert packaged.codec == "aac"
    assert packaged.checksum_sha256
    assert (tmp_path / "work" / "hls" / "master.m3u8").exists()


@pytest.mark.asyncio
async def test_piper_tts_when_configured(tmp_path):
    """Runs only where Piper is set up (scripts/piper-setup.sh); CI has neither
    the binary nor the models, so it skips there."""
    voices_dir = os.environ.get("PIPER_VOICES_DIR", "")
    if not voices_dir:
        pytest.skip("PIPER_VOICES_DIR not set")
    try:
        tts = PiperTTSProvider(voices_dir)
    except GenerationError as exc:
        pytest.skip(f"piper not usable: {exc}")

    segments = [
        TTSSegment(speaker="Narrator", text="The tide turned just before first light.", voice="narrator"),
        TTSSegment(speaker="Wren", text="You said the line would hold. It did not hold.", voice="bright_quick"),
    ]
    result = await tts.synthesize(segments, str(tmp_path / "piper"))
    assert result.duration_sec > 1
    assert result.sample_rate == 22050
    assert result.channels == 1

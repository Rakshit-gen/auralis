"""Language threading: prompts, bible persistence, and the local-provider downgrade."""

from __future__ import annotations

import pytest

from auralis_ai_media.languages import DEFAULT_LANGUAGE, normalize, writing_directive
from auralis_ai_media.providers.local_llm import LocalLLMProvider
from auralis_ai_media.schemas import ContinuityContext


def test_normalize_falls_back_to_english_for_unknown_codes():
    assert normalize("hi") == "hi"
    assert normalize("hi-IN") == "hi"
    assert normalize("ES") == "es"
    assert normalize("") == DEFAULT_LANGUAGE
    assert normalize(None) == DEFAULT_LANGUAGE
    assert normalize("kl") == DEFAULT_LANGUAGE  # not in the supported set


def test_writing_directive_is_empty_for_english_and_specific_otherwise():
    assert writing_directive("en") == ""
    hi = writing_directive("hi")
    assert "Hindi" in hi and "Devanagari" in hi
    assert "English" in hi  # field names / enums stay English


@pytest.mark.asyncio
async def test_local_provider_labels_output_english_even_when_hindi_requested():
    llm = LocalLLMProvider()
    bible = await llm.generate_bible("a diver who trades in tide-secrets", 6, seed=7, language="hi")
    # The procedural generator cannot translate; it must not claim to be Hindi.
    assert bible.language == "en"

    # Outlines and metadata accept the keyword without breaking.
    outlines = await llm.generate_outlines(bible, seed=7, language="hi")
    assert len(outlines) == bible.episode_count
    meta = await llm.generate_metadata(bible, seed=7, language="hi")
    assert 2 <= len(meta.tags) <= 12


@pytest.mark.asyncio
async def test_continuity_context_carries_language_to_the_script_call():
    llm = LocalLLMProvider()
    bible = await llm.generate_bible("a night market that assembles for the grieving", 6, seed=11)
    outlines = await llm.generate_outlines(bible, seed=11)
    ctx = ContinuityContext(
        concept=bible.concept,
        characters=bible.characters,
        world_rules=bible.world_rules,
        recent_summaries=[],
        unresolved_threads=[],
        arc=bible.arc,
        outline=outlines[0],
        language="es",
    )
    assert ctx.language == "es"
    script = await llm.generate_script(ctx, seed=11)
    assert script.number == 1

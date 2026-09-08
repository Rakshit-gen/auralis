"""Language metadata shared by the LLM prompts and the TTS voice selection.

`code` is the ISO 639-1 code stored on the show. Anything not listed here is
treated as English: the local procedural provider cannot translate, and an
honestly labelled English show beats a mislabelled broken one.
"""

from __future__ import annotations

import structlog

log = structlog.get_logger()

DEFAULT_LANGUAGE = "en"

# code -> (english name, endonym, script name)
LANGUAGES: dict[str, tuple[str, str, str]] = {
    "en": ("English", "English", "Latin"),
    "hi": ("Hindi", "हिन्दी", "Devanagari"),
    "es": ("Spanish", "Español", "Latin"),
}


def normalize(code: str | None) -> str:
    """Reduce a requested code to one we can actually produce, else English."""
    if not code:
        return DEFAULT_LANGUAGE
    code = code.strip().lower().split("-")[0]
    return code if code in LANGUAGES else DEFAULT_LANGUAGE


def language_name(code: str) -> str:
    return LANGUAGES[normalize(code)][0]


def writing_directive(code: str) -> str:
    """A clause instructing the model which language to write in.

    Empty for English, so English prompts are unchanged.
    """
    code = normalize(code)
    if code == DEFAULT_LANGUAGE:
        return ""
    name, endonym, script = LANGUAGES[code]
    return (
        f"Write all prose, dialogue, titles, and descriptions in {name} ({endonym}), in {script} "
        f"script. Keep character and place names in the form a {name} speaker would use. Do not add "
        f"an English translation. Return field names and enum values (roles, maturity) in English. "
    )

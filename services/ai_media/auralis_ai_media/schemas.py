"""Pydantic schemas for AI-generated story content. Every provider response is
validated against these before it is persisted or used."""

from __future__ import annotations

from pydantic import BaseModel, Field, field_validator


class Character(BaseModel):
    name: str = Field(min_length=1, max_length=80)
    role: str = Field(min_length=1, max_length=60)  # protagonist, antagonist, ally, mentor, ...
    description: str = Field(min_length=10, max_length=600)
    voice: str = Field(default="narrator", max_length=40)  # tts voice key
    motivation: str = Field(min_length=5, max_length=300)


class WorldRule(BaseModel):
    title: str = Field(min_length=1, max_length=100)
    detail: str = Field(min_length=10, max_length=500)


class Relationship(BaseModel):
    from_character: str
    to_character: str
    nature: str = Field(max_length=120)


class SeriesConcept(BaseModel):
    title: str = Field(min_length=2, max_length=120)
    logline: str = Field(min_length=20, max_length=300)
    synopsis: str = Field(min_length=60, max_length=1200)
    genres: list[str] = Field(min_length=1, max_length=4)
    tone: str = Field(max_length=120)
    setting: str = Field(min_length=10, max_length=400)
    themes: list[str] = Field(min_length=1, max_length=6)


class StoryArc(BaseModel):
    premise: str = Field(min_length=20, max_length=600)
    inciting_incident: str = Field(min_length=10, max_length=400)
    midpoint: str = Field(min_length=10, max_length=400)
    climax: str = Field(min_length=10, max_length=400)
    resolution: str = Field(min_length=10, max_length=400)


class StoryBible(BaseModel):
    concept: SeriesConcept
    characters: list[Character] = Field(min_length=2, max_length=12)
    world_rules: list[WorldRule] = Field(min_length=1, max_length=10)
    relationships: list[Relationship] = Field(default_factory=list, max_length=20)
    arc: StoryArc
    episode_count: int = Field(ge=3, le=30)


class EpisodeOutline(BaseModel):
    number: int = Field(ge=1)
    title: str = Field(min_length=2, max_length=120)
    summary: str = Field(min_length=30, max_length=800)
    beats: list[str] = Field(min_length=3, max_length=10)
    cliffhanger: str = Field(min_length=5, max_length=300)


class ScriptLine(BaseModel):
    speaker: str = Field(min_length=1, max_length=80)  # "Narrator" or a character name
    text: str = Field(min_length=1, max_length=1200)

    @field_validator("text")
    @classmethod
    def _no_stage_direction_only(cls, v: str) -> str:
        if not any(c.isalpha() for c in v):
            raise ValueError("script line must contain spoken words")
        return v


class EpisodeScript(BaseModel):
    number: int = Field(ge=1)
    title: str = Field(min_length=2, max_length=120)
    synopsis: str = Field(min_length=20, max_length=600)
    lines: list[ScriptLine] = Field(min_length=8, max_length=400)
    recap: str = Field(default="", max_length=600)

    @property
    def word_count(self) -> int:
        return sum(len(line.text.split()) for line in self.lines)


class GeneratedMetadata(BaseModel):
    tags: list[str] = Field(min_length=2, max_length=12)
    maturity: str = Field(pattern="^(general|teen|mature)$")
    accent_color: str = Field(pattern="^#[0-9a-fA-F]{6}$")
    short_description: str = Field(min_length=20, max_length=300)


class PlotThread(BaseModel):
    summary: str = Field(min_length=5, max_length=300)
    introduced_in: int = Field(ge=1)
    resolved: bool = False


class ContinuityContext(BaseModel):
    """The bounded context handed to the model for episode N. The full history is
    never sent: only persisted characters, world rules, recent summaries, and
    unresolved threads."""

    concept: SeriesConcept
    characters: list[Character]
    world_rules: list[WorldRule]
    recent_summaries: list[str] = Field(max_length=3)
    unresolved_threads: list[str] = Field(default_factory=list)
    arc: StoryArc
    outline: EpisodeOutline

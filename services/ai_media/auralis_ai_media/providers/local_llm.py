"""Local story generator. No network, no API key.

This is a procedural generator, not a stub: it composes a coherent story bible,
per-episode outlines that follow a three-act arc, and full dialogue scripts that
reference the persisted characters and unresolved plot threads. Output is
deterministic for a given seed so a regenerated episode is reproducible, and
varied enough across seeds that two shows do not read alike.
"""

from __future__ import annotations

import random

import structlog

from auralis_ai_media.languages import normalize
from auralis_ai_media.providers.base import GenerationError
from auralis_ai_media.schemas import (
    Character,
    ContinuityContext,
    EpisodeOutline,
    EpisodeScript,
    GeneratedMetadata,
    Relationship,
    ScriptLine,
    SeriesConcept,
    StoryArc,
    StoryBible,
    WorldRule,
)

log = structlog.get_logger()

_SETTINGS = [
    ("a tidal city built on the backs of sleeping leviathans", "coastal, brass and salt"),
    ("a border town where the last working railway meets the dust", "dry, wind-scoured, patient"),
    ("an orbital archive that remembers more than the people who built it", "cold, humming, watchful"),
    ("a mountain monastery that trades in other people's memories", "hushed, candlelit, exact"),
    ("a river delta where the water keeps rewriting the maps", "humid, green, shifting"),
    ("a night market that only assembles when someone is grieving", "lantern-lit, crowded, kind"),
]
_ENGINES = [
    "a debt that must be spoken aloud to stay real",
    "a machine that runs on things people refuse to say",
    "a tide that carries messages between the living and whatever the sea keeps",
    "a contract written in a language only two people still read",
    "a signal from a place that should not have a voice",
    "an inheritance nobody in the family will name",
]
_THEMES = [
    "what we owe the people who raised us",
    "the cost of being believed",
    "how a place remembers violence",
    "whether a promise survives the person who made it",
    "the difference between rescue and possession",
    "who gets to decide when a story is finished",
]
_GENRES = ["mystery", "science fiction", "drama", "thriller", "folk horror", "adventure", "noir"]
_TONES = ["patient and eerie", "warm but unsentimental", "tense and procedural", "wry, then sudden", "quiet dread"]
_FIRST = ["Marisol", "Idris", "Nnenna", "Bassey", "Vale", "Toma", "Wren", "Okonkwo", "Sable", "Ari", "Del", "Halden"]
_LAST = ["Okafor", "Reyes", "Sunder", "Achebe", "Voss", "Karim", "Idle", "Marsh", "Dane", "Osei", "Pell"]
_ROLES = ["protagonist", "antagonist", "ally", "mentor", "witness", "rival", "outsider"]
_VOICES = ["narrator", "low_warm", "bright_quick", "dry_measured", "rough_soft", "clear_high"]
_COLORS = ["#c98a3c", "#3c6ec9", "#8a3cc9", "#c93c5e", "#3cc98a", "#c9b23c", "#5e3cc9"]


def _rng(seed: int) -> random.Random:
    return random.Random(seed)


def _person(r: random.Random) -> str:
    return f"{r.choice(_FIRST)} {r.choice(_LAST)}"


class LocalLLMProvider:
    name = "local"

    async def generate_bible(self, brief: str, episode_count: int, seed: int, language: str = "en") -> StoryBible:
        if normalize(language) != "en":
            log.warning(
                "local provider cannot translate; generating an English series",
                requested_language=normalize(language),
            )
        r = _rng(seed)
        setting, _palette = r.choice(_SETTINGS)
        engine = r.choice(_ENGINES)
        themes = r.sample(_THEMES, k=r.randint(2, 3))
        genres = r.sample(_GENRES, k=r.randint(1, 3))
        brief = brief.strip() or "an original serialized audio drama"

        title = self._title(r, setting)
        concept = SeriesConcept(
            title=title,
            logline=f"When {engine} surfaces in {setting}, one person has to decide what telling the truth is worth.",
            synopsis=(
                f"{title} follows a small group in {setting}. The premise brief was: {brief[:400]}. "
                f"At the centre is {engine}. Over {episode_count} episodes the story works through {themes[0]} "
                f"and {themes[-1]}, one confrontation at a time, without ever leaving the place it started."
            ),
            genres=genres,
            tone=r.choice(_TONES),
            setting=setting,
            themes=themes,
        )

        n_chars = r.randint(4, 6)
        names = {_person(r) for _ in range(n_chars * 2)}
        names = list(names)[:n_chars]
        roles = ["protagonist", "antagonist", *r.sample(_ROLES[2:], k=n_chars - 2)]
        characters = [
            Character(
                name=name,
                role=role,
                description=self._char_desc(r, name, role, setting, engine),
                voice=r.choice(_VOICES),
                motivation=self._motivation(r, role, themes),
            )
            for name, role in zip(names, roles, strict=True)
        ]

        world_rules = [
            WorldRule(
                title="The engine",
                detail=f"The story turns on {engine}. It is real, it is limited, and it has a price.",
            ),
            WorldRule(
                title="The place",
                detail=f"Everything happens in or just outside {setting}. Nobody gets to leave for long.",
            ),
            WorldRule(
                title="What it costs",
                detail="Using the engine always takes something the user did not plan to give. The show never lets that be free.",
            ),
        ]
        if r.random() < 0.6:
            world_rules.append(
                WorldRule(
                    title="Who knows",
                    detail="Only three people understand how the engine works, and two of them are lying about it.",
                )
            )

        relationships = [
            Relationship(
                from_character=characters[0].name,
                to_character=characters[1].name,
                nature="opposed, and bound by the same debt",
            ),
        ]
        if len(characters) > 2:
            relationships.append(
                Relationship(
                    from_character=characters[0].name,
                    to_character=characters[2].name,
                    nature="trust that has not been tested yet",
                )
            )

        arc = StoryArc(
            premise=f"{characters[0].name} is the only person who can work {engine}, and the only one who understands what it takes.",
            inciting_incident=f"In episode 1, {engine} is used without {characters[0].name}'s consent and someone is hurt.",
            midpoint=f"Halfway through, {characters[0].name} learns {characters[1].name} has been running the engine for years.",
            climax=f"{characters[0].name} has to choose between shutting the engine down for good and saving one specific person.",
            resolution=f"The place goes on. The engine is quieter. {themes[0].capitalize()} does not get resolved so much as carried.",
        )

        bible = StoryBible(
            concept=concept,
            characters=characters,
            world_rules=world_rules,
            relationships=relationships,
            arc=arc,
            episode_count=max(3, min(30, episode_count)),
        )
        return bible

    async def generate_outlines(self, bible: StoryBible, seed: int, language: str = "en") -> list[EpisodeOutline]:
        r = _rng(seed + 101)
        n = bible.episode_count
        lead = bible.characters[0].name
        foil = bible.characters[1].name
        outlines: list[EpisodeOutline] = []
        for i in range(1, n + 1):
            phase = "setup" if i <= n / 3 else ("confrontation" if i <= 2 * n / 3 else "resolution")
            focus = bible.characters[(i - 1) % len(bible.characters)]
            beats = [
                f"Open on {focus.name} dealing with the fallout of episode {max(1, i - 1)}.",
                f"A new demand is made of {lead} involving {bible.concept.themes[i % len(bible.concept.themes)]}.",
                f"{lead} and {foil} are forced into the same room.",
                "The engine is used, and the cost lands on someone who did not agree to it.",
            ]
            if phase != "setup":
                beats.append(f"{focus.name} makes a decision that cannot be walked back.")
            outlines.append(
                EpisodeOutline(
                    number=i,
                    title=self._episode_title(r, i, phase, bible),
                    summary=(
                        f"Episode {i} ({phase}). {focus.name} is at the centre. The pressure from {bible.arc.premise[:120]} "
                        f"tightens, and by the end {lead} is further from a clean way out."
                    ),
                    beats=beats[: r.randint(3, len(beats))] or beats[:3],
                    cliffhanger=self._cliffhanger(r, i, n, bible),
                )
            )
        return outlines

    async def generate_script(self, ctx: ContinuityContext, seed: int) -> EpisodeScript:
        r = _rng(seed + 202 + ctx.outline.number)
        chars = ctx.characters
        if len(chars) < 2:
            raise GenerationError("continuity context needs at least two characters")
        lead, foil = chars[0], chars[1]
        speakers = [c.name for c in chars]

        lines: list[ScriptLine] = []
        recap = ""
        if ctx.recent_summaries:
            recap = "Previously: " + " ".join(s.rstrip(".") + "." for s in ctx.recent_summaries[-2:])
            lines.append(ScriptLine(speaker="Narrator", text=recap))

        lines.append(
            ScriptLine(
                speaker="Narrator",
                text=f"{ctx.outline.summary} The scene is {ctx.concept.setting}.",
            )
        )
        for beat_i, beat in enumerate(ctx.outline.beats):
            lines.append(ScriptLine(speaker="Narrator", text=beat))
            a = speakers[beat_i % len(speakers)]
            b = speakers[(beat_i + 1) % len(speakers)]
            lines.append(ScriptLine(speaker=a, text=self._dialogue(r, a, b, beat, ctx)))
            lines.append(ScriptLine(speaker=b, text=self._reply(r, b, a, ctx)))
            if ctx.unresolved_threads and r.random() < 0.5:
                thread = r.choice(ctx.unresolved_threads)
                lines.append(
                    ScriptLine(speaker=lead.name, text=f"We still haven't dealt with {thread.rstrip('.').lower()}.")
                )

        lines.append(
            ScriptLine(
                speaker=foil.name,
                text=self._confrontation(r, foil, lead, ctx),
            )
        )
        lines.append(ScriptLine(speaker="Narrator", text=ctx.outline.cliffhanger))

        # Pad to the minimum line count with grounded exchanges if a short outline
        # produced too few lines.
        while len(lines) < 10:
            a, b = r.sample(speakers, 2)
            lines.append(ScriptLine(speaker=a, text=self._dialogue(r, a, b, ctx.outline.title, ctx)))
            lines.append(ScriptLine(speaker=b, text=self._reply(r, b, a, ctx)))

        return EpisodeScript(
            number=ctx.outline.number,
            title=ctx.outline.title,
            synopsis=ctx.outline.summary[:580],
            lines=lines[:400],
            recap=recap,
        )

    async def generate_metadata(self, bible: StoryBible, seed: int, language: str = "en") -> GeneratedMetadata:
        r = _rng(seed + 303)
        tag_pool = (
            list(bible.concept.genres)
            + [t.split()[0] for t in bible.concept.themes]
            + ["serialized", "audio-drama", "original"]
        )
        tags = list(dict.fromkeys(t.lower() for t in tag_pool))[:8]
        if len(tags) < 2:
            tags = ["audio-drama", "original"]
        return GeneratedMetadata(
            tags=tags,
            maturity=r.choice(["general", "teen", "teen", "mature"]),
            accent_color=r.choice(_COLORS),
            short_description=bible.concept.logline[:290],
        )

    # --- composition helpers ---

    @staticmethod
    def _title(r: random.Random, setting: str) -> str:
        heads = ["The", "A", "Every", "No"]
        nouns = ["Tide", "Ledger", "Signal", "Inheritance", "Debt", "Archive", "Crossing", "Market", "Witness"]
        tails = ["We Keep", "That Answers", "of Salt", "Between Us", "Left Running", "and What It Wants"]
        if r.random() < 0.5:
            return f"{r.choice(heads)} {r.choice(nouns)} {r.choice(tails)}"
        return f"{r.choice(nouns)} {r.choice(tails)}"

    @staticmethod
    def _episode_title(r: random.Random, i: int, phase: str, bible: StoryBible) -> str:
        pool = {
            "setup": ["First Draw", "The Quiet Part", "Terms", "What the Water Left", "Opening the Ledger"],
            "confrontation": [
                "The Middle of It",
                "No Clean Version",
                "Both Hands",
                "The Room They Share",
                "Interest Owed",
            ],
            "resolution": ["Last Draw", "Carried, Not Closed", "The Long Way Back", "What Stays Running", "Settlement"],
        }
        return f"{r.choice(pool[phase])}"

    @staticmethod
    def _cliffhanger(r: random.Random, i: int, n: int, bible: StoryBible) -> str:
        if i == n:
            return f"The engine is still running, just quieter, and {bible.characters[0].name} is the only one who checks it now."
        return r.choice(
            [
                f"The line goes dead, and then it says {bible.characters[0].name}'s name back.",
                f"{bible.characters[1].name} was in the room the whole time.",
                "The cost this time is not money, and everyone in the room knows whose it is.",
                "Someone already paid it, weeks ago, and told no one.",
            ]
        )

    @staticmethod
    def _char_desc(r: random.Random, name: str, role: str, setting: str, engine: str) -> str:
        jobs = ["dockworker", "archivist", "medic", "smuggler", "teacher", "auditor", "fisher", "signal tech"]
        return (
            f"{name} is a {r.choice(jobs)} in {setting}. As the {role}, {name} is closest to {engine} and least able "
            f"to pretend it does not matter. Careful with words, worse with silence."
        )

    @staticmethod
    def _motivation(r: random.Random, role: str, themes: list[str]) -> str:
        base = {
            "protagonist": "wants to stop the engine hurting people without losing the one person it could still save",
            "antagonist": "believes the engine is worth any price and has already decided who pays",
        }.get(role, f"is trying to protect their stake in {r.choice(themes)}")
        return base

    @staticmethod
    def _dialogue(r: random.Random, a: str, b: str, beat: str, ctx: ContinuityContext) -> str:
        openers = [
            f"{b}, we don't have the room to do this the slow way.",
            "You keep saying it's handled. Show me the part that's handled.",
            f"I read the outline of this before it happened, {b}. That's the problem.",
            "Tell me what it took this time. All of it.",
        ]
        return r.choice(openers)

    @staticmethod
    def _reply(r: random.Random, b: str, a: str, ctx: ContinuityContext) -> str:
        replies = [
            f"You want it simple, {a}. It stopped being simple three episodes ago.",
            "I did what the place needed. You'd have done the same and hated yourself louder.",
            "There's a version where nobody gets hurt. We are not in it.",
            f"Ask me again when you've paid something, {a}.",
        ]
        return r.choice(replies)

    @staticmethod
    def _confrontation(r: random.Random, foil: Character, lead: Character, ctx: ContinuityContext) -> str:
        return (
            f"You think shutting it down is the brave choice, {lead.name}. It's just the one that lets you stop deciding. "
            f"I've been deciding for years. Someone has to."
        )

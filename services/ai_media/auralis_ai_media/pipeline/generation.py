"""Job orchestration: turn a queued generation job into a draft show, draft
episodes, scripts, and packaged HLS audio, driving the content service through
its service-to-service API. Continuity is assembled from persisted state, never
the full history."""

from __future__ import annotations

import os
import shutil

import structlog
from auralis_common.svcclient import ServiceClient
from auralis_common.telemetry import AI_DURATION, TTS_DURATION, timed

from auralis_ai_media import models
from auralis_ai_media.languages import normalize
from auralis_ai_media.pipeline import media
from auralis_ai_media.providers.base import LLMProvider, TTSProvider, TTSSegment
from auralis_ai_media.repo import Repo
from auralis_ai_media.schemas import ContinuityContext
from auralis_ai_media.storage import ObjectStore

log = structlog.get_logger()


class Pipeline:
    def __init__(
        self,
        llm: LLMProvider,
        tts: TTSProvider,
        store: ObjectStore,
        content: ServiceClient,
        work_dir: str,
    ):
        self.llm = llm
        self.tts = tts
        self.store = store
        self.content = content
        self.work_dir = work_dir

    async def run_series(self, repo: Repo, job: models.GenerationJob) -> None:
        brief = str(job.prompt.get("brief", "")).strip()
        episode_count = int(job.prompt.get("episode_count", 8))
        seed = int(job.prompt.get("seed", abs(hash(job.id)) % (2**31)))
        requested_language = normalize(job.prompt.get("language_code", "en"))

        await repo.advance_job(job, "generating_bible", 10)
        await repo.s.flush()
        with timed(AI_DURATION, "ai-media", "bible", self.llm.name):
            bible = await self.llm.generate_bible(brief, episode_count, seed, requested_language)

        # The provider is the authority on which language it actually wrote in: a
        # local fallback cannot translate, so a Hindi request can come back as an
        # English bible. Label the show for what it is.
        language = normalize(bible.language)
        if language != requested_language:
            log.warning(
                "generation language downgraded",
                job_id=job.id,
                requested=requested_language,
                produced=language,
            )

        with timed(AI_DURATION, "ai-media", "metadata", self.llm.name):
            meta = await self.llm.generate_metadata(bible, seed, language)

        show = await self.content.post(
            "/internal/authoring/shows",
            {
                "creator_user_id": job.requested_by,
                "creator_name": "Auralis Studio",
                "title": bible.concept.title,
                "synopsis": bible.concept.logline,
                "description": bible.concept.synopsis,
                "language_code": language,
                "tags": meta.tags,
                "maturity": meta.maturity,
                "accent_color": meta.accent_color,
                "is_premium": bool(job.prompt.get("is_premium", False)),
            },
            correlation_id=job.id,
        )
        show_id = show["id"]
        job.show_id = show_id
        await repo.save_bible(show_id, bible)

        await repo.advance_job(job, "generating_outline", 30)
        await repo.s.flush()
        with timed(AI_DURATION, "ai-media", "outlines", self.llm.name):
            outlines = await self.llm.generate_outlines(bible, seed, language)

        child_ids: list[str] = []
        for outline in outlines:
            episode = await self.content.post(
                "/internal/authoring/episodes",
                {
                    "show_id": show_id,
                    "season_number": 1,
                    "number": outline.number,
                    "title": outline.title,
                    "synopsis": outline.summary[:580],
                    "ai_job_id": job.id,
                    "is_premium": bool(job.prompt.get("is_premium", False)) and outline.number > 3,
                    "free_preview_sec": 90 if outline.number > 3 else 0,
                },
                correlation_id=job.id,
            )
            child = await repo.create_job(
                kind="episode",
                status="queued",
                requested_by=job.requested_by,
                provider=self.llm.name,
                show_id=show_id,
                episode_id=episode["id"],
                episode_number=outline.number,
                prompt={"outline": outline.model_dump(), "seed": seed, "language": language},
            )
            child_ids.append(child.id)

        job.result = {"show_id": show_id, "episode_jobs": child_ids, "episode_count": len(outlines)}
        await repo.advance_job(job, "completed", 100, f"created show {show_id} with {len(outlines)} episode jobs")

    async def run_episode(self, repo: Repo, job: models.GenerationJob) -> None:
        show_id = job.show_id
        episode_id = job.episode_id
        assert show_id and episode_id
        seed = int(job.prompt.get("seed", 0))
        outline = _outline_from(job.prompt["outline"])

        bible = await repo.load_bible(show_id)
        if bible is None:
            raise RuntimeError(f"no story bible for show {show_id}")

        language = normalize(job.prompt.get("language") or bible.language)

        ctx = ContinuityContext(
            concept=bible.concept,
            characters=bible.characters,
            world_rules=bible.world_rules,
            recent_summaries=await repo.recent_summaries(show_id, outline.number),
            unresolved_threads=await repo.unresolved_threads(show_id),
            arc=bible.arc,
            outline=outline,
            language=language,
        )

        await repo.advance_job(job, "generating_script", 15)
        await self._patch_episode(job.id, episode_id, {"processing": "generating_script"})
        await repo.s.flush()
        with timed(AI_DURATION, "ai-media", "script", self.llm.name):
            script = await self.llm.generate_script(ctx, seed)

        script_text = "\n".join(f"{ln.speaker}: {ln.text}" for ln in script.lines)
        await self._patch_episode(job.id, episode_id, {"script": script_text, "processing": "synthesizing"})
        await repo.save_episode_summary(show_id, outline.number, script.title, outline.summary[:600])
        if outline.number >= bible.episode_count / 2:
            await repo.resolve_one_thread(show_id, outline.number)

        await repo.advance_job(job, "synthesizing", 40)
        await repo.s.flush()

        work = os.path.join(self.work_dir, job.id)
        os.makedirs(work, exist_ok=True)
        try:
            voice_map = {c.name: c.voice for c in bible.characters}
            segments = [
                TTSSegment(
                    speaker=ln.speaker,
                    text=ln.text,
                    voice="narrator" if ln.speaker == "Narrator" else voice_map.get(ln.speaker, "clear_high"),
                )
                for ln in script.lines
            ]
            with timed(TTS_DURATION, "ai-media", self.tts.name):
                tts_result = await self.tts.synthesize(segments, os.path.join(work, "tts"), language=language)

            await repo.advance_job(job, "assembling", 60)
            await self._patch_episode(job.id, episode_id, {"processing": "assembling"})
            await repo.s.flush()

            await repo.advance_job(job, "packaging", 75)
            await self._patch_episode(job.id, episode_id, {"processing": "packaging"})
            await repo.s.flush()
            packaged = await media.package(tts_result.wav_path, work)

            key_prefix = f"hls/{show_id}/{episode_id}"
            self.store.upload_dir(packaged.root, key_prefix)

            media_meta = {
                "hls_master_key": f"{key_prefix}/master.m3u8",
                "variants": [
                    {
                        "bitrate_kbps": v["bitrate_kbps"],
                        "key": f"{key_prefix}/{v['playlist']}",
                        "codec": v["codec"],
                        "size_bytes": v["size_bytes"],
                    }
                    for v in packaged.variants
                ],
                "codec": packaged.codec,
                "sample_rate_hz": packaged.sample_rate_hz,
                "channels": packaged.channels,
                "file_size_bytes": packaged.file_size_bytes,
                "checksum_sha256": packaged.checksum_sha256,
                "duration_sec": packaged.duration_sec,
            }
            await self._patch_episode(job.id, episode_id, {"media": media_meta, "processing": "ready"})
            job.result = {"episode_id": episode_id, "duration_sec": packaged.duration_sec, "words": script.word_count}
            await repo.advance_job(job, "completed", 100, "audio packaged and attached")
        finally:
            shutil.rmtree(work, ignore_errors=True)

    async def package_upload(self, repo: Repo, job: models.GenerationJob) -> None:
        """Handle a creator upload: fetch the source object, package, attach."""
        episode_id = job.episode_id
        source_key = job.prompt["source_key"]
        assert episode_id

        await repo.advance_job(job, "packaging", 30)
        await self._patch_episode(job.id, episode_id, {"processing": "packaging"})
        await repo.s.flush()

        work = os.path.join(self.work_dir, job.id)
        os.makedirs(work, exist_ok=True)
        src_path = os.path.join(work, "source" + os.path.splitext(source_key)[1])
        try:
            self.store._client.fget_object(self.store._bucket, source_key, src_path)
            wav_path = os.path.join(work, "normalized.wav")
            await media._ffmpeg(["-i", src_path, "-ac", "2", "-ar", "44100", wav_path])
            packaged = await media.package(wav_path, work)
            key_prefix = f"hls/{job.show_id}/{episode_id}"
            self.store.upload_dir(packaged.root, key_prefix)
            await self._patch_episode(
                job.id,
                episode_id,
                {
                    "media": {
                        "hls_master_key": f"{key_prefix}/master.m3u8",
                        "variants": [
                            {
                                "bitrate_kbps": v["bitrate_kbps"],
                                "key": f"{key_prefix}/{v['playlist']}",
                                "codec": v["codec"],
                                "size_bytes": v["size_bytes"],
                            }
                            for v in packaged.variants
                        ],
                        "codec": packaged.codec,
                        "sample_rate_hz": packaged.sample_rate_hz,
                        "channels": packaged.channels,
                        "file_size_bytes": packaged.file_size_bytes,
                        "checksum_sha256": packaged.checksum_sha256,
                        "duration_sec": packaged.duration_sec,
                    },
                    "processing": "ready",
                },
            )
            await repo.advance_job(job, "completed", 100, "uploaded audio packaged")
        finally:
            shutil.rmtree(work, ignore_errors=True)

    async def _patch_episode(self, job_id: str, episode_id: str, body: dict) -> None:
        await self.content.patch(f"/internal/authoring/episodes/{episode_id}", body, correlation_id=job_id)


def _outline_from(data: dict):
    from auralis_ai_media.schemas import EpisodeOutline

    return EpisodeOutline.model_validate(data)

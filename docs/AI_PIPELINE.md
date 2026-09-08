# AI Generation Pipeline

The ai-media service turns a short brief into a serialized show: a story bible,
a season of episode outlines, per-episode scripts, synthesized speech, and
packaged HLS audio. It drives the content service through its internal API and
never blocks an HTTP request on generation work.

## Request to job

| Endpoint | Body | Result |
| --- | --- | --- |
| `POST /api/generate/series` | `brief`, `episode_count` (3 to 24), `language_code`, `is_premium`, `seed?` | `202`, `{ job_id, status: "queued" }` |
| `POST /api/generate/episode` | `show_id`, `number`, `title?`, `seed?` | `202`, queued episode job |
| `GET /api/generate/jobs/{id}` | | job status, progress, events |
| `GET /api/ai/jobs` (ADMIN) | `?status=&limit=` | job list |

A request writes a `generation_jobs` row with status `queued` and returns. The
caller polls the job endpoint (the web client shows a progress bar driven by
`job.progress` and `job.events`).

## Worker

A separate process (`ai-media` with `command: ["worker"]`, never the API
process) polls for queued jobs every 2 seconds, claims one with
`next_queued_job`, increments `attempts`, and runs the matching pipeline. One
job at a time per worker; scale by running more worker replicas.

On failure the job goes back to `queued` with the error recorded, up to
`max_attempts`; after that it is marked `failed` and, if it has an episode, the
episode's processing status is set to `failed` on content.

Job statuses: `queued`, `generating_bible`, `generating_outline`,
`generating_script`, `synthesizing`, `assembling`, `packaging`, `completed`,
`failed`. The episode's `processing_status` on content tracks a parallel set
(`queued`, `generating_script`, `synthesizing`, `assembling`, `packaging`,
`ready`, `failed`).

## Series pipeline (`run_series`)

1. **Bible**: `llm.generate_bible(brief, episode_count, seed)` produces the
   concept (title, logline, synopsis), characters (name, role, voice), world
   rules, relationships, and a season arc.
2. **Metadata**: `llm.generate_metadata` produces tags, maturity rating, and an
   accent color for the show card.
3. **Create draft show** on content via `POST /internal/authoring/shows`
   (creator name "Auralis Studio", the requesting user as owner).
4. **Persist** the bible: `series_bibles`, `bible_characters`,
   `bible_relationships`, `bible_world_rules`.
5. **Outlines**: `llm.generate_outlines(bible, seed)` produces one outline per
   episode (number, title, summary).
6. For each outline: create a draft episode on content
   (`POST /internal/authoring/episodes`), then spawn a child `episode` job.
   Episodes past number 3 in a premium show are marked premium with a 90-second
   free preview.
7. The series job completes; its `result` lists the child job ids.

The show and its episodes are drafts. A human still moves them through review
(`ready_for_review` to `approved` to `published`). Generation does not publish.

## Episode pipeline (`run_episode`)

1. Load the bible and assemble a `ContinuityContext`: concept, characters,
   world rules, the **recent episode summaries** (not full scripts), the
   **unresolved plot threads**, the season arc, and this episode's outline.
2. `llm.generate_script(ctx, seed)` produces a titled script as speaker or line
   pairs.
3. Save an `episode_summaries` row for this episode (so later episodes can see
   what happened) and, from the season midpoint on, resolve one open plot
   thread.
4. Patch the script text onto the episode; set processing `synthesizing`.
5. `tts.synthesize(segments, ...)` renders each line to audio using the voice
   assigned to that character in the bible (`Narrator` uses the narrator
   voice).
6. `media.package(...)` assembles and packages HLS (see
   [AUDIO_PROCESSING.md](AUDIO_PROCESSING.md)).
7. Upload the HLS tree to object storage under `hls/{show_id}/{episode_id}/`
   and patch the media metadata (`hls_master_key`, `variants`, duration,
   checksum) onto the episode. Processing status becomes `ready`.

## Continuity without full history

The context passed to the model is bounded: a fixed bible plus the last few
episode summaries plus the list of open threads. It never grows with the season
length, so episode 20 costs the same as episode 2 and the model is not asked to
re-read twenty scripts. Thread resolution is tracked explicitly in
`plot_threads` rather than inferred.

## Providers

Both providers are interfaces (`LLMProvider`, `TTSProvider` Python Protocols)
with two implementations each.

### LLM

- **`LocalLLMProvider`** (default): fully procedural and deterministic by seed.
  It composes a bible, outlines, and scripts from settings, engines, and theme
  tables. No network, no API key, no cost. Output is coherent and consistent
  but templated.
- **`GroqLLMProvider`**: used only when `GROQ_API_KEY` is set. Model
  `openai/gpt-oss-20b` over the OpenAI-compatible API with JSON-schema
  structured output. Any error (network, rate limit, malformed output) falls
  back to the local provider for that call, so a job never fails because Groq
  is unavailable.

`AI_DEFAULT_PROVIDER` is `auto` (Groq if keyed, else local), `local`, or
`groq`.

### TTS

- **`LocalTTSProvider`** (default): espeak-ng. Always available, robotic.
- **`PiperTTSProvider`**: Piper neural voices, used when a Piper model file is
  present on disk.

## Determinism

Every generation path takes a `seed`. The same brief and seed produce the same
bible, outlines, and scripts with the local provider, which is what makes the
seed script and the tests reproducible.

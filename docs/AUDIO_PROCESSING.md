# Audio Processing

How a source WAV becomes streamable HLS. This runs in the ai-media worker
only, never in an HTTP handler, in `pipeline/media.py`.

## Inputs

- A generated episode: the TTS provider renders each script line and assembles
  one source WAV.
- A creator upload: the raw file lands in object storage, `content` emits
  `content.audio_uploaded`, and ai-media picks it up as a `media` job.

## Pipeline

1. **Probe the source** with `ffprobe`. Reject if it is missing or shorter than
   3 seconds.
2. **Encode three AAC renditions** with FFmpeg, one pass each:
   - bitrates: 64, 128, 256 kbps
   - `-c:a aac -ar 44100 -ac 2`
   - HLS output: `-f hls -hls_time 6 -hls_playlist_type vod`
   - segments: `audio_<kbps>_%03d.ts`, playlist `audio_<kbps>.m3u8`
3. **Write the master playlist** `master.m3u8` with one
   `#EXT-X-STREAM-INF:BANDWIDTH=<kbps*1000>,CODECS="mp4a.40.2"` entry per
   rendition.
4. **Validate**: `ffprobe` the 256 kbps playlist, confirm it has an audio
   stream, derive duration, sample rate, channels, and codec from it. Reject
   if the total segment bytes are implausibly small (under 1 KB).
5. **Checksum**: SHA-256 of the source WAV, stored with the media metadata so a
   re-package can be detected as identical.

Output tree:

```
hls/
  master.m3u8
  audio_64.m3u8    audio_64_000.ts  audio_64_001.ts ...
  audio_128.m3u8   audio_128_000.ts ...
  audio_256.m3u8   audio_256_000.ts ...
```

## Upload and attach

The whole `hls/` tree is uploaded to object storage under
`hls/{show_id}/{episode_id}/`. Then ai-media patches the episode on content:

```json
{
  "hls_master_key": "hls/<show>/<episode>/master.m3u8",
  "variants": [
    { "bitrate_kbps": 64,  "key": "hls/.../audio_64.m3u8",  "codec": "aac", "size_bytes": 812340 },
    { "bitrate_kbps": 128, "key": "hls/.../audio_128.m3u8", "codec": "aac", "size_bytes": 1521002 },
    { "bitrate_kbps": 256, "key": "hls/.../audio_256.m3u8", "codec": "aac", "size_bytes": 2903114 }
  ],
  "duration_sec": 1180,
  "checksum_sha256": "…",
  "processing": "ready"
}
```

Publishing the episode later emits `content.episode_published`; playback then
pulls these keys into its cache.

## Delivery

At authorize time, playback returns URLs for `hls_master_key` and every variant
key. The web client loads the master URL with hls.js, which picks a rendition
based on measured bandwidth. Segments are fetched directly from object storage.
No audio byte passes through an Auralis service.

How those URLs are formed depends on `S3_PUBLIC_BASE_URL`:

- **Set** (production): playback returns plain `S3_PUBLIC_BASE_URL + key` URLs.
  The media bucket is exposed through a public read-only domain (an R2 `r2.dev`
  domain or a custom domain) with a CORS policy that allows GET and HEAD from
  the web client's origin. This is what desktop browsers need: hls.js resolves
  the variant playlists and segments relative to the master, and a browser
  drops the query string when it does that, so anything signed would come back
  unsigned and 403. A public prefix sidesteps the whole problem, and the bucket
  still holds nothing private.
- **Unset** (local, single-origin demos): playback presigns the master and
  every variant key with a 2-hour TTL. This is fine for native HLS (iOS Safari)
  and for tools that keep the query string, but not for hls.js in a desktop
  browser.

Set `S3_PUBLIC_BASE_URL` on the playback service to the public media domain,
with no trailing slash.

## Voices

Generated episodes are voiced by **Piper**, a free offline neural TTS. The
ai-media image bundles the `piper` binary and six medium-quality voice models
(about 360 MB total), fetched at build time by `scripts/piper-fetch-voices.sh`.
The pipeline assigns each character an abstract voice key in the story bible
(`narrator`, `low_warm`, `bright_quick`, `dry_measured`, `rough_soft`,
`clear_high`); `providers/tts.py` maps each key to one Piper model and a
`length_scale` that nudges the pace. A key whose model is missing falls back to
the narrator voice; if the narrator model or the binary is missing entirely the
service drops to **espeak-ng**, which is intelligible but plainly synthetic.

Selection is by env: `PIPER_VOICES_DIR` (the models directory) and `PIPER_BIN`
(defaults to `piper` on `PATH`). For local development, `scripts/piper-setup.sh`
downloads the binary and models into `.piper/` and `scripts/dev-native.sh`
points the worker at them. The media backfill script
(`scripts/deploy/media-backfill.py`) uses the same resolution.

## Requirements

`ffmpeg` and `ffprobe` on `PATH` in the worker image (the compose and Render
images install them). For speech, either the bundled Piper binary and voice
models, or `espeak-ng` as the fallback.

## Failure handling

Any FFmpeg or ffprobe non-zero exit raises `GenerationError`, which fails the
job attempt. The worker retries up to `max_attempts`; a terminal failure sets
the episode processing status to `failed` with a truncated error message
visible in the creator UI and the admin jobs view.

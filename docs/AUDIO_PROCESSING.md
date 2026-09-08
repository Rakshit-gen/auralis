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

At authorize time, playback presigns `hls_master_key` and every variant key
with a 2-hour TTL and returns the URLs. The web client loads the master URL
with hls.js, which picks a rendition based on measured bandwidth. Segments are
fetched directly from object storage. No audio byte passes through an Auralis
service.

Because the variant playlists reference segments by relative name and the
whole tree is in one prefix, a presigned master URL plus presigned variant
URLs are enough; the client rewrites segment requests against the same signed
prefix.

## Requirements

`ffmpeg` and `ffprobe` on `PATH` in the worker image (the compose and Render
images install them). `espeak-ng` for the local TTS provider. Piper is
optional and only used if a model file is present.

## Failure handling

Any FFmpeg or ffprobe non-zero exit raises `GenerationError`, which fails the
job attempt. The worker retries up to `max_attempts`; a terminal failure sets
the episode processing status to `failed` with a truncated error message
visible in the creator UI and the admin jobs view.

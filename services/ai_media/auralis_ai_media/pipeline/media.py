"""Audio packaging: WAV master to multi-bitrate AAC to HLS.

FFmpeg is never run inside an HTTP request; this module is only called from the
worker. It produces 64, 128, and 256 kbps renditions, an HLS master playlist,
and validates the output before it is handed back.
"""

from __future__ import annotations

import hashlib
import json
import os
import secrets
import shutil

import structlog
from auralis_common.telemetry import FFMPEG_DURATION, timed

from auralis_ai_media.providers.base import GenerationError, run_process

log = structlog.get_logger()

BITRATES = (64, 128, 256)
HLS_SEGMENT_SECONDS = 6


# Encoding a long episode takes a while; anything past these is stuck.
FFMPEG_TIMEOUT = 900
FFPROBE_TIMEOUT = 60


async def _ffmpeg(args: list[str]) -> None:
    await run_process(["ffmpeg", "-hide_banner", "-loglevel", "error", "-y", *args], limit_sec=FFMPEG_TIMEOUT)


async def _ffprobe(path: str) -> dict:
    out = await run_process(
        [
            "ffprobe",
            "-hide_banner",
            "-loglevel",
            "error",
            "-print_format",
            "json",
            "-show_format",
            "-show_streams",
            path,
        ],
        limit_sec=FFPROBE_TIMEOUT,
    )
    return json.loads(out)


# The demuxer for each upload extension content allows. Naming it stops
# ffmpeg guessing the format from the file's contents, so an upload can't
# turn out to be a playlist or concat list that makes ffmpeg read other files
# or URLs.
UPLOAD_FORMATS = {".mp3": "mp3", ".m4a": "mov", ".wav": "wav", ".flac": "flac", ".ogg": "ogg"}


async def decode_upload(src_path: str, wav_path: str) -> None:
    """Decode a creator's upload to a stereo 44.1 kHz WAV, reading nothing
    but the file itself."""
    fmt = UPLOAD_FORMATS.get(os.path.splitext(src_path)[1].lower())
    if fmt is None:
        raise GenerationError("unsupported upload format")
    args = ["-protocol_whitelist", "file", "-f", fmt, "-i", src_path]
    await _ffmpeg([*args, "-map", "0:a:0", "-vn", "-ac", "2", "-ar", "44100", wav_path])


def _randomize_segment_names(hls_dir: str, playlist: str) -> list[str]:
    """Rename a rendition's segments to random names and rewrite its playlist.

    The bucket is publicly readable, so numbered names let anyone who learned
    one segment URL (a preview listener, say) fetch the rest of a premium
    episode. With random names, only the playlist, which playback serves and
    cuts off at the preview limit, says where the segments are.
    """
    with open(playlist) as f:
        lines = f.read().splitlines()
    names = []
    for i, line in enumerate(lines):
        if line and not line.startswith("#"):
            new = f"{secrets.token_hex(16)}.ts"
            os.rename(os.path.join(hls_dir, line), os.path.join(hls_dir, new))
            lines[i] = new
            names.append(new)
    with open(playlist, "w") as f:
        f.write("\n".join(lines) + "\n")
    return names


def key_prefix(show_id: str, episode_id: str) -> str:
    """Where an episode's HLS files go. The random part keeps the playlists,
    which list every segment, from being found by guessing the path."""
    return f"hls/{show_id}/{episode_id}/{secrets.token_hex(16)}"


class PackagedAudio:
    def __init__(self, root: str):
        self.root = root
        self.master_playlist = os.path.join(root, "master.m3u8")
        self.variants: list[dict] = []
        self.duration_sec = 0
        self.sample_rate_hz = 0
        self.channels = 0
        self.codec = "aac"
        self.file_size_bytes = 0
        self.checksum_sha256 = ""


async def package(wav_path: str, work_dir: str) -> PackagedAudio:
    """Encode wav_path into HLS renditions under work_dir/hls."""
    if not os.path.exists(wav_path):
        raise GenerationError("source audio is missing")
    probe = await _ffprobe(wav_path)
    fmt = probe.get("format", {})
    source_duration = float(fmt.get("duration", 0) or 0)
    if source_duration < 3:
        raise GenerationError(f"source audio is too short ({source_duration:.1f}s)")

    hls_dir = os.path.join(work_dir, "hls")
    if os.path.isdir(hls_dir):
        shutil.rmtree(hls_dir)
    os.makedirs(hls_dir, exist_ok=True)

    result = PackagedAudio(hls_dir)
    master_lines = ["#EXTM3U", "#EXT-X-VERSION:3"]

    for kbps in BITRATES:
        name = f"audio_{kbps}"
        playlist = os.path.join(hls_dir, f"{name}.m3u8")
        with timed(FFMPEG_DURATION, "ai-media", f"encode_{kbps}"):
            await _ffmpeg(
                [
                    "-i",
                    wav_path,
                    "-c:a",
                    "aac",
                    "-b:a",
                    f"{kbps}k",
                    "-ar",
                    "44100",
                    "-ac",
                    "2",
                    "-f",
                    "hls",
                    "-hls_time",
                    str(HLS_SEGMENT_SECONDS),
                    "-hls_playlist_type",
                    "vod",
                    "-hls_segment_filename",
                    os.path.join(hls_dir, f"{name}_%03d.ts"),
                    playlist,
                ]
            )
        segments = _randomize_segment_names(hls_dir, playlist)
        seg_bytes = sum(os.path.getsize(os.path.join(hls_dir, f)) for f in segments)
        result.variants.append(
            {"bitrate_kbps": kbps, "playlist": f"{name}.m3u8", "codec": "aac", "size_bytes": seg_bytes}
        )
        master_lines.append(f'#EXT-X-STREAM-INF:BANDWIDTH={kbps * 1000},CODECS="mp4a.40.2"')
        master_lines.append(f"{name}.m3u8")

    with open(result.master_playlist, "w") as f:
        f.write("\n".join(master_lines) + "\n")

    # Validate the highest rendition and derive metadata from it.
    top = os.path.join(hls_dir, "audio_256.m3u8")
    top_probe = await _ffprobe(top)
    stream = next((s for s in top_probe.get("streams", []) if s.get("codec_type") == "audio"), None)
    if stream is None:
        raise GenerationError("packaged audio has no audio stream")
    result.duration_sec = round(float(top_probe.get("format", {}).get("duration", source_duration)))
    result.sample_rate_hz = int(stream.get("sample_rate", 44100))
    result.channels = int(stream.get("channels", 2))
    result.codec = stream.get("codec_name", "aac")
    result.file_size_bytes = sum(
        os.path.getsize(os.path.join(hls_dir, f)) for f in os.listdir(hls_dir) if f.endswith(".ts")
    )
    if result.file_size_bytes < 1024:
        raise GenerationError("packaged audio is implausibly small")

    h = hashlib.sha256()
    with open(wav_path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    result.checksum_sha256 = h.hexdigest()

    log.info(
        "audio packaged",
        duration_sec=result.duration_sec,
        bitrates=list(BITRATES),
        size_bytes=result.file_size_bytes,
    )
    return result

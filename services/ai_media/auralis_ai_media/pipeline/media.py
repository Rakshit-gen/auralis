"""Audio packaging: WAV master to multi-bitrate AAC to HLS.

FFmpeg is never run inside an HTTP request; this module is only called from the
worker. It produces 64, 128, and 256 kbps renditions, an HLS master playlist,
and validates the output before it is handed back.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import os
import shutil

import structlog
from auralis_common.telemetry import FFMPEG_DURATION, timed

from auralis_ai_media.providers.base import GenerationError

log = structlog.get_logger()

BITRATES = (64, 128, 256)
HLS_SEGMENT_SECONDS = 6


async def _ffmpeg(args: list[str]) -> None:
    proc = await asyncio.create_subprocess_exec(
        "ffmpeg",
        "-hide_banner",
        "-loglevel",
        "error",
        "-y",
        *args,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    _, stderr = await proc.communicate()
    if proc.returncode != 0:
        raise GenerationError(f"ffmpeg failed: {stderr.decode()[:400]}")


async def _ffprobe(path: str) -> dict:
    proc = await asyncio.create_subprocess_exec(
        "ffprobe",
        "-hide_banner",
        "-loglevel",
        "error",
        "-print_format",
        "json",
        "-show_format",
        "-show_streams",
        path,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    out, err = await proc.communicate()
    if proc.returncode != 0:
        raise GenerationError(f"ffprobe failed: {err.decode()[:200]}")
    return json.loads(out)


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
        seg_bytes = sum(
            os.path.getsize(os.path.join(hls_dir, f))
            for f in os.listdir(hls_dir)
            if f.startswith(f"{name}_") and f.endswith(".ts")
        )
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

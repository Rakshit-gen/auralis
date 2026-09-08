"""TTS providers.

LocalTTSProvider uses espeak-ng, which is free, offline, and available in every
container. PiperTTSProvider uses Piper neural voices (also free and offline) when
a voice model is present on disk; it is the "production" adapter. Both render
each line to a WAV segment and concatenate them with short pauses.
"""

from __future__ import annotations

import asyncio
import os
import shutil
import wave

import structlog

from auralis_ai_media.providers.base import GenerationError, TTSResult, TTSSegment

log = structlog.get_logger()

_SAMPLE_RATE = 22050

# espeak-ng voice variants mapped to the abstract voice keys used in the bible.
_ESPEAK_VOICES = {
    "narrator": "en-us",
    "low_warm": "en-us+m3",
    "bright_quick": "en-us+f4",
    "dry_measured": "en-us+m1",
    "rough_soft": "en-us+m5",
    "clear_high": "en-us+f2",
}


async def _run(cmd: list[str]) -> None:
    proc = await asyncio.create_subprocess_exec(*cmd, stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE)
    _, stderr = await proc.communicate()
    if proc.returncode != 0:
        raise GenerationError(f"{cmd[0]} failed: {stderr.decode()[:300]}")


def _wav_duration(path: str) -> tuple[float, int, int]:
    with wave.open(path, "rb") as w:
        frames = w.getnframes()
        rate = w.getframerate()
        channels = w.getnchannels()
        return (frames / rate if rate else 0.0), rate, channels


def _silence_wav(path: str, seconds: float, rate: int = _SAMPLE_RATE) -> None:
    frames = int(seconds * rate)
    with wave.open(path, "wb") as w:
        w.setnchannels(1)
        w.setsampwidth(2)
        w.setframerate(rate)
        w.writeframes(b"\x00\x00" * frames)


async def _concat(segment_paths: list[str], out_path: str) -> None:
    """Concatenate mono 16-bit WAV files that share a sample rate."""
    if not segment_paths:
        raise GenerationError("no audio segments to assemble")
    with wave.open(segment_paths[0], "rb") as first:
        params = first.getparams()
    with wave.open(out_path, "wb") as out:
        out.setparams(params)
        for p in segment_paths:
            with wave.open(p, "rb") as seg:
                out.writeframes(seg.readframes(seg.getnframes()))


class LocalTTSProvider:
    name = "espeak"

    def __init__(self) -> None:
        self._bin = shutil.which("espeak-ng") or shutil.which("espeak")
        if not self._bin:
            raise GenerationError("espeak-ng is not installed")

    async def synthesize(self, segments: list[TTSSegment], out_dir: str) -> TTSResult:
        os.makedirs(out_dir, exist_ok=True)
        parts: list[str] = []
        for i, seg in enumerate(segments):
            voice = _ESPEAK_VOICES.get(seg.voice, "en-us")
            seg_path = os.path.join(out_dir, f"seg_{i:04d}.wav")
            await _run([self._bin, "-v", voice, "-s", "165", "-w", seg_path, seg.text.replace("\n", " ")[:1800]])
            parts.append(seg_path)
            pause_path = os.path.join(out_dir, f"pause_{i:04d}.wav")
            _silence_wav(pause_path, 0.45 if seg.speaker == "Narrator" else 0.3)
            parts.append(pause_path)

        combined = os.path.join(out_dir, "voice.wav")
        await _concat(parts, combined)
        duration, rate, channels = _wav_duration(combined)
        log.info("tts synthesized", provider=self.name, segments=len(segments), duration_sec=round(duration, 1))
        return TTSResult(wav_path=combined, duration_sec=duration, sample_rate=rate, channels=channels)


class PiperTTSProvider:
    name = "piper"

    def __init__(self, model_path: str) -> None:
        self._bin = shutil.which("piper")
        self._model = model_path
        if not self._bin or not os.path.exists(model_path):
            raise GenerationError("piper binary or voice model not available")

    async def synthesize(self, segments: list[TTSSegment], out_dir: str) -> TTSResult:
        os.makedirs(out_dir, exist_ok=True)
        parts: list[str] = []
        for i, seg in enumerate(segments):
            seg_path = os.path.join(out_dir, f"seg_{i:04d}.wav")
            proc = await asyncio.create_subprocess_exec(
                self._bin,
                "--model",
                self._model,
                "--output_file",
                seg_path,
                stdin=asyncio.subprocess.PIPE,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )
            _, stderr = await proc.communicate(seg.text.replace("\n", " ")[:1800].encode())
            if proc.returncode != 0:
                raise GenerationError(f"piper failed: {stderr.decode()[:200]}")
            parts.append(seg_path)
            pause_path = os.path.join(out_dir, f"pause_{i:04d}.wav")
            with wave.open(seg_path, "rb") as w:
                rate = w.getframerate()
            _silence_wav(pause_path, 0.4, rate=rate)
            parts.append(pause_path)

        combined = os.path.join(out_dir, "voice.wav")
        await _concat(parts, combined)
        duration, rate, channels = _wav_duration(combined)
        return TTSResult(wav_path=combined, duration_sec=duration, sample_rate=rate, channels=channels)

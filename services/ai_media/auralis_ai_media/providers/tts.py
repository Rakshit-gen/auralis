"""TTS providers.

PiperTTSProvider renders each line with a Piper neural voice (free, offline) and
is the one we ship. It needs the piper binary and the voice models on disk; when
either is missing the service falls back to LocalTTSProvider, which drives
espeak-ng. espeak is intelligible but plainly synthetic, so it is a floor, not
the intended sound.

Both providers render each script line to its own WAV segment, insert a short
pause after it, and concatenate the segments into one voice track.
"""

from __future__ import annotations

import asyncio
import os
import shutil
import wave

import structlog

from auralis_ai_media.languages import normalize
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

# Non-English espeak-ng languages. espeak has one voice per language, so the
# abstract voice keys collapse to gendered variants of the same base.
_ESPEAK_LANG_BASE = {"en": "en-us", "hi": "hi", "es": "es"}

# The abstract voice keys the story bible assigns to characters, mapped to a
# Piper voice model. The narrator model is required; the rest degrade to it if a
# model file is missing. length_scale > 1 slows the delivery a little, which
# suits narration; the character voices sit near 1.0 with small offsets so a
# two-hander does not sound like one person talking to themselves.
_PIPER_VOICES = {
    "narrator": ("en_US-lessac-medium", 1.04),
    "low_warm": ("en_US-ryan-medium", 1.0),
    "bright_quick": ("en_US-amy-medium", 0.98),
    "dry_measured": ("en_GB-alan-medium", 1.03),
    "rough_soft": ("en_US-hfc_male-medium", 1.01),
    "clear_high": ("en_US-hfc_female-medium", 0.99),
}
_PIPER_NARRATOR_MODEL = _PIPER_VOICES["narrator"][0]

# Per-language voice sets. Non-English languages have fewer distinct Piper
# models, so several abstract keys share one. Any model file that is not on disk
# is skipped at load time and resolves to that language's narrator, then to the
# English narrator, so a partial download still produces audio.
_PIPER_VOICES_BY_LANG: dict[str, dict[str, tuple[str, float]]] = {
    "en": _PIPER_VOICES,
    "hi": {
        "narrator": ("hi_IN-pratham-medium", 1.04),
        "low_warm": ("hi_IN-pratham-medium", 1.0),
        "bright_quick": ("hi_IN-priyamvada-medium", 0.98),
        "dry_measured": ("hi_IN-pratham-medium", 1.03),
        "rough_soft": ("hi_IN-pratham-medium", 1.01),
        "clear_high": ("hi_IN-priyamvada-medium", 0.99),
    },
    "es": {
        "narrator": ("es_ES-davefx-medium", 1.04),
        "low_warm": ("es_ES-davefx-medium", 1.0),
        "bright_quick": ("es_ES-sharvard-medium", 0.98),
        "dry_measured": ("es_ES-davefx-medium", 1.03),
        "rough_soft": ("es_ES-davefx-medium", 1.01),
        "clear_high": ("es_ES-sharvard-medium", 0.99),
    },
}


async def _run(cmd: list[str], *, stdin: bytes | None = None, env: dict[str, str] | None = None) -> None:
    proc = await asyncio.create_subprocess_exec(
        *cmd,
        stdin=asyncio.subprocess.PIPE if stdin is not None else None,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        env=env,
    )
    _, stderr = await proc.communicate(stdin)
    if proc.returncode != 0:
        raise GenerationError(f"{os.path.basename(cmd[0])} failed: {stderr.decode()[:300]}")


def _wav_duration(path: str) -> tuple[float, int, int]:
    with wave.open(path, "rb") as w:
        frames = w.getnframes()
        rate = w.getframerate()
        channels = w.getnchannels()
        return (frames / rate if rate else 0.0), rate, channels


def _silence_wav(path: str, seconds: float, rate: int, channels: int = 1, width: int = 2) -> None:
    frames = int(seconds * rate)
    with wave.open(path, "wb") as w:
        w.setnchannels(channels)
        w.setsampwidth(width)
        w.setframerate(rate)
        w.writeframes(b"\x00" * (frames * channels * width))


async def _concat(segment_paths: list[str], out_path: str) -> None:
    """Concatenate 16-bit WAV files, skipping any whose format does not match the first."""
    if not segment_paths:
        raise GenerationError("no audio segments to assemble")
    with wave.open(segment_paths[0], "rb") as first:
        params = first.getparams()
    with wave.open(out_path, "wb") as out:
        out.setparams(params)
        for p in segment_paths:
            with wave.open(p, "rb") as seg:
                if seg.getframerate() != params.framerate or seg.getnchannels() != params.nchannels:
                    log.warning("tts segment format mismatch, skipped", path=os.path.basename(p))
                    continue
                out.writeframes(seg.readframes(seg.getnframes()))


class LocalTTSProvider:
    name = "espeak"

    def __init__(self) -> None:
        self._bin = shutil.which("espeak-ng") or shutil.which("espeak")
        if not self._bin:
            raise GenerationError("espeak-ng is not installed")

    async def synthesize(self, segments: list[TTSSegment], out_dir: str, language: str = "en") -> TTSResult:
        os.makedirs(out_dir, exist_ok=True)
        lang = normalize(language)
        parts: list[str] = []
        rate = _SAMPLE_RATE
        for i, seg in enumerate(segments):
            if lang == "en":
                voice = _ESPEAK_VOICES.get(seg.voice, "en-us")
            else:
                voice = _ESPEAK_LANG_BASE.get(lang, "en-us")
            seg_path = os.path.join(out_dir, f"seg_{i:04d}.wav")
            await _run([self._bin, "-v", voice, "-s", "165", "-w", seg_path, seg.text.replace("\n", " ")[:1800]])
            with wave.open(seg_path, "rb") as w:
                rate = w.getframerate()
            parts.append(seg_path)
            pause_path = os.path.join(out_dir, f"pause_{i:04d}.wav")
            _silence_wav(pause_path, 0.45 if seg.speaker == "Narrator" else 0.3, rate)
            parts.append(pause_path)

        combined = os.path.join(out_dir, "voice.wav")
        await _concat(parts, combined)
        duration, rate, channels = _wav_duration(combined)
        log.info("tts synthesized", provider=self.name, segments=len(segments), duration_sec=round(duration, 1))
        return TTSResult(wav_path=combined, duration_sec=duration, sample_rate=rate, channels=channels)


class PiperTTSProvider:
    name = "piper"

    def __init__(self, voices_dir: str, binary: str = "") -> None:
        self._bin = binary or os.environ.get("PIPER_BIN") or shutil.which("piper") or ""
        if not self._bin or not os.path.isfile(self._bin):
            raise GenerationError("piper binary not found (set PIPER_BIN or put piper on PATH)")
        self._bin_dir = os.path.dirname(os.path.abspath(self._bin))

        if not voices_dir or not os.path.isdir(voices_dir):
            raise GenerationError(f"piper voices directory not found: {voices_dir!r}")
        self._voices_dir = voices_dir

        # (language, voice key) -> (onnx path, length scale), for every model
        # file that is actually present.
        self._models: dict[tuple[str, str], tuple[str, float]] = {}
        for lang, vmap in _PIPER_VOICES_BY_LANG.items():
            for key, (model_name, length_scale) in vmap.items():
                onnx = os.path.join(voices_dir, f"{model_name}.onnx")
                if os.path.isfile(onnx) and os.path.isfile(onnx + ".json"):
                    self._models[(lang, key)] = (onnx, length_scale)
        if ("en", "narrator") not in self._models:
            raise GenerationError(f"piper narrator voice {_PIPER_NARRATOR_MODEL}.onnx missing from {voices_dir}")
        for lang, vmap in _PIPER_VOICES_BY_LANG.items():
            missing = [k for k in vmap if (lang, k) not in self._models]
            if missing:
                log.warning("piper voices missing, will reuse narrator", language=lang, keys=missing)

    def _resolve(self, lang: str, voice_key: str) -> tuple[str, float]:
        for candidate in (lang, "en"):
            hit = self._models.get((candidate, voice_key)) or self._models.get((candidate, "narrator"))
            if hit:
                return hit
        return self._models[("en", "narrator")]

    @property
    def voice_count(self) -> int:
        return len(self._models)

    def _env(self) -> dict[str, str]:
        env = dict(os.environ)
        existing = env.get("LD_LIBRARY_PATH", "")
        env["LD_LIBRARY_PATH"] = f"{self._bin_dir}:{existing}" if existing else self._bin_dir
        return env

    async def synthesize(self, segments: list[TTSSegment], out_dir: str, language: str = "en") -> TTSResult:
        os.makedirs(out_dir, exist_ok=True)
        lang = normalize(language)
        env = self._env()
        parts: list[str] = []
        rate = _SAMPLE_RATE
        for i, seg in enumerate(segments):
            onnx, length_scale = self._resolve(lang, seg.voice)
            seg_path = os.path.join(out_dir, f"seg_{i:04d}.wav")
            await _run(
                [
                    self._bin,
                    "--model",
                    onnx,
                    "--config",
                    onnx + ".json",
                    "--output_file",
                    seg_path,
                    "--length_scale",
                    f"{length_scale}",
                    "--sentence_silence",
                    "0.3",
                ],
                stdin=seg.text.replace("\n", " ").strip()[:1800].encode(),
                env=env,
            )
            with wave.open(seg_path, "rb") as w:
                rate = w.getframerate()
            parts.append(seg_path)
            pause_path = os.path.join(out_dir, f"pause_{i:04d}.wav")
            _silence_wav(pause_path, 0.5 if seg.speaker == "Narrator" else 0.32, rate)
            parts.append(pause_path)

        combined = os.path.join(out_dir, "voice.wav")
        await _concat(parts, combined)
        duration, rate, channels = _wav_duration(combined)
        log.info("tts synthesized", provider=self.name, segments=len(segments), duration_sec=round(duration, 1))
        return TTSResult(wav_path=combined, duration_sec=duration, sample_rate=rate, channels=channels)

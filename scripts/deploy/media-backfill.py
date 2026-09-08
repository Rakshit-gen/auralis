#!/usr/bin/env python3
"""Generate real narrated HLS audio for the published catalogue and attach it.

The catalogue seed (scripts/seed.py) attaches fictional packaged-audio metadata
but never produces audio, so seeded episodes 404 on playback. This script closes
that gap: for every published episode it composes an original narration from the
show and episode metadata, synthesizes it with espeak-ng, packages it to the
same three-bitrate HLS layout the ai-media worker produces, uploads it to the
media bucket under the key the episode already points at
(hls/<show_id>/<episode_id>/...), and re-attaches the real media metadata.

It is idempotent (episodes that already have a real master playlist are skipped
unless --force) and stays within a byte budget so the R2 free tier is safe.

    python scripts/deploy/media-backfill.py                 # up to --target-gb
    python scripts/deploy/media-backfill.py --dry-run       # plan only
    python scripts/deploy/media-backfill.py --limit 5       # first 5 episodes
    python scripts/deploy/media-backfill.py --only-show the-long-room

Requires ffmpeg, ffprobe and espeak-ng on PATH. Reads S3 and gateway settings
from .env.deploy (repo root).
"""

from __future__ import annotations

import argparse
import contextlib
import hashlib
import json
import os
import random
import re
import shutil
import subprocess
import sys
import tempfile
import wave
from pathlib import Path

import httpx
from minio import Minio
from minio.error import S3Error

ROOT = Path(__file__).resolve().parents[2]
ENV_DEPLOY = ROOT / ".env.deploy"

BITRATES = (64, 128, 256)
HLS_SEGMENT_SECONDS = 6
TOTAL_KBPS = sum(BITRATES)  # ~= combined size of the three renditions
ESPEAK_WPM = 165
WORDS_PER_SEC = ESPEAK_WPM / 60.0

# espeak-ng voice variants for the narrator and the two speakers used in the
# dialogue beats. All offline, all in the base package.
VOICES = {"narrator": "en-us", "a": "en-us+m3", "b": "en-us+f3"}


# --------------------------------------------------------------------------- env
def load_env() -> dict[str, str]:
    out = dict(os.environ)
    if ENV_DEPLOY.exists():
        for line in ENV_DEPLOY.read_text().splitlines():
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                k, _, v = line.partition("=")
                out[k.strip()] = v.strip()
    return out


def require_tools() -> None:
    missing = [t for t in ("ffmpeg", "ffprobe", "espeak-ng") if not shutil.which(t)]
    if missing:
        sys.exit(f"missing required tools: {', '.join(missing)}")


# ------------------------------------------------------------------- narration
# The backfill narration is procedural, deterministic per episode, and built
# from the show and episode metadata. It is not a transcript of anything; it is
# original connective prose so the seeded catalogue has real audio to serve.
_REFLECTIONS = [
    "Nobody in {setting} says the important thing on the first try. They circle it the way you circle a hole in the floor.",
    "The work does not care whether {lead} is ready. It arrives on its own schedule and expects to be met.",
    "There is a version of this where everyone keeps their hands clean. {lead} has stopped believing they live in it.",
    "This is a story about the difference between carrying a thing and setting it down, and how rarely anyone gets the choice.",
    "A debt spoken aloud is still a debt. {lead} has learned that saying it does not make it lighter, only harder to ignore.",
    "The room remembers what was said in it. That is the rule here, and everyone acts as if it is not.",
    "{lead} keeps a list of what can wait. The list has not gotten shorter in a long time.",
    "Everyone in {setting} agrees on the facts. What they cannot agree on is what the facts are asking of them.",
    "The night before a decision always feels longer than the decision. {lead} has stopped trying to sleep through it.",
    "There is a kind of silence that means agreement and a kind that means the opposite. {lead} has learned to tell them apart.",
    "What breaks first is never the thing you reinforced. It is the join you forgot was holding weight.",
    "You can be right about the problem and still wrong about the hour. Timing is its own kind of honesty.",
    "The people who stay are not always the loyal ones. Sometimes they are only the ones with nowhere else to be.",
    "{lead} has started measuring the day in the conversations avoided rather than the ones had.",
]
_TURNS = [
    ("We do not have the room to do this the slow way.", "The slow way is the only way that has ever held."),
    ("Tell me what it cost this time. All of it.", "You want it itemised. It stopped being a list a long while ago."),
    ("You said this was handled.", "It is handled the way weather is handled. You stand in it and keep moving."),
    ("I need you to be honest with me for one minute.", "One minute. Then we go back to the useful version of talking."),
    ("Someone has to decide.", "Someone already did. That is the part you are not going to like."),
    ("Why are you telling me this now?", "Because there is no version of later where it is easier to hear."),
    ("If we do this, there is no undoing it.", "There was no undoing the last one either. We just pretended otherwise."),
    ("What do you want me to say?", "Nothing you do not mean. I have had enough of the other kind."),
    ("I can hold it a little longer.", "You have been holding it for a year. That is not holding, that is hiding."),
    ("They will ask who signed off on it.", "Then they will find my name, and I will still be standing here."),
    ("We could wait until morning.", "Morning does not change what the water is doing."),
    ("You are asking me to trust you.", "I am asking you to act as if you do. The trust can come after."),
]
_TAG_LINES = [
    "The word that keeps coming up is {tag}. Everyone uses it to mean something slightly different.",
    "Later, someone will call this a matter of {tag}. Right now it is just a room and a clock.",
    "If there is a lesson about {tag} here, nobody in the room is in the mood to hear it.",
]


def compose_narration(show: dict, episode: dict, target_sec: int, seed: int) -> list[tuple[str, str]]:
    """Return a list of (voice_key, text) lines totalling roughly target_sec of speech."""
    rng = random.Random(seed)
    show_title = show.get("title", "the series")
    setting = show.get("synopsis") or "a small place at the edge of somewhere larger"
    setting_short = _first_clause(setting)
    ep_title = episode.get("title", "this episode")
    ep_num = episode.get("number", 1)
    ep_syn = (episode.get("synopsis") or "").strip()
    generic_syn = not ep_syn or re.fullmatch(r"Episode \d+ of .+\.?", ep_syn) is not None
    tags = [t for t in (show.get("tags") or []) if isinstance(t, str)][:4]
    lead = _lead_name(rng)

    lines: list[tuple[str, str]] = [("narrator", f"{show_title}. {ep_title}.")]
    scene = setting_short[:1].lower() + setting_short[1:] if setting_short[:2] in ("A ", "An") or setting_short[:4] == "The " else setting_short
    opener = f"The scene is {scene}."
    if not generic_syn:
        opener += f" {ep_syn if ep_syn.endswith('.') else ep_syn + '.'}"
    else:
        opener += f" This is the {_ordinal(ep_num)} part, and it turns on a decision that will not wait."
    lines.append(("narrator", opener))

    budget_words = max(120, int(target_sec * WORDS_PER_SEC))
    reflect = rng.sample(_REFLECTIONS, k=len(_REFLECTIONS))
    turns = rng.sample(_TURNS, k=len(_TURNS))
    beat = 0

    def used() -> int:
        return sum(len(t.split()) for _, t in lines)

    while used() < budget_words:
        lines.append(("narrator", reflect[beat % len(reflect)].format(setting=setting_short, lead=lead, show=show_title)))
        q, a = turns[beat % len(turns)]
        lines.append(("a", q))
        lines.append(("b", a))
        if tags and rng.random() < 0.55:
            tag = tags[beat % len(tags)]
            lines.append(("narrator", rng.choice(_TAG_LINES).format(tag=tag)))
        if rng.random() < 0.4:
            q2, a2 = turns[(beat + 3) % len(turns)]
            lines.append(("b", q2))
            lines.append(("a", a2))
        beat += 1

    lines.append(("narrator", f"The {_ordinal(ep_num)} part closes on {lead}, and on the question the room has been avoiding since it started."))
    return lines


def _ordinal(n: int) -> str:
    words = ["zeroth", "first", "second", "third", "fourth", "fifth", "sixth", "seventh",
             "eighth", "ninth", "tenth", "eleventh", "twelfth"]
    return words[n] if 0 <= n < len(words) else f"{n}th"


def _first_clause(text: str) -> str:
    text = re.sub(r"\s+", " ", text).strip()
    for sep in (". ", "; ", ", "):
        if sep in text:
            head = text.split(sep, 1)[0]
            if len(head) >= 12:
                return head
    return text[:140]


def _lead_name(rng: random.Random) -> str:
    firsts = ["Marisol", "Idris", "Nnenna", "Bassey", "Vale", "Toma", "Wren", "Sable", "Ari", "Del", "Halden", "Okafor"]
    return rng.choice(firsts)


# ------------------------------------------------------------------- synthesis
def _espeak(text: str, voice: str, out_path: str) -> None:
    clean = re.sub(r"\s+", " ", text).strip()[:1800]
    subprocess.run(
        ["espeak-ng", "-v", voice, "-s", str(ESPEAK_WPM), "-w", out_path, clean],
        check=True,
        capture_output=True,
    )


def _silence(path: str, seconds: float, rate: int, channels: int, width: int) -> None:
    with wave.open(path, "wb") as w:
        w.setnchannels(channels)
        w.setsampwidth(width)
        w.setframerate(rate)
        w.writeframes(b"\x00" * int(seconds * rate) * channels * width)


def _concat(parts: list[str], out_path: str) -> None:
    with wave.open(parts[0], "rb") as first:
        params = first.getparams()
    with wave.open(out_path, "wb") as out:
        out.setparams(params)
        for p in parts:
            with wave.open(p, "rb") as seg:
                if seg.getframerate() != params.framerate or seg.getnchannels() != params.nchannels:
                    continue
                out.writeframes(seg.readframes(seg.getnframes()))


def synthesize(lines: list[tuple[str, str]], work: str) -> str:
    seg_dir = os.path.join(work, "seg")
    os.makedirs(seg_dir, exist_ok=True)
    parts: list[str] = []
    rate = channels = width = None
    for i, (vk, text) in enumerate(lines):
        seg = os.path.join(seg_dir, f"{i:04d}.wav")
        _espeak(text, VOICES.get(vk, "en-us"), seg)
        with wave.open(seg, "rb") as w:
            rate, channels, width = w.getframerate(), w.getnchannels(), w.getsampwidth()
        parts.append(seg)
        pause = os.path.join(seg_dir, f"{i:04d}_p.wav")
        _silence(pause, 0.45 if vk == "narrator" else 0.3, rate, channels, width)
        parts.append(pause)
    combined = os.path.join(work, "voice.wav")
    _concat(parts, combined)
    return combined


# ------------------------------------------------------------------- packaging
def _ffmpeg(args: list[str]) -> None:
    subprocess.run(["ffmpeg", "-hide_banner", "-loglevel", "error", "-y", *args], check=True, capture_output=True)


def _ffprobe(path: str) -> dict:
    out = subprocess.run(
        ["ffprobe", "-hide_banner", "-loglevel", "error", "-print_format", "json", "-show_format", "-show_streams", path],
        check=True,
        capture_output=True,
    )
    return json.loads(out.stdout)


def package(wav_path: str, work: str) -> dict:
    hls = os.path.join(work, "hls")
    if os.path.isdir(hls):
        shutil.rmtree(hls)
    os.makedirs(hls)
    master = ["#EXTM3U", "#EXT-X-VERSION:3"]
    variants = []
    for kbps in BITRATES:
        name = f"audio_{kbps}"
        _ffmpeg(
            [
                "-i", wav_path, "-c:a", "aac", "-b:a", f"{kbps}k", "-ar", "44100", "-ac", "2",
                "-f", "hls", "-hls_time", str(HLS_SEGMENT_SECONDS), "-hls_playlist_type", "vod",
                "-hls_segment_filename", os.path.join(hls, f"{name}_%03d.ts"),
                os.path.join(hls, f"{name}.m3u8"),
            ]
        )
        seg_bytes = sum(
            os.path.getsize(os.path.join(hls, f))
            for f in os.listdir(hls)
            if f.startswith(f"{name}_") and f.endswith(".ts")
        )
        variants.append({"bitrate_kbps": kbps, "playlist": f"{name}.m3u8", "codec": "aac", "size_bytes": seg_bytes})
        master.append(f'#EXT-X-STREAM-INF:BANDWIDTH={kbps * 1000},CODECS="mp4a.40.2"')
        master.append(f"{name}.m3u8")
    with open(os.path.join(hls, "master.m3u8"), "w") as f:
        f.write("\n".join(master) + "\n")

    top = _ffprobe(os.path.join(hls, "audio_256.m3u8"))
    stream = next((s for s in top.get("streams", []) if s.get("codec_type") == "audio"), {})
    total_ts = sum(os.path.getsize(os.path.join(hls, f)) for f in os.listdir(hls) if f.endswith(".ts"))
    h = hashlib.sha256()
    with open(wav_path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return {
        "root": hls,
        "variants": variants,
        "duration_sec": round(float(top.get("format", {}).get("duration", 0)) or 0),
        "sample_rate_hz": int(stream.get("sample_rate", 44100)),
        "channels": int(stream.get("channels", 2)),
        "codec": stream.get("codec_name", "aac"),
        "file_size_bytes": total_ts,
        "checksum_sha256": h.hexdigest(),
    }


# --------------------------------------------------------------------- catalog
def fetch_catalog(client: httpx.Client) -> list[dict]:
    shows: list[dict] = []
    offset = 0
    while True:
        page = client.get("/api/catalog/shows", params={"limit": 100, "offset": offset}).json()
        batch = page.get("shows", [])
        shows.extend(batch)
        if len(batch) < 100:
            break
        offset += 100
    out = []
    for s in shows:
        detail = client.get(f"/api/catalog/shows/{s['slug']}").json()
        merged = {**s, **(detail.get("show") or {})}
        merged["episodes"] = detail.get("episodes", [])
        out.append(merged)
    return out


def object_exists(mc: Minio, bucket: str, key: str) -> bool:
    try:
        mc.stat_object(bucket, key)
        return True
    except S3Error:
        return False


def upload_hls(mc: Minio, bucket: str, root: str, key_prefix: str) -> int:
    total = 0
    for name in sorted(os.listdir(root)):
        full = os.path.join(root, name)
        ct = (
            "application/vnd.apple.mpegurl"
            if name.endswith(".m3u8")
            else "video/mp2t"
            if name.endswith(".ts")
            else "application/octet-stream"
        )
        mc.fput_object(bucket, f"{key_prefix}/{name}", full, content_type=ct)
        total += os.path.getsize(full)
    return total


# ------------------------------------------------------------------------ main
def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--gateway", default="")
    ap.add_argument("--content-url", default="https://auralis-content.onrender.com")
    ap.add_argument("--target-gb", type=float, default=2.8)
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("--only-show", action="append", default=[])
    ap.add_argument("--min-sec", type=int, default=90)
    ap.add_argument("--max-sec", type=int, default=420)
    ap.add_argument("--force", action="store_true", help="regenerate episodes that already have audio")
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    require_tools()
    d = load_env()
    gateway = (args.gateway or d.get("NEXT_PUBLIC_API_BASE", "").removesuffix("/api") or "https://auralis-gateway.onrender.com").rstrip("/")
    token = d.get("SERVICE_SHARED_TOKEN") or sys.exit("SERVICE_SHARED_TOKEN missing")
    bucket = d.get("S3_BUCKET", "auralis-media")
    mc = Minio(
        d["S3_ENDPOINT"],
        access_key=d["S3_ACCESS_KEY"],
        secret_key=d["S3_SECRET_KEY"],
        secure=d.get("S3_USE_SSL", "true").lower() in {"1", "true", "yes"},
        region=d.get("S3_REGION", "auto"),
    )

    cat_client = httpx.Client(base_url=gateway, timeout=60.0)
    content_client = httpx.Client(
        base_url=args.content_url.rstrip("/"), timeout=60.0, headers={"X-Auralis-Service-Token": token}
    )

    shows = fetch_catalog(cat_client)
    if args.only_show:
        want = set(args.only_show)
        shows = [s for s in shows if s["slug"] in want or s.get("title") in want]

    episodes = [(s, e) for s in shows for e in s.get("episodes", [])]
    total_eps = len(episodes)
    if not total_eps:
        sys.exit("no published episodes found")

    budget_bytes = int(args.target_gb * 1024**3)
    per_ep_sec = int(budget_bytes / total_eps / (TOTAL_KBPS * 1000 / 8))
    per_ep_sec = max(args.min_sec, min(args.max_sec, per_ep_sec))
    print(
        f"catalog: {len(shows)} shows, {total_eps} episodes | "
        f"budget {args.target_gb:.2f} GB -> ~{per_ep_sec}s/episode",
        flush=True,
    )

    done = skipped = 0
    used_bytes = 0
    for i, (show, ep) in enumerate(episodes, 1):
        if args.limit and done >= args.limit:
            break
        key_prefix = f"hls/{show['id']}/{ep['id']}"
        master_key = f"{key_prefix}/master.m3u8"
        if not args.force and object_exists(mc, bucket, master_key):
            skipped += 1
            continue
        if used_bytes >= budget_bytes:
            print(f"reached byte budget ({used_bytes/1024**3:.2f} GB); stopping", flush=True)
            break

        label = f"[{i}/{total_eps}] {show.get('title','?')} / {ep.get('title','?')}"
        if args.dry_run:
            print(f"  would generate {label} (~{per_ep_sec}s)", flush=True)
            done += 1
            continue

        work = tempfile.mkdtemp(prefix="auralis-backfill-")
        try:
            seed = int(hashlib.sha256(ep["id"].encode()).hexdigest()[:12], 16)
            lines = compose_narration(show, ep, per_ep_sec, seed)
            voice_wav = synthesize(lines, work)
            pkg = package(voice_wav, work)
            uploaded = upload_hls(mc, bucket, pkg["root"], key_prefix)
            used_bytes += uploaded

            media = {
                "hls_master_key": master_key,
                "variants": [
                    {"bitrate_kbps": v["bitrate_kbps"], "key": f"{key_prefix}/{v['playlist']}", "codec": v["codec"], "size_bytes": v["size_bytes"]}
                    for v in pkg["variants"]
                ],
                "codec": pkg["codec"],
                "sample_rate_hz": pkg["sample_rate_hz"],
                "channels": pkg["channels"],
                "file_size_bytes": pkg["file_size_bytes"],
                "checksum_sha256": pkg["checksum_sha256"],
                "duration_sec": pkg["duration_sec"],
            }
            script_text = "\n".join(f"{'Narrator' if vk=='narrator' else 'Voice'}: {t}" for vk, t in lines)
            resp = content_client.patch(
                f"/internal/authoring/episodes/{ep['id']}",
                json={"media": media, "script": script_text, "processing": "ready"},
            )
            resp.raise_for_status()
            done += 1
            print(
                f"  ok {label}  {pkg['duration_sec']}s  {uploaded/1024/1024:.1f} MB  "
                f"(total {used_bytes/1024**3:.2f} GB)",
                flush=True,
            )
        except subprocess.CalledProcessError as e:
            print(f"  FAIL {label}: {e.stderr.decode()[:300] if e.stderr else e}", flush=True)
        except httpx.HTTPError as e:
            print(f"  FAIL {label}: attach failed: {e}", flush=True)
        finally:
            shutil.rmtree(work, ignore_errors=True)

    print(f"\ndone: {done} generated, {skipped} already had audio, {used_bytes/1024**3:.2f} GB uploaded", flush=True)


if __name__ == "__main__":
    with contextlib.suppress(KeyboardInterrupt):
        main()

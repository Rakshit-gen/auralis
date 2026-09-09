#!/usr/bin/env python3
"""Generate a real cover image for every published show and attach it.

The frontend fakes cover art with a per-show gradient (coverStyle in
frontend/src/lib/format.ts). This script gives the seeded catalogue actual
artwork: for each published show it builds a prompt from the show's title,
synopsis and genres, renders a portrait with a local Stable Diffusion XL model
(SDXL + the SDXL-Lightning 4-step LoRA, on Apple's MPS backend), crops it to
3:4, encodes a small WebP, uploads it to the media bucket under
covers/shows/<show_id>.webp, and PATCHes the show with the public URL plus a
dominant accent colour pulled from the image.

It is idempotent: shows that already have cover_image_url set are skipped
unless --force, and an already-uploaded WebP is reused rather than regenerated.

    .imggen/bin/python scripts/deploy/cover-backfill.py --dry-run
    .imggen/bin/python scripts/deploy/cover-backfill.py --limit 3
    .imggen/bin/python scripts/deploy/cover-backfill.py --only-show the-long-room
    .imggen/bin/python scripts/deploy/cover-backfill.py --self-test   # no model

Run scripts/img-setup.sh once first. Reads S3 and gateway settings from
.env.deploy (repo root); model weights are cached under ~/.cache/huggingface.

Machine load: generation is bursty (a few seconds of GPU per image at 4 steps),
never sustained. --sleep adds a gap between shows; --limit caps a run.
"""

from __future__ import annotations

import argparse
import io
import os
import sys
import time
from pathlib import Path

from PIL import Image

# httpx and minio are imported inside run()/fetch_catalog() so --self-test only
# needs Pillow.

ROOT = Path(__file__).resolve().parents[2]
ENV_DEPLOY = ROOT / ".env.deploy"

# SDXL base + ByteDance's 4-step Lightning LoRA: near-SDXL quality at 4 steps
# with classifier-free guidance off, which is what keeps this feasible on a
# 16 GB machine.
BASE_MODEL = "stabilityai/stable-diffusion-xl-base-1.0"
LIGHTNING_REPO = "ByteDance/SDXL-Lightning"
LIGHTNING_LORA = "sdxl_lightning_4step_lora.safetensors"

GEN_W, GEN_H = 832, 1216  # SDXL-friendly portrait, cropped to 3:4 after
OUT_W, OUT_H = 768, 1024  # stored cover, 3:4
WEBP_QUALITY = 82

STYLE = (
    "book cover illustration, dramatic cinematic lighting, moody atmosphere, "
    "painterly, textured, no text, no lettering, no watermark"
)
NEGATIVE = (
    "text, title, typography, letters, words, watermark, signature, logo, "
    "frame, border, low quality, blurry, deformed, extra limbs"
)


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


# ----------------------------------------------------------------------- prompt
def build_prompt(show: dict) -> str:
    """A deterministic image prompt from the show's own metadata."""
    title = (show.get("title") or "an untitled series").strip()
    synopsis = " ".join((show.get("synopsis") or "").split())
    if len(synopsis) > 240:
        synopsis = synopsis[:240].rsplit(" ", 1)[0] + "..."
    genres = [g.get("name", "") for g in (show.get("genres") or []) if g.get("name")]
    tags = [t for t in (show.get("tags") or []) if isinstance(t, str)][:3]
    mood = ", ".join(genres + tags) or "drama"

    parts = [f'"{title}", {mood}']
    if synopsis:
        parts.append(synopsis)
    parts.append(STYLE)
    return ". ".join(parts)


def seed_for(show_id: str) -> int:
    return int.from_bytes(show_id.encode()[:8].ljust(8, b"\0"), "big") % (2**31)


# ------------------------------------------------------------------ image maths
def crop_to_cover(img: Image.Image) -> Image.Image:
    """Centre-crop to 3:4 then resize to the stored cover size."""
    target = OUT_W / OUT_H
    w, h = img.size
    if w / h > target:
        new_w = round(h * target)
        left = (w - new_w) // 2
        img = img.crop((left, 0, left + new_w, h))
    else:
        new_h = round(w / target)
        top = (h - new_h) // 2
        img = img.crop((0, top, w, top + new_h))
    return img.resize((OUT_W, OUT_H), Image.LANCZOS)


def to_webp(img: Image.Image) -> bytes:
    buf = io.BytesIO()
    img.convert("RGB").save(buf, format="WEBP", quality=WEBP_QUALITY, method=6)
    return buf.getvalue()


def accent_color(img: Image.Image) -> str:
    """Dominant vivid colour of the image as #rrggbb, for the show's accent."""
    small = img.convert("RGB").resize((64, 64), Image.BILINEAR)
    quant = small.quantize(colors=8, method=Image.MEDIANCUT).convert("RGB")
    colors = quant.getcolors(maxcolors=64) or [(1, (128, 128, 128))]

    def score(item: tuple[int, tuple[int, int, int]]) -> float:
        n, (r, g, b) = item
        mx, mn = max(r, g, b), min(r, g, b)
        sat = (mx - mn) / mx if mx else 0.0
        bright = mx / 255
        # favour colours that are both reasonably saturated and mid-bright,
        # not near-black shadow or blown-out highlight
        return n * (0.3 + sat) * (0.4 + min(bright, 1 - bright) * 2)

    _, (r, g, b) = max(colors, key=score)
    return f"#{r:02x}{g:02x}{b:02x}"


# ---------------------------------------------------------------------- catalog
def fetch_catalog(client) -> list[dict]:
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
        out.append({**s, **(detail.get("show") or {})})
    return out


def object_exists(mc, bucket: str, key: str) -> bool:
    from minio.error import S3Error

    try:
        mc.stat_object(bucket, key)
        return True
    except S3Error:
        return False


# ------------------------------------------------------------------- generation
class Renderer:
    """Lazily loads SDXL + the Lightning LoRA on first use."""

    def __init__(self, steps: int, size: tuple[int, int]) -> None:
        self.steps = steps
        self.size = size
        self.pipe = None

    def _load(self) -> None:
        import torch
        from diffusers import DiffusionPipeline, EulerDiscreteScheduler
        from huggingface_hub import hf_hub_download

        print("loading SDXL + SDXL-Lightning (first run downloads ~7 GB)", flush=True)
        pipe = DiffusionPipeline.from_pretrained(
            BASE_MODEL, torch_dtype=torch.float16, variant="fp16", use_safetensors=True
        )
        pipe.load_lora_weights(hf_hub_download(LIGHTNING_REPO, LIGHTNING_LORA))
        pipe.fuse_lora()
        pipe.scheduler = EulerDiscreteScheduler.from_config(
            pipe.scheduler.config, timestep_spacing="trailing"
        )
        pipe.to("mps")
        pipe.enable_attention_slicing()
        pipe.enable_vae_slicing()
        pipe.set_progress_bar_config(disable=True)
        self.pipe = pipe

    def render(self, prompt: str, seed: int) -> Image.Image:
        import torch

        if self.pipe is None:
            self._load()
        w, h = self.size
        gen = torch.Generator(device="mps").manual_seed(seed)
        image = self.pipe(
            prompt=prompt,
            negative_prompt=NEGATIVE,
            width=w,
            height=h,
            num_inference_steps=self.steps,
            guidance_scale=0.0,
            generator=gen,
        ).images[0]
        return image


# ------------------------------------------------------------------------ main
def run(args: argparse.Namespace) -> None:
    import httpx
    from minio import Minio
    from minio.error import S3Error

    d = load_env()
    gateway = (
        args.gateway
        or d.get("NEXT_PUBLIC_API_BASE", "").removesuffix("/api")
        or "https://auralis-gateway.onrender.com"
    ).rstrip("/")
    token = d.get("SERVICE_SHARED_TOKEN") or sys.exit("SERVICE_SHARED_TOKEN missing")
    public_base = d.get("S3_PUBLIC_BASE_URL", "").rstrip("/")
    if not public_base:
        sys.exit("S3_PUBLIC_BASE_URL missing; covers need a public media URL")
    bucket = d.get("S3_BUCKET", "auralis-media")
    mc = Minio(
        d["S3_ENDPOINT"],
        access_key=d["S3_ACCESS_KEY"],
        secret_key=d["S3_SECRET_KEY"],
        secure=d.get("S3_USE_SSL", "true").lower() in {"1", "true", "yes"},
        region=d.get("S3_REGION", "auto"),
    )

    cat = httpx.Client(base_url=gateway, timeout=60.0)
    content = httpx.Client(
        base_url=args.content_url.rstrip("/"),
        timeout=60.0,
        headers={"X-Auralis-Service-Token": token},
    )

    shows = fetch_catalog(cat)
    if args.only_show:
        want = set(args.only_show)
        shows = [s for s in shows if s["slug"] in want or s.get("title") in want]
    if not shows:
        sys.exit("no shows matched")

    renderer = Renderer(args.steps, (args.width, args.height))
    done = skipped = failed = 0
    print(f"catalog: {len(shows)} shows", flush=True)

    for i, show in enumerate(shows, 1):
        if args.limit and done >= args.limit:
            break
        sid = show["id"]
        label = f"[{i}/{len(shows)}] {show.get('title', '?')}"
        if show.get("cover_image_url") and not args.force:
            skipped += 1
            continue

        key = f"covers/shows/{sid}.webp"
        url = f"{public_base}/{key}"
        prompt = build_prompt(show)

        if args.dry_run:
            print(f"  would render {label}\n    {prompt}", flush=True)
            done += 1
            continue

        try:
            if object_exists(mc, bucket, key) and not args.force:
                data = mc.get_object(bucket, key).read()
                img = Image.open(io.BytesIO(data))
                reused = True
            else:
                raw = renderer.render(prompt, seed_for(sid))
                img = crop_to_cover(raw)
                data = to_webp(img)
                mc.put_object(
                    bucket, key, io.BytesIO(data), length=len(data), content_type="image/webp"
                )
                reused = False

            resp = content.patch(
                f"/internal/authoring/shows/{sid}",
                json={"cover_image_url": url, "accent_color": accent_color(img)},
            )
            resp.raise_for_status()
            done += 1
            print(
                f"  ok {label}  {len(data) / 1024:.0f} KB{'  (reused)' if reused else ''}",
                flush=True,
            )
        except (httpx.HTTPError, S3Error, OSError) as e:
            failed += 1
            print(f"  FAIL {label}: {e}", flush=True)

        if args.sleep and not args.dry_run:
            time.sleep(args.sleep)

    print(f"\ndone: {done} attached, {skipped} already had a cover, {failed} failed", flush=True)


# ------------------------------------------------------------------- self-test
def self_test() -> None:
    show = {
        "id": "show_abc123",
        "title": "The Long Room",
        "synopsis": "  A night-shift archivist   maps a building that keeps   growing new corridors.  " * 4,
        "genres": [{"name": "Mystery"}, {"name": "Horror"}],
        "tags": ["slow burn", "atmospheric", "unreliable narrator"],
    }
    p = build_prompt(show)
    assert p.startswith('"The Long Room", Mystery, Horror, slow burn'), p
    assert "..." in p and "no text" in p, p
    assert seed_for(show["id"]) == seed_for(show["id"]) and 0 <= seed_for(show["id"]) < 2**31

    # 1024x1024 -> centre-cropped and resized to the 3:4 stored size
    src = Image.new("RGB", (1024, 1024))
    for x in range(1024):
        for y in range(0, 1024, 8):
            src.putpixel((x, y), (200, 60, 40))
    cover = crop_to_cover(src)
    assert cover.size == (OUT_W, OUT_H), cover.size

    wide = crop_to_cover(Image.new("RGB", (2000, 500)))
    assert wide.size == (OUT_W, OUT_H), wide.size

    blob = to_webp(cover)
    assert blob[:4] == b"RIFF" and blob[8:12] == b"WEBP", blob[:12]
    assert Image.open(io.BytesIO(blob)).size == (OUT_W, OUT_H)

    swatch = Image.new("RGB", (32, 32), (12, 12, 12))
    for x in range(32):
        for y in range(20):
            swatch.putpixel((x, y), (30, 120, 200))
    hexc = accent_color(swatch)
    assert hexc.startswith("#") and len(hexc) == 7, hexc
    r, g, b = (int(hexc[j : j + 2], 16) for j in (1, 3, 5))
    assert b > r and b > 80, hexc  # picked the blue block, not the near-black

    print("self-test ok")


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--gateway", default="")
    ap.add_argument("--content-url", default="https://auralis-content.onrender.com")
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("--only-show", action="append", default=[])
    ap.add_argument("--steps", type=int, default=4)
    ap.add_argument("--width", type=int, default=GEN_W)
    ap.add_argument("--height", type=int, default=GEN_H)
    ap.add_argument("--sleep", type=float, default=2.0, help="pause between shows, seconds")
    ap.add_argument("--force", action="store_true", help="regenerate shows that already have a cover")
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--self-test", action="store_true", help="exercise prompt + image maths, no model")
    args = ap.parse_args()

    if args.self_test:
        self_test()
        return
    run(args)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        pass

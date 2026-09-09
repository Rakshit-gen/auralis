#!/usr/bin/env bash
# Set up a local Stable Diffusion stack for scripts/deploy/cover-backfill.py:
# a Python 3.11 venv at .imggen (gitignored) with diffusers + torch (MPS on
# Apple silicon) and the R2 upload deps. The cover backfill defaults to
# .imggen/bin/python.
#
# Re-run any time; every step is idempotent. The first run installs ~3 GB of
# wheels. Model weights (~7 GB) are pulled on the first backfill run and cached
# under ~/.cache/huggingface.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VENV="${IMGGEN_VENV:-$ROOT/.imggen}"
PY="${IMGGEN_PYTHON:-/opt/homebrew/bin/python3.11}"

if [ ! -x "$PY" ]; then
  PY="$(command -v python3.11 || true)"
fi
if [ -z "$PY" ] || [ ! -x "$PY" ]; then
  echo "need python3.11 on PATH (brew install python@3.11) or set IMGGEN_PYTHON" >&2
  exit 1
fi

if [ ! -x "$VENV/bin/python" ]; then
  echo "creating venv at $VENV"
  "$PY" -m venv "$VENV"
fi

pip="$VENV/bin/pip"
"$pip" install --quiet --upgrade pip

if ! "$VENV/bin/python" -c "import torch, diffusers, PIL, minio, httpx" 2>/dev/null; then
  echo "installing torch + diffusers + upload deps (first run downloads ~3 GB)"
  "$pip" install --quiet \
    torch \
    "diffusers>=0.31" transformers accelerate peft safetensors huggingface_hub \
    pillow minio httpx
fi

echo
echo "done. run the cover backfill with:"
echo "  $VENV/bin/python scripts/deploy/cover-backfill.py --dry-run"

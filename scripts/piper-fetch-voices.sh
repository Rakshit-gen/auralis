#!/usr/bin/env bash
# Download the Piper voice models the ai-media TTS pipeline expects into a
# target directory. Platform independent (the models are just ONNX files), so
# the Dockerfile and the local dev setup both call this.
#
#   scripts/piper-fetch-voices.sh /opt/piper/voices
#
# Idempotent: a model whose .onnx and .onnx.json are already present and
# non-empty is skipped.
#
# The English set is required; a failure there aborts. The other languages are
# best effort: a 404 on one of them is logged and skipped so an English-only
# box still comes up. The abstract voice keys these map to live in
# services/ai_media/auralis_ai_media/providers/tts.py (_PIPER_VOICES_BY_LANG).
set -euo pipefail

DEST="${1:-.piper/voices}"
BASE="https://huggingface.co/rhasspy/piper-voices/resolve/main"

# "<lang>/<locale>/<name>"
REQUIRED_VOICES=(
  "en/en_US/lessac"
  "en/en_US/ryan"
  "en/en_US/amy"
  "en/en_GB/alan"
  "en/en_US/hfc_male"
  "en/en_US/hfc_female"
)

# Non-English voices. Missing files here are tolerated.
OPTIONAL_VOICES=(
  "hi/hi_IN/pratham"
  "hi/hi_IN/priyamvada"
  "es/es_ES/davefx"
  "es/es_ES/sharvard"
)

mkdir -p "$DEST"

fetch() {
  local url="$1" out="$2" required="$3"
  if [ -s "$out" ]; then
    echo "  have $(basename "$out")"
    return 0
  fi
  echo "  get  $(basename "$out")"
  if curl -fsSL --retry 3 -o "$out" "$url"; then
    return 0
  fi
  rm -f "$out"
  if [ "$required" = "required" ]; then
    echo "  ERROR: could not fetch $url" >&2
    return 1
  fi
  echo "  skip (unavailable): $url" >&2
  return 0
}

fetch_model() {
  local spec="$1" required="$2"
  local lang="${spec%%/*}" rest="${spec#*/}"
  local locale="${rest%%/*}" name="${rest##*/}"
  local model="${locale}-${name}-medium"
  local dir="$BASE/$lang/$locale/$name/medium"
  echo "$model"
  fetch "$dir/$model.onnx" "$DEST/$model.onnx" "$required" || return 1
  # Only chase the config if the model landed.
  if [ -s "$DEST/$model.onnx" ]; then
    fetch "$dir/$model.onnx.json" "$DEST/$model.onnx.json" "$required" || return 1
  fi
}

for v in "${REQUIRED_VOICES[@]}"; do
  fetch_model "$v" required
done

for v in "${OPTIONAL_VOICES[@]}"; do
  fetch_model "$v" optional || true
done

echo "voices in $DEST:"
ls -1 "$DEST"

#!/usr/bin/env bash
# Download the six Piper voice models the ai-media TTS pipeline expects into a
# target directory. Platform independent (the models are just ONNX files), so
# the Dockerfile and the local dev setup both call this.
#
#   scripts/piper-fetch-voices.sh /opt/piper/voices
#
# Idempotent: a model whose .onnx and .onnx.json are already present and
# non-empty is skipped.
set -euo pipefail

DEST="${1:-.piper/voices}"
BASE="https://huggingface.co/rhasspy/piper-voices/resolve/main/en"

# key -> "<locale>/<name>". The abstract voice keys live in
# services/ai_media/auralis_ai_media/providers/tts.py (_PIPER_VOICES).
VOICES=(
  "en_US/lessac"
  "en_US/ryan"
  "en_US/amy"
  "en_GB/alan"
  "en_US/hfc_male"
  "en_US/hfc_female"
)

mkdir -p "$DEST"

fetch() {
  local url="$1" out="$2"
  if [ -s "$out" ]; then
    echo "  have $(basename "$out")"
    return
  fi
  echo "  get  $(basename "$out")"
  curl -fsSL --retry 3 -o "$out" "$url"
}

for v in "${VOICES[@]}"; do
  locale="${v%%/*}"
  name="${v##*/}"
  model="${locale}-${name}-medium"
  echo "$model"
  fetch "$BASE/$locale/$name/medium/$model.onnx" "$DEST/$model.onnx"
  fetch "$BASE/$locale/$name/medium/$model.onnx.json" "$DEST/$model.onnx.json"
done

echo "voices in $DEST:"
ls -1 "$DEST"

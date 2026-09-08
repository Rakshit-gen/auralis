#!/usr/bin/env bash
# Set up Piper for local development and the media backfill: put a working piper
# binary at .piper/bin/piper and the voice models in .piper/voices (both
# gitignored). The ai-media worker, scripts/dev-native.sh, and
# scripts/deploy/media-backfill.py all default to those paths:
#
#   PIPER_BIN=.piper/bin/piper  PIPER_VOICES_DIR=.piper/voices
#
# Re-run any time; every step is idempotent.
#
# Linux uses the prebuilt release binary (its shared libs are bundled). macOS
# uses the piper-tts PyPI package instead, because the 2023.11.14 macOS release
# archive ships without its dylibs; .piper/bin/piper becomes a symlink to the
# venv entry point.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PIPER_DIR="${PIPER_DIR:-$ROOT/.piper}"
VENV="${VENV:-$ROOT/.venv}"
RELEASE="2023.11.14-2"

mkdir -p "$PIPER_DIR/bin"

os="$(uname -s)"
arch="$(uname -m)"

install_binary_linux() {
  case "$arch" in
    x86_64)  asset="piper_linux_x86_64.tar.gz" ;;
    aarch64) asset="piper_linux_aarch64.tar.gz" ;;
    armv7l)  asset="piper_linux_armv7l.tar.gz" ;;
    *) echo "no prebuilt piper for linux/$arch; set PIPER_BIN manually" >&2; exit 1 ;;
  esac
  if [ -x "$PIPER_DIR/bin/piper" ] && [ ! -L "$PIPER_DIR/bin/piper" ]; then
    echo "piper binary already at $PIPER_DIR/bin/piper"
    return
  fi
  echo "downloading $asset"
  tmp="$(mktemp -d)"
  curl -fsSL --retry 3 -o "$tmp/piper.tar.gz" \
    "https://github.com/rhasspy/piper/releases/download/$RELEASE/$asset"
  tar -xzf "$tmp/piper.tar.gz" -C "$PIPER_DIR/bin" --strip-components=1
  chmod +x "$PIPER_DIR/bin/piper"
  rm -rf "$tmp"
  echo "piper binary at $PIPER_DIR/bin/piper"
}

install_piper_tts_pip() {
  local pip="$VENV/bin/pip"
  if [ ! -x "$pip" ]; then
    echo "no venv at $VENV; create it first (python -m venv .venv)" >&2
    exit 1
  fi
  if ! "$VENV/bin/python" -c "import piper" 2>/dev/null; then
    echo "installing piper-tts into $VENV"
    "$pip" install --quiet piper-tts
  fi
  ln -sf "$VENV/bin/piper" "$PIPER_DIR/bin/piper"
  echo "piper (piper-tts) linked at $PIPER_DIR/bin/piper"
}

case "$os" in
  Linux)  install_binary_linux ;;
  Darwin) install_piper_tts_pip ;;
  *) echo "unsupported OS: $os; install piper manually and set PIPER_BIN" >&2; exit 1 ;;
esac

"$ROOT/scripts/piper-fetch-voices.sh" "$PIPER_DIR/voices"

echo
echo "done. the worker and the backfill script pick these up by default:"
echo "  PIPER_BIN=$PIPER_DIR/bin/piper"
echo "  PIPER_VOICES_DIR=$PIPER_DIR/voices"

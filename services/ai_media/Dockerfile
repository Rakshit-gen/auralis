# ai-media needs ffmpeg and the Piper TTS runtime, so it ships on a slim Python
# base rather than distroless. espeak-ng stays as the fallback voice.

# --- fetch the Piper binary and voice models in a throwaway stage -----------
FROM debian:bookworm-slim AS piper
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates tar \
    && rm -rf /var/lib/apt/lists/*
ARG PIPER_RELEASE=2023.11.14-2
RUN curl -fsSL --retry 3 -o /tmp/piper.tar.gz \
      "https://github.com/rhasspy/piper/releases/download/${PIPER_RELEASE}/piper_linux_x86_64.tar.gz" \
    && mkdir -p /opt/piper/bin \
    && tar -xzf /tmp/piper.tar.gz -C /opt/piper/bin --strip-components=1 \
    && rm /tmp/piper.tar.gz
COPY scripts/piper-fetch-voices.sh /tmp/piper-fetch-voices.sh
RUN bash /tmp/piper-fetch-voices.sh /opt/piper/voices

# --- runtime --------------------------------------------------------------
FROM python:3.13-slim AS base
ENV PYTHONUNBUFFERED=1 PIP_NO_CACHE_DIR=1
RUN apt-get update \
    && apt-get install -y --no-install-recommends ffmpeg espeak-ng ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=piper /opt/piper /opt/piper
ENV PIPER_BIN=/opt/piper/bin/piper \
    PIPER_VOICES_DIR=/opt/piper/voices \
    LD_LIBRARY_PATH=/opt/piper/bin

WORKDIR /app
COPY libs/py-common /app/libs/py-common
COPY services/ai_media /app/services/ai_media
RUN pip install ./libs/py-common ./services/ai_media

WORKDIR /app/services/ai_media
EXPOSE 8085
# Default to the API; the worker runs as a second command in compose/k8s.
ENTRYPOINT ["python", "-m", "auralis_ai_media"]

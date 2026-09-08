# ai-media needs ffmpeg and espeak-ng at runtime, so it ships on a slim Python
# base rather than distroless.
FROM python:3.13-slim AS base
ENV PYTHONUNBUFFERED=1 PIP_NO_CACHE_DIR=1
RUN apt-get update \
    && apt-get install -y --no-install-recommends ffmpeg espeak-ng ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY libs/py-common /app/libs/py-common
COPY services/ai_media /app/services/ai_media
RUN pip install ./libs/py-common ./services/ai_media

WORKDIR /app/services/ai_media
EXPOSE 8085
# Default to the API; the worker runs as a second command in compose/k8s.
ENTRYPOINT ["python", "-m", "auralis_ai_media"]

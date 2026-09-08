# Recommendation service: FastAPI API + Kafka consumer in one process.
FROM python:3.13-slim AS base
ENV PYTHONUNBUFFERED=1 PIP_NO_CACHE_DIR=1
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY libs/py-common /app/libs/py-common
COPY services/recommendation /app/services/recommendation
RUN pip install ./libs/py-common ./services/recommendation

WORKDIR /app/services/recommendation
EXPOSE 8086
ENTRYPOINT ["python", "-m", "auralis_reco"]

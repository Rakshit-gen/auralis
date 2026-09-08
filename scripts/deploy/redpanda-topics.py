#!/usr/bin/env python3
"""Pre-create the Auralis Kafka topics on a managed broker.

Redpanda Cloud (and most managed Kafka) disable auto topic creation, so the
five event topics and their `.dlq` counterparts must exist before any service
starts. Idempotent: topics that already exist are left alone.

Reads KAFKA_* from .env.deploy (repo root) or the environment:

    KAFKA_BROKERS=host:9092
    KAFKA_SASL_MECHANISM=SCRAM-SHA-256
    KAFKA_SASL_USERNAME=...
    KAFKA_SASL_PASSWORD=...
    KAFKA_TLS_ENABLED=true

    python scripts/deploy/redpanda-topics.py            # create missing topics
    python scripts/deploy/redpanda-topics.py --list     # just list what exists
"""

from __future__ import annotations

import argparse
import asyncio
import ssl
import sys
from pathlib import Path

from aiokafka.admin import AIOKafkaAdminClient, NewTopic
from aiokafka.helpers import create_ssl_context

ROOT = Path(__file__).resolve().parents[2]
ENV_DEPLOY = ROOT / ".env.deploy"

# from docs/KAFKA.md; partitions match the platform default (3)
EVENT_TOPICS = [
    "auralis.user.events",
    "auralis.content.events",
    "auralis.playback.events",
    "auralis.ai.events",
    "auralis.media.events",
]
PARTITIONS = 3


def load_env() -> dict[str, str]:
    import os

    out = dict(os.environ)
    if ENV_DEPLOY.exists():
        for line in ENV_DEPLOY.read_text().splitlines():
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                k, _, v = line.partition("=")
                out[k.strip()] = v.strip()  # file is the source of truth for the deploy
    return out


def client_kwargs(d: dict[str, str]) -> dict:
    brokers = d.get("KAFKA_BROKERS")
    if not brokers:
        sys.exit("KAFKA_BROKERS not set")
    kw: dict = {"bootstrap_servers": brokers}
    mechanism = d.get("KAFKA_SASL_MECHANISM", "").strip().upper()
    tls = d.get("KAFKA_TLS_ENABLED", "").strip().lower() in {"1", "true", "yes", "on"} or bool(mechanism)
    ssl_context: ssl.SSLContext | None = None
    if tls:
        ssl_context = create_ssl_context()
    if mechanism:
        user = d.get("KAFKA_SASL_USERNAME", "")
        pw = d.get("KAFKA_SASL_PASSWORD", "")
        if not user or not pw:
            sys.exit("KAFKA_SASL_MECHANISM set but username/password missing")
        kw.update(
            security_protocol="SASL_SSL" if tls else "SASL_PLAINTEXT",
            sasl_mechanism=mechanism,
            sasl_plain_username=user,
            sasl_plain_password=pw,
            ssl_context=ssl_context,
        )
    elif tls:
        kw.update(security_protocol="SSL", ssl_context=ssl_context)
    return kw


async def run(list_only: bool) -> None:
    d = load_env()
    admin = AIOKafkaAdminClient(**client_kwargs(d))
    await admin.start()
    try:
        existing = set(await admin.list_topics())
        if list_only:
            for t in sorted(existing):
                print(t)
            return

        wanted = []
        for base in EVENT_TOPICS:
            wanted.append(base)
            wanted.append(base + ".dlq")

        missing = [t for t in wanted if t not in existing]
        if not missing:
            print(f"all {len(wanted)} topics already exist")
            return

        # Redpanda Cloud fixes replication at 3; a single-node broker needs 1.
        last_err: Exception | None = None
        for rf in (3, 1):
            new_topics = [
                NewTopic(name=t, num_partitions=PARTITIONS, replication_factor=rf) for t in missing
            ]
            try:
                await admin.create_topics(new_topics)
                last_err = None
                break
            except Exception as e:  # noqa: BLE001 - broker rejects the factor, try the next
                last_err = e
        if last_err is not None:
            raise last_err

        for t in missing:
            print(f"+ created {t}")
        print(f"done: {len(missing)} created, {len(wanted) - len(missing)} already existed")
    finally:
        await admin.close()


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--list", action="store_true", help="list existing topics and exit")
    args = ap.parse_args()
    asyncio.run(run(args.list))


if __name__ == "__main__":
    main()

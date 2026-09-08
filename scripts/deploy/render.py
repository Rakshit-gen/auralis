#!/usr/bin/env python3
"""Create or update the Auralis services on Render from the Render API.

Render's free tier has only web services, so all eight services deploy as web
services (the seven internal ones still require a valid gateway identity
signature and, on /internal, the shared token). The ai-media worker runs
in-process with the ai-media API (AI_MEDIA_RUN_WORKER defaults to true).

Reads credentials from .env.deploy (repo root). Idempotent: existing services
are updated in place, missing ones are created.

    RENDER_API_KEY=...  python scripts/deploy/render.py            # create/update, then deploy
    python scripts/deploy/render.py --no-deploy                    # sync config only
    python scripts/deploy/render.py --only auralis-gateway         # one service
    python scripts/deploy/render.py --print-env auralis-playback   # show computed env, do nothing

The script never prints secret values.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

API = "https://api.render.com/v1"
REPO = "https://github.com/Rakshit-gen/auralis"
BRANCH = "master"
REGION = "ohio"  # closest Render region to Neon us-east-2
PLAN = "free"
RENDER_PORT = 10000  # Render's default web port; every service binds this and Render routes to it

ROOT = Path(__file__).resolve().parents[2]
ENV_DEPLOY = ROOT / ".env.deploy"

# name -> spec. `internal` services are reached by the gateway over their public
# onrender.com URL; `deps` lists other services whose URL this one also needs.
SERVICES: dict[str, dict] = {
    "auralis-gateway": {
        "dockerfile": "services/gateway/Dockerfile",
        "port": 8080,
        "addr_env": "GATEWAY_HTTP_ADDR",
        "public": True,
        "deps": ["auth", "user", "content", "playback", "analytics", "ai-media", "recommendation"],
        "kafka": False,
        "redis": True,
    },
    "auralis-auth": {
        "dockerfile": "services/auth/Dockerfile",
        "port": 8081,
        "addr_env": "AUTH_HTTP_ADDR",
        "db_env": "AUTH_DATABASE_URL",
        "bootstrap_admin": True,
    },
    "auralis-user": {
        "dockerfile": "services/user/Dockerfile",
        "port": 8083,
        "addr_env": "USER_HTTP_ADDR",
        "db_env": "USER_DATABASE_URL",
    },
    "auralis-content": {
        "dockerfile": "services/content/Dockerfile",
        "port": 8082,
        "addr_env": "CONTENT_HTTP_ADDR",
        "db_env": "CONTENT_DATABASE_URL",
        "s3": True,
    },
    "auralis-playback": {
        "dockerfile": "services/playback/Dockerfile",
        "port": 8084,
        "addr_env": "PLAYBACK_HTTP_ADDR",
        "db_env": "PLAYBACK_DATABASE_URL",
        "s3": True,
        "deps": ["content", "user"],
    },
    "auralis-analytics": {
        "dockerfile": "services/analytics/Dockerfile",
        "port": 8087,
        "addr_env": "ANALYTICS_HTTP_ADDR",
        "db_env": "ANALYTICS_DATABASE_URL",
    },
    "auralis-ai-media": {
        "dockerfile": "services/ai_media/Dockerfile",
        "port": 8085,
        "addr_env": "AI_MEDIA_HTTP_ADDR",
        "db_env": "AI_MEDIA_DATABASE_URL",
        "s3": True,
        "deps": ["content", "user"],
        "python": True,
    },
    "auralis-recommendation": {
        "dockerfile": "services/recommendation/Dockerfile",
        "port": 8086,
        "addr_env": "RECOMMENDATION_HTTP_ADDR",
        "db_env": "RECOMMENDATION_DATABASE_URL",
        "python": True,
    },
}

# gateway dependency key -> service name and its *_SERVICE_URL env var
DEP_ENV = {
    "auth": "AUTH_SERVICE_URL",
    "user": "USER_SERVICE_URL",
    "content": "CONTENT_SERVICE_URL",
    "playback": "PLAYBACK_SERVICE_URL",
    "analytics": "ANALYTICS_SERVICE_URL",
    "ai-media": "AI_MEDIA_SERVICE_URL",
    "recommendation": "RECOMMENDATION_SERVICE_URL",
}
DEP_SERVICE = {k: ("auralis-" + k) for k in DEP_ENV}


def load_env_deploy() -> dict[str, str]:
    if not ENV_DEPLOY.exists():
        sys.exit(f"{ENV_DEPLOY} not found. Fill it in from .env.deploy first.")
    out: dict[str, str] = {}
    for line in ENV_DEPLOY.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        k, _, v = line.partition("=")
        out[k.strip()] = v.strip()
    return out


class Render:
    def __init__(self, key: str):
        self._key = key

    def _req(self, method: str, path: str, body: dict | list | None = None) -> object:
        url = path if path.startswith("http") else API + path
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(url, data=data, method=method)
        req.add_header("Authorization", f"Bearer {self._key}")
        req.add_header("Accept", "application/json")
        if data is not None:
            req.add_header("Content-Type", "application/json")
        try:
            with urllib.request.urlopen(req) as resp:
                raw = resp.read()
        except urllib.error.HTTPError as e:
            detail = e.read().decode(errors="replace")
            sys.exit(f"Render API {method} {path} -> {e.code}\n{detail}")
        return json.loads(raw) if raw else None

    def owner_id(self) -> str:
        owners = self._req("GET", "/owners?limit=50")
        assert isinstance(owners, list) and owners, "no workspaces on this API key"
        if len(owners) == 1:
            return owners[0]["owner"]["id"]
        for o in owners:
            print(f"  {o['owner']['id']}  {o['owner']['name']} ({o['owner']['email']})")
        picked = os.environ.get("RENDER_OWNER_ID")
        if not picked:
            sys.exit("Multiple workspaces. Set RENDER_OWNER_ID to one of the ids above.")
        return picked

    def find(self, name: str) -> dict | None:
        res = self._req("GET", f"/services?name={name}&limit=20")
        assert isinstance(res, list)
        for row in res:
            svc = row.get("service", row)
            if svc.get("name") == name:
                return svc
        return None

    def create(self, payload: dict) -> dict:
        res = self._req("POST", "/services", payload)
        assert isinstance(res, dict)
        return res.get("service", res)

    def put_env(self, service_id: str, env: dict[str, str]) -> None:
        body = [{"key": k, "value": v} for k, v in sorted(env.items())]
        self._req("PUT", f"/services/{service_id}/env-vars", body)

    def deploy(self, service_id: str, clear_cache: bool = False) -> str:
        body = {"clearCache": "clear" if clear_cache else "do_not_clear"}
        res = self._req("POST", f"/services/{service_id}/deploys", body)
        assert isinstance(res, dict)
        return res["id"]

    def deploy_status(self, service_id: str, deploy_id: str) -> str:
        res = self._req("GET", f"/services/{service_id}/deploys/{deploy_id}")
        assert isinstance(res, dict)
        return res.get("status", "unknown")


def shared_env(d: dict[str, str]) -> dict[str, str]:
    env = {
        "JWT_SECRET": d["JWT_SECRET"],
        "IDENTITY_SECRET": d["IDENTITY_SECRET"],
        "SERVICE_SHARED_TOKEN": d["SERVICE_SHARED_TOKEN"],
        "JWT_ISSUER": d.get("JWT_ISSUER", "auralis-auth"),
        "JWT_AUDIENCE": d.get("JWT_AUDIENCE", "auralis"),
        "LOG_LEVEL": d.get("LOG_LEVEL", "info"),
    }
    if d.get("OTEL_EXPORTER_OTLP_ENDPOINT"):
        env["OTEL_EXPORTER_OTLP_ENDPOINT"] = d["OTEL_EXPORTER_OTLP_ENDPOINT"]
    return env


def kafka_env(d: dict[str, str]) -> dict[str, str]:
    if not d.get("KAFKA_BROKERS"):
        sys.exit("KAFKA_BROKERS is empty in .env.deploy (need the Redpanda Cloud seed broker).")
    env = {"KAFKA_BROKERS": d["KAFKA_BROKERS"]}
    if d.get("KAFKA_SASL_USERNAME"):
        env.update(
            KAFKA_SASL_MECHANISM=d.get("KAFKA_SASL_MECHANISM", "SCRAM-SHA-256"),
            KAFKA_SASL_USERNAME=d["KAFKA_SASL_USERNAME"],
            KAFKA_SASL_PASSWORD=d["KAFKA_SASL_PASSWORD"],
            KAFKA_TLS_ENABLED=d.get("KAFKA_TLS_ENABLED", "true"),
        )
    return env


def s3_env(d: dict[str, str]) -> dict[str, str]:
    for k in ("S3_ENDPOINT", "S3_ACCESS_KEY", "S3_SECRET_KEY"):
        if not d.get(k):
            sys.exit(f"{k} is empty in .env.deploy")
    return {
        "S3_ENDPOINT": d["S3_ENDPOINT"],
        "S3_ACCESS_KEY": d["S3_ACCESS_KEY"],
        "S3_SECRET_KEY": d["S3_SECRET_KEY"],
        "S3_BUCKET": d.get("S3_BUCKET", "auralis-media"),
        "S3_REGION": d.get("S3_REGION", "auto"),
        "S3_USE_SSL": d.get("S3_USE_SSL", "true"),
    }


def compute_env(name: str, spec: dict, d: dict[str, str], urls: dict[str, str]) -> dict[str, str]:
    env = shared_env(d)
    env[spec["addr_env"]] = f"0.0.0.0:{RENDER_PORT}"
    env["PORT"] = str(RENDER_PORT)
    if spec.get("kafka", True):
        env.update(kafka_env(d))
    if spec.get("db_env"):
        if not d.get(spec["db_env"]):
            sys.exit(f"{spec['db_env']} is empty in .env.deploy")
        env[spec["db_env"]] = d[spec["db_env"]]
    if spec.get("s3"):
        env.update(s3_env(d))
    if spec.get("redis"):
        if not d.get("REDIS_URL"):
            sys.exit("REDIS_URL is empty in .env.deploy")
        env["REDIS_URL"] = d["REDIS_URL"]
    if d.get("CORS_ALLOWED_ORIGINS") and spec.get("public"):
        env["CORS_ALLOWED_ORIGINS"] = d["CORS_ALLOWED_ORIGINS"]
    if spec.get("bootstrap_admin"):
        env["AUTH_BOOTSTRAP_ADMIN_EMAIL"] = d["ADMIN_EMAIL"]
        env["AUTH_BOOTSTRAP_ADMIN_PASSWORD"] = d["ADMIN_PASSWORD"]
        env["AUTH_BOOTSTRAP_ADMIN_NAME"] = d.get("AUTH_BOOTSTRAP_ADMIN_NAME", "Auralis Admin")
    for dep in spec.get("deps", []):
        target = DEP_SERVICE[dep]
        if target not in urls:
            sys.exit(f"{name} needs {target} URL but it is not known yet")
        env[DEP_ENV[dep]] = urls[target]
    if spec.get("python") and name == "auralis-ai-media":
        env["AI_MEDIA_RUN_WORKER"] = "true"
    return env


def create_payload(name: str, spec: dict, owner: str, env: dict[str, str]) -> dict:
    details: dict = {
        "runtime": "docker",
        "plan": PLAN,
        "region": REGION,
        "envSpecificDetails": {
            "dockerfilePath": "./" + spec["dockerfile"],
            "dockerContext": ".",
        },
    }
    if spec.get("public"):
        details["healthCheckPath"] = "/health"
    return {
        "type": "web_service",
        "name": name,
        "ownerId": owner,
        "repo": REPO,
        "branch": BRANCH,
        "autoDeploy": "no",
        "serviceDetails": details,
        "envVars": [{"key": k, "value": v} for k, v in sorted(env.items())],
    }


def service_url(svc: dict, name: str) -> str:
    d = svc.get("serviceDetails") or {}
    url = d.get("url")
    if not url:
        # fall back to the conventional hostname
        url = f"https://{name}.onrender.com"
    return url.rstrip("/")


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--no-deploy", action="store_true", help="sync config, do not trigger deploys")
    ap.add_argument("--only", action="append", help="limit to these service names")
    ap.add_argument("--print-env", metavar="NAME", help="print computed env for one service and exit")
    ap.add_argument("--clear-cache", action="store_true", help="clear build cache on deploy")
    args = ap.parse_args()

    d = load_env_deploy()

    if args.print_env:
        name = args.print_env
        # dep URLs are guessed for a dry print
        urls = {n: f"https://{n}.onrender.com" for n in SERVICES}
        env = compute_env(name, SERVICES[name], d, urls)
        for k in sorted(env):
            shown = env[k] if not any(s in k for s in ("SECRET", "PASSWORD", "TOKEN", "KEY", "DATABASE_URL", "REDIS_URL")) else "***"
            print(f"{k}={shown}")
        return

    key = os.environ.get("RENDER_API_KEY") or d.get("RENDER_API_KEY")
    if not key:
        sys.exit("RENDER_API_KEY not set (env or .env.deploy)")
    r = Render(key)
    owner = r.owner_id()
    print(f"workspace {owner}")

    names = list(SERVICES)
    if args.only:
        names = [n for n in names if n in set(args.only)]

    # Phase 1: ensure every service exists; collect URLs.
    existing: dict[str, dict] = {}
    urls: dict[str, str] = {}
    for name in SERVICES:  # always resolve all, deps need them
        svc = r.find(name)
        if svc:
            existing[name] = svc
            urls[name] = service_url(svc, name)
            print(f"  found {name}  {urls[name]}")

    for name in names:
        if name in existing:
            continue
        spec = SERVICES[name]
        # first create with a provisional env; phase 2 rewrites it with real dep URLs
        provisional_urls = dict(urls)
        for dep in spec.get("deps", []):
            provisional_urls.setdefault(DEP_SERVICE[dep], f"https://{DEP_SERVICE[dep]}.onrender.com")
        env = compute_env(name, spec, d, provisional_urls)
        svc = r.create(create_payload(name, spec, owner, env))
        existing[name] = svc
        urls[name] = service_url(svc, name)
        print(f"  created {name}  {urls[name]}")
        time.sleep(1)

    # Phase 2: rewrite every targeted service's env with the real URL map.
    for name in names:
        spec = SERVICES[name]
        env = compute_env(name, spec, d, urls)
        r.put_env(existing[name]["id"], env)
        print(f"  env synced {name} ({len(env)} vars)")

    if args.no_deploy:
        print("config synced; skipping deploys (--no-deploy)")
        print_next_steps(urls)
        return

    # Phase 3: deploy and poll.
    deploys = {}
    for name in names:
        did = r.deploy(existing[name]["id"], clear_cache=args.clear_cache)
        deploys[name] = did
        print(f"  deploy queued {name} ({did})")

    print("\nwaiting for deploys (Ctrl-C to stop watching; deploys continue)...")
    terminal = {"live", "deactivated", "build_failed", "update_failed", "canceled", "pre_deploy_failed"}
    pending = dict(deploys)
    while pending:
        time.sleep(15)
        for name, did in list(pending.items()):
            st = r.deploy_status(existing[name]["id"], did)
            if st in terminal:
                mark = "OK" if st == "live" else "FAIL"
                print(f"  [{mark}] {name}: {st}")
                del pending[name]
            else:
                print(f"  ... {name}: {st}")

    print_next_steps(urls)


def print_next_steps(urls: dict[str, str]) -> None:
    gw = urls.get("auralis-gateway", "https://auralis-gateway.onrender.com")
    print("\nNext:")
    print(f"  API base:  {gw}/api")
    print(f"  Seed:      API_BASE={gw}/api \\")
    print("             SERVICE_SHARED_TOKEN=... AUTH_BOOTSTRAP_ADMIN_EMAIL=... AUTH_BOOTSTRAP_ADMIN_PASSWORD=... \\")
    print("             .venv/bin/python scripts/seed.py")
    print(f"  Verify:    API_BASE={gw}/api .venv/bin/python scripts/e2e.py")
    print(f"  Frontend:  set NEXT_PUBLIC_API_BASE={gw}/api on Vercel")


if __name__ == "__main__":
    main()

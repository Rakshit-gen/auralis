# Auralis Helm chart

Installs the whole platform: the eight services, the ai-media worker, the web
client, and optionally in-cluster Postgres, Redis, Redpanda, MinIO, an
OpenTelemetry Collector, and Prometheus.

This is the parametrized equivalent of the plain manifests in
[`../../k8s/`](../../k8s/). Use whichever you prefer.

## Install

```
helm install auralis infra/helm/auralis --namespace auralis --create-namespace
```

Self-contained defaults come up with everything in-cluster. Watch the rollout:

```
kubectl -n auralis get pods -w
```

Then port-forward the gateway (no ingress host is real by default) and seed:

```
kubectl -n auralis port-forward svc/gateway 8080:8080 &
SERVICE_SHARED_TOKEN=dev-service-token-change-me-0123456789 \
AUTH_BOOTSTRAP_ADMIN_EMAIL=admin@auralis.local \
AUTH_BOOTSTRAP_ADMIN_PASSWORD=auralis-admin-pw \
.venv/bin/python scripts/seed.py
```

## Production install

Do not ship the placeholder secrets. Create the two Secrets yourself and tell
the chart not to:

```
kubectl create namespace auralis
kubectl -n auralis create secret generic auralis-secrets \
  --from-literal=JWT_SECRET=... \
  --from-literal=IDENTITY_SECRET=... \
  --from-literal=SERVICE_SHARED_TOKEN=... \
  --from-literal=ADMIN_EMAIL=... --from-literal=ADMIN_PASSWORD=... \
  --from-literal=S3_ACCESS_KEY=... --from-literal=S3_SECRET_KEY=... \
  --from-literal=GROQ_API_KEY=
kubectl -n auralis create secret generic auralis-db \
  --from-literal=AUTH_DATABASE_URL=... \
  --from-literal=USER_DATABASE_URL=... \
  --from-literal=CONTENT_DATABASE_URL=... \
  --from-literal=PLAYBACK_DATABASE_URL=... \
  --from-literal=ANALYTICS_DATABASE_URL=... \
  --from-literal=AI_MEDIA_DATABASE_URL=... \
  --from-literal=RECOMMENDATION_DATABASE_URL=...

helm install auralis infra/helm/auralis -n auralis -f prod-values.yaml
```

`prod-values.yaml`:

```yaml
secrets:
  create: false
image:
  tag: v1.0.0            # pin, never latest
postgres: { enabled: false }
redis: { enabled: false }
minio: { enabled: false }
redpanda:
  enabled: true          # or false + a managed / replicated cluster
config:
  kafkaBrokers: "redpanda:9092"
  corsAllowedOrigins: "https://auralis.example.com"
  s3:
    endpoint: "<account>.r2.cloudflarestorage.com"
    useSsl: "true"
ingress:
  host: auralis.example.com
  tls: { enabled: true, secretName: auralis-tls }
frontend:
  apiBase: "https://auralis.example.com/api"
```

## Values

| Key | Default | Notes |
| --- | --- | --- |
| `image.registry` / `image.tag` | `ghcr.io/rakshit-gen` / `latest` | image is `<registry>/auralis-<name>:<tag>` |
| `secrets.create` | `true` | set `false` to bring your own Secrets |
| `services.<name>.replicas` | 1 or 2 | per-service replica count |
| `services.<name>.resources` | small | requests and limits |
| `worker.enabled` / `worker.replicas` | `true` / 1 | ai-media generation worker |
| `frontend.enabled` / `frontend.apiBase` | `true` / localhost | web client |
| `ingress.enabled` / `ingress.host` / `ingress.tls` | `true` / example.com / off | |
| `migrateJob.enabled` | `true` | one-shot migration Job per revision |
| `postgres` / `redis` / `redpanda` / `minio` | `enabled: true` | in-cluster infra; turn off for managed |
| `observability.enabled` | `true` | OTel Collector + Prometheus |

## Upgrade

```
helm upgrade auralis infra/helm/auralis -n auralis -f prod-values.yaml
```

Deployments roll one pod at a time (`maxSurge: 1, maxUnavailable: 0`), so a
release carrying a new migration applies it once. A fresh `*-migrate-<rev>` Job
also runs each upgrade.

## Uninstall

```
helm uninstall auralis -n auralis
```

PersistentVolumeClaims from the StatefulSets are left behind on purpose; delete
them by hand if you want the data gone.

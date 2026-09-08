# Kubernetes manifests

Plain manifests for running the whole platform in a cluster: the eight
services, the ai-media worker, the web client, and (optionally) in-cluster
Postgres, Redis, Redpanda, MinIO, an OpenTelemetry Collector, and Prometheus.

For the managed free-tier deployment (Vercel + Render + Neon + Upstash + R2)
see [`../../docs/DEPLOYMENT.md`](../../docs/DEPLOYMENT.md). These manifests are
for a self-hosted cluster.

## Layout

| File | Contents |
| --- | --- |
| `namespace.yaml` | the `auralis` namespace |
| `config.yaml` | `auralis-config` ConfigMap, `auralis-secrets` and `auralis-db` Secrets (dev placeholders) |
| `postgres.yaml` | single Postgres StatefulSet, seven databases created on init |
| `infra.yaml` | Redis, single-node Redpanda, MinIO, and a bucket-create Job |
| `observability.yaml` | OpenTelemetry Collector and Prometheus |
| `services-go.yaml` | gateway, auth, user, content, playback, analytics |
| `services-python.yaml` | ai-media API, ai-media worker, recommendation |
| `frontend.yaml` | Next.js client and the Ingress |
| `job-migrate.yaml` | optional one-shot migration Job (not in the kustomization) |
| `kustomization.yaml` | applies everything except `job-migrate.yaml` |

## Apply

```
kubectl apply -k infra/k8s/
```

or without kustomize:

```
kubectl apply -f infra/k8s/namespace.yaml
kubectl apply -f infra/k8s/config.yaml
kubectl apply -f infra/k8s/postgres.yaml -f infra/k8s/infra.yaml
kubectl apply -f infra/k8s/observability.yaml
kubectl apply -f infra/k8s/services-go.yaml -f infra/k8s/services-python.yaml
kubectl apply -f infra/k8s/frontend.yaml
```

Wait for Postgres, Redpanda, and MinIO to be ready, then the services. Each
service applies its own migrations on startup; the rollout strategy
(`maxSurge: 1, maxUnavailable: 0`) means a deploy carrying a new migration
never has two pods applying it at once. To run migrations as one explicit step
first:

```
kubectl apply -f infra/k8s/job-migrate.yaml
kubectl wait --for=condition=complete job/migrate -n auralis --timeout=180s
```

## Before anything public

1. Replace every value in the `auralis-secrets` Secret. The three shared
   secrets (`JWT_SECRET`, `IDENTITY_SECRET`, `SERVICE_SHARED_TOKEN`) must be
   identical across all services. Use a real secrets workflow
   (sealed-secrets, external-secrets, or `kubectl create secret` from a
   vault), not the committed placeholders.
2. Point `auralis-db` at managed databases and delete `postgres.yaml`, or keep
   the StatefulSet and give it a real storage class and backups.
3. For managed Redis / Kafka / object storage, update `auralis-config` and
   `auralis-secrets` and remove the matching pieces from `infra.yaml`. A
   single-node Redpanda has no replication and is not production-grade on its
   own.
4. Set the Ingress `host`, add a `tls` block with a cert-manager issuer, and
   set `NEXT_PUBLIC_API_BASE` on the frontend and `CORS_ALLOWED_ORIGINS` in
   `auralis-config` to the public URL.
5. Set image tags (via the kustomization `images:` block) to a pinned version,
   not `latest`.

## Seeding

Run the seed script from a machine with the repo, pointed at the gateway (port-
forward or the Ingress):

```
kubectl -n auralis port-forward svc/gateway 8080:8080 &
SERVICE_SHARED_TOKEN=<value> \
AUTH_BOOTSTRAP_ADMIN_EMAIL=<value> AUTH_BOOTSTRAP_ADMIN_PASSWORD=<value> \
.venv/bin/python scripts/seed.py
```

## Scaling notes

- gateway, auth, playback, and frontend are horizontally scalable; bump
  `replicas`.
- The ai-media worker scales by replica count (one job per worker). Its
  Deployment uses `strategy: Recreate` so a rollout does not briefly double the
  workers.
- user, content, analytics, and recommendation each run a Kafka consumer in a
  fixed consumer group; extra replicas share partitions but there is little
  reason to exceed 2.
- Postgres and Redpanda here are single-instance. Swap in an operator
  (CloudNativePG, Redpanda operator) or managed services for HA.

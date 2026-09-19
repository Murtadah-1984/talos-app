# Disaster Recovery

See [ADR-0004](../adr/0004-git-source-of-truth.md) and §33 of the product spec.

## What's authoritative

- **Git** — desired cluster/infrastructure/application configuration. This is what
  Argo CD and Cluster API actually reconcile against.
- **PostgreSQL** — platform metadata: who owns what, workflow/audit history,
  credential references (not plaintext credentials).
- **Vault** (`PLATFORM_SECRET_STORE_BACKEND=vault`) — secrets, when configured for
  production. `PLATFORM_SECRET_STORE_BACKEND=local` (the default) stores secrets
  locally instead; back that up as part of the platform database/volume, since it
  is not otherwise replicated. Vault's own backup/DR (snapshot/unseal) is Vault's
  responsibility, not this project's.

## What is explicitly NOT authoritative

The platform's own database is metadata, not desired state. Losing it does not orphan
or destroy managed clusters: Git still has everything Argo CD/Cluster API need to
reconcile, and Talos machines keep running independent of whether the platform
database exists.

## Backing up PostgreSQL

Run [`scripts/backup-postgres.sh`](../../scripts/backup-postgres.sh) on a schedule
(cron, a Kubernetes CronJob, or your managed-Postgres provider's snapshot mechanism)
against `PLATFORM_POSTGRES_DSN`. It produces a timestamped, compressed
`pg_dump --format=custom` archive:

```bash
PLATFORM_POSTGRES_DSN=postgres://user:pass@host:5432/platform \
  ./scripts/backup-postgres.sh /path/to/backup-dir
```

Store the resulting `platform-<timestamp>.dump` file somewhere durable and
off-cluster (object storage, a separate backup volume) — a backup that lives on the
same disk as the database it protects does not survive the failure modes it exists
for.

## Recovering the platform itself

1. Restore PostgreSQL from the most recent backup using
   [`scripts/restore-postgres.sh`](../../scripts/restore-postgres.sh):

   ```bash
   PLATFORM_POSTGRES_DSN=postgres://user:pass@host:5432/platform \
     ./scripts/restore-postgres.sh /path/to/platform-<timestamp>.dump
   ```

2. If running Vault-backed secrets (`PLATFORM_SECRET_STORE_BACKEND=vault`), recover
   Vault independently per its own backup/DR procedure — the platform stores only
   references to secrets in Postgres, not the secret values themselves.
3. Redeploy `platform-api`/`platform-worker`/`platform-scheduler` via the Helm chart
   (`deploy/helm/platform`) pointing at the restored database.
4. `platform-api` re-applies any migrations newer than the restored snapshot on
   startup.
5. In-flight workflows resume from their last completed step (ADR-0005) — no manual
   intervention needed for workflows that were `RUNNING` at the time of the outage.
   Workflows that completed a step after the backup was taken but before the outage
   will re-run that step (steps must be idempotent, per the `StepFunc` contract in
   `internal/workflows/engine.go`).

## Recovering a managed cluster

Not applicable to the platform's own state: a managed cluster's desired state lives in
Git, and Argo CD/Cluster API continue reconciling it independent of the platform's
availability. If the *cluster itself* is lost, standard Talos/Kubernetes/CAPI recovery
procedures apply — this is intentionally outside the platform's own responsibility
(§41 responsibility matrix).

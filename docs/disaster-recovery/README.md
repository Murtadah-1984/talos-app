# Disaster Recovery

See [ADR-0004](../adr/0004-git-source-of-truth.md) and §33 of the product spec.

## What's authoritative

- **Git** — desired cluster/infrastructure/application configuration. This is what
  Argo CD and Cluster API actually reconcile against.
- **PostgreSQL** — platform metadata: who owns what, workflow/audit history,
  credential references (not plaintext credentials).
- **Vault** (production; not yet implemented — see [../roadmap.md](../roadmap.md)) —
  secrets.

## What is explicitly NOT authoritative

The platform's own database is metadata, not desired state. Losing it does not orphan
or destroy managed clusters: Git still has everything Argo CD/Cluster API need to
reconcile, and Talos machines keep running independent of whether the platform
database exists.

## Recovering the platform itself

1. Restore PostgreSQL from a routine backup (standard `pg_dump`/`pg_restore` or your
   platform's managed-Postgres backup mechanism — not yet automated by this project).
2. Redeploy `platform-api`/`platform-worker`/`platform-scheduler` via the Helm chart
   (`deploy/helm/platform`) pointing at the restored database.
3. `platform-api` re-applies any migrations newer than the restored snapshot on
   startup.
4. In-flight workflows resume from their last completed step (ADR-0005) — no manual
   intervention needed for workflows that were `RUNNING` at the time of the outage.

## Recovering a managed cluster

Not applicable to the platform's own state: a managed cluster's desired state lives in
Git, and Argo CD/Cluster API continue reconciling it independent of the platform's
availability. If the *cluster itself* is lost, standard Talos/Kubernetes/CAPI recovery
procedures apply — this is intentionally outside the platform's own responsibility
(§41 responsibility matrix).

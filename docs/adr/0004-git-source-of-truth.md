# ADR-0004: Git is the Source of Truth for Desired Cluster State

## Status
Accepted

## Context
PostgreSQL stores application metadata (who owns what, workflow history, audit trail),
but if it also held the authoritative desired state of clusters, the platform database
would become a second source of truth that Argo CD/CAPI don't read from — reconstructing
infrastructure after a platform database loss would then be impossible without also
recovering that exact database.

## Decision
Desired cluster/infrastructure/application configuration (cluster topology, Kubernetes
and Talos versions, machine roles, CNI, storage, ingress, Argo CD Applications) is
represented as versioned YAML/Helm values under a `gitops/` repository layout (§3 of the
product spec) and is authoritative. PostgreSQL stores:

- a *reference* to the Git repository/path/commit associated with each cluster,
- the workflow/change history that produced each commit,
- operational metadata that has no business being in Git (credentials pointers, audit
  logs, live health snapshots, user/RBAC data).

Every meaningful infrastructure change flows through: generate → commit → PR → review →
merge (§20, "Change Sets"). The platform never treats an in-memory or database-only
representation of desired state as something Argo CD/CAPI should act on directly.

## Consequences
- Disaster recovery for *managed clusters* only requires Git + Vault (secrets), not the
  platform's own database — see [ADR-0006](0006-secrets-management.md) and §33.
- The platform database can be rebuilt/restored without destroying or orphaning managed
  clusters, since Git remains the record of what should exist.
- All manifest-generation code must be deterministic and idempotent: regenerating from
  the same stored parameters must produce the same YAML (modulo formatting), so PRs show
  clean diffs.

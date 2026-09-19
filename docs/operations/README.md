# Operations

## Health/readiness

Every backend component exposes:

- `GET /healthz` — liveness (process is up, no dependency checks).
- `GET /readyz` — readiness (currently process-level only; dependency health checks are
  a Phase 7 hardening item — see [../roadmap.md](../roadmap.md)).
- `GET /metrics` — Prometheus exposition format (`platform-api` only today).

## Common tasks (§17)

All of the following go through the REST API (and therefore the CLI, which is a thin
client over it) — never direct database or Talos access:

| Action | Endpoint |
| --- | --- |
| View cluster health | `GET /api/v1/clusters/{id}/health` |
| View cluster nodes | `GET /api/v1/clusters/{id}/nodes` |
| View GitOps/Argo CD status | `GET /api/v1/clusters/{id}/gitops` |
| Reboot a machine (graceful, via Talos) | `POST /api/v1/machines/{id}/reboot` |
| Hard power on/off/cycle a machine (via BMC/hypervisor) | `POST /api/v1/machines/{id}/power/{on,off,cycle}` |
| Upgrade a machine's Talos version | `POST /api/v1/machines/{id}/upgrade` |
| Upgrade a cluster | `POST /api/v1/clusters/{id}/upgrade` |
| Scale a worker pool | `POST /api/v1/clusters/{id}/scale` |
| Trigger an Argo CD sync now | `POST /api/v1/clusters/{id}/sync` |
| Destroy a cluster | `DELETE /api/v1/clusters/{id}` |

Destructive operations (`upgrade`, `scale`, `delete`) require at least the
`CLUSTER_ADMIN`/`OPERATOR` role on the target cluster (or an ancestor scope) — see
[../security/README.md](../security/README.md).

## Watching a workflow

Cluster provisioning/upgrade/scaling runs as a durable workflow. Poll its status:

```bash
go run ./cmd/talos-platform workflow get <workflow-id>
```

or watch `GET /api/v1/workflows/{id}` from the UI's Workflows tab, which shows each
step's status as it completes.

## High availability

- `platform-api` and `platform-worker` are stateless and safe to run at any replica
  count behind a load balancer; workflow step execution is serialized via Postgres
  `SELECT ... FOR UPDATE SKIP LOCKED` claims (ADR-0005), so multiple `platform-worker`
  replicas share the load without double-executing a step.
- `platform-scheduler` runs its periodic reconciliation pass only on the replica
  holding a Redis-lease leader election (`internal/infrastructure/leaderelection`),
  refreshed on a 45s TTL. It is now safe to run more than one replica: standbys skip
  their reconciliation tick until they observe the lease expire, and failover is
  automatic (no manual intervention) within one lease TTL of the leader disappearing.
  This requires `PLATFORM_REDIS_ADDR` to point at a reachable Redis.

## When a workflow gets stuck

A workflow that lands in `NEEDS_ATTENTION` stopped because a step failed — the
platform never automatically retries or rolls back destructively (§35). Check the
step's `Error` field (via the workflow detail endpoint/UI), fix the underlying
condition, and re-submit the originating request with a **new** idempotency key once
you're ready to try again.

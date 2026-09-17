# Architecture

## What this system is

The Talos Platform is a **Kubernetes cluster lifecycle and platform management system**
for clusters running on [Talos Linux](https://www.talos.dev/). It is explicitly **not**
a GitOps engine, **not** a Kubernetes controller framework, and **not** a replacement for
Cluster API. It orchestrates and visualizes; specialized systems reconcile.

```
                         ┌─────────────────────────┐
                         │        Web UI            │
                         │      React/TypeScript    │
                         └────────────┬────────────┘
                                      │
                                      ▼
                         ┌─────────────────────────┐
                         │       REST API           │
                         │         Go               │
                         └────────────┬────────────┘
                                      │
                    ┌─────────────────┼─────────────────┐
                    │                 │                 │
                    ▼                 ▼                 ▼
             ┌────────────┐   ┌──────────────┐  ┌──────────────┐
             │ Cluster     │   │ Machine      │  │ GitOps       │
             │ Lifecycle   │   │ Management   │  │ Management   │
             └──────┬─────┘   └──────┬───────┘  └──────┬───────┘
                    │                 │                 │
                    ▼                 ▼                 ▼
              Cluster API        Talos API         GitHub/Git
                    │                 │                 │
                    ▼                 │                 ▼
              Kubernetes             │            Argo CD
                                      │                 │
                                      └────────┬────────┘
                                               ▼
                                        Kubernetes Cluster
```

## Responsibility matrix

| Component               | Responsibility                                     |
| ------------------------ | --------------------------------------------------- |
| Platform Application    | User-facing orchestration and lifecycle management |
| Git                     | Desired state                                       |
| Argo CD                 | GitOps reconciliation                               |
| Cluster API             | Kubernetes cluster lifecycle reconciliation         |
| Talos                   | Machine OS lifecycle                                |
| Kubernetes              | Container orchestration                             |
| Infrastructure Provider | Compute/network/VM/bare-metal lifecycle             |
| PostgreSQL              | Application metadata                                |
| Vault                   | Secrets                                             |
| Prometheus              | Metrics                                             |
| OpenTelemetry           | Telemetry                                           |
| Grafana                 | Visualization                                       |

The platform never duplicates the reconciliation logic of Argo CD, Cluster API, or
Kubernetes controllers. Where a controller already owns a piece of state, the platform's
job is to generate the desired-state manifest, commit it to Git, and observe the result —
never to `kubectl apply` it directly in the steady state.

## Management Plane vs. Cluster Plane

**Management Plane** (owned by this application, backed by PostgreSQL):
users, organizations, projects, environments, sites, clusters (metadata), machines
(metadata), credentials, infrastructure providers, cluster templates, Git repositories,
GitOps configuration records, workflows, audit logs, observability metadata.

**Cluster Plane** (owned by Kubernetes-native systems, observed by this application):
Talos machines, Kubernetes control planes/workers, Cluster API resources, Kubernetes
resources, Argo CD Applications, Helm releases, networking, storage, in-cluster
observability.

## Imperative vs. declarative operations

This is the most important design principle in the system.

- **Imperative** operations talk directly to the Talos API (or Kubernetes API) for
  actions that are inherently point-in-time: discovery, health, reboot, log retrieval,
  version inspection. These are read-mostly or narrowly-scoped mutating calls with no
  meaningful "desired state" — see [ADR-0001](adr/0001-talos-integration.md).
- **Declarative** operations describe desired state (topology, versions, roles, CNI,
  storage, ingress, Argo CD apps). These are rendered into manifests, committed to Git,
  and reconciled by Argo CD / Cluster API — see [ADR-0003](adr/0003-argocd-gitops.md) and
  [ADR-0004](adr/0004-git-source-of-truth.md).

## Layered backend architecture

The Go backend follows Clean/Hexagonal Architecture and Dependency Inversion:

```
cmd/                      entry points (platform-api, platform-worker, platform-scheduler, CLI)
internal/
  domain/                 entities, value objects, domain services — no external deps
  application/            use cases / orchestration — depends only on domain + ports
  infrastructure/         postgres, redis, rabbitmq, config, vault — implements ports
  interfaces/
    http/                 REST handlers, DTOs, middleware
    websocket/            real-time operation/event streaming
  integrations/           talos, kubernetes, clusterapi, argocd, github, proxmox, baremetal
  workflows/              durable, resumable workflow engine + workflow definitions
  observability/          OpenTelemetry wiring, metrics
  auth/                   OIDC, RBAC
```

`domain` never imports `infrastructure` or `integrations`. All external systems are
reached through interfaces (ports) defined alongside the domain/application layer and
implemented in `infrastructure`/`integrations`. See [ADR-0007](adr/0007-infrastructure-provider-abstraction.md)
for the provider abstraction pattern used across Talos, Git, and infrastructure providers.

## Cluster provider modes

A cluster is managed through exactly one of two modes, recorded on the cluster record:

- `DIRECT_TALOS` — the platform talks to Talos machines directly (via `talosctl`-equivalent
  gRPC API) to bootstrap and manage the cluster. Used for simple/bare-metal/edge sites
  where running Cluster API is not justified.
- `CLUSTER_API` — the platform generates and commits CAPI resources (Cluster,
  ControlPlane, MachineDeployment, infrastructure provider CRs) to Git; CAPI controllers
  (running in a management cluster) perform the actual reconciliation.

See [ADR-0002](adr/0002-cluster-api-integration.md).

## Capability states

Integrations that are not fully wired for a given deployment must report an explicit
capability state rather than silently no-op or fake success:

```
NOT_CONFIGURED   -- credentials/config missing
NOT_SUPPORTED    -- this provider/version does not support the operation
AVAILABLE        -- ready to use
DEGRADED         -- reachable but failing health checks / partial functionality
```

The UI and API surface these states directly; nothing is faked to look "done".

## Asynchrony

- **RabbitMQ** carries durable, retryable, long-running work (provisioning, upgrades,
  discovery, GitHub/Argo CD operations) via the workflow engine — see
  [ADR-0005](adr/0005-workflow-architecture.md).
- **Redis** is used only for caching, distributed locks, rate limiting, and short-lived
  job coordination. It is never the system of record.
- **PostgreSQL** is the authoritative store for all application metadata (§24 of the
  product spec / see `migrations/`).

## Secrets

Talos secrets, kubeconfigs, GitHub tokens, and provider credentials are encrypted at
rest and never sent to the browser. See [ADR-0006](adr/0006-secrets-management.md).

## Further reading

- [Roadmap](roadmap.md)
- [ADRs](adr/)

# ADR-0001: Talos Integration via a Dedicated Adapter Layer

## Status
Accepted

## Context
Talos Linux exposes an authenticated gRPC API (`machined`/`talosctl` API) for both
imperative operations (reboot, upgrade, version, health, service/disk/network
inspection) and retrieval/application of machine configuration. Several parts of the
platform need this data: machine management, cluster provisioning, health dashboards,
and the upgrade engine. Without discipline, Talos client calls end up scattered across
handlers, workflows, and background jobs, each with slightly different error handling,
timeouts, and auth.

## Decision
All Talos access goes through a single port defined in the domain/application layer:

```go
type TalosClient interface {
    GetMachineStatus(ctx context.Context, endpoint string) (MachineStatus, error)
    GetMachineConfiguration(ctx context.Context, endpoint string) (MachineConfiguration, error)
    ApplyMachineConfiguration(ctx context.Context, endpoint string, cfg MachineConfiguration, opts ApplyOptions) error
    Reboot(ctx context.Context, endpoint string) error
    Shutdown(ctx context.Context, endpoint string) error
    Upgrade(ctx context.Context, endpoint string, image string, opts UpgradeOptions) error
    GetVersion(ctx context.Context, endpoint string) (VersionInfo, error)
    GetHealth(ctx context.Context, endpoint string) (HealthStatus, error)
    GetServices(ctx context.Context, endpoint string) ([]ServiceInfo, error)
    GetDisks(ctx context.Context, endpoint string) ([]DiskInfo, error)
    GetNetworkInfo(ctx context.Context, endpoint string) (NetworkInfo, error)
    GetEtcdHealth(ctx context.Context, endpoint string) (EtcdHealth, error)
}
```

`internal/integrations/talos` implements this port using the official Talos Go client
(`github.com/siderolabs/talos/pkg/machinery/client`) — done in Phase 2
(`client.go`). A mock implementation (`mock.go`) lives alongside it for local
development and tests, selected via `PLATFORM_TALOS_ADAPTER=mock|real`
(`factory.go`). The real client is scoped to a single Talos cluster's PKI per
process today; per-cluster credential resolution for multi-cluster deployments is
tracked in `docs/roadmap.md` as follow-up work, not yet implemented.

Imperative actions (reboot, health, discovery, logs, version) call this port directly
from the application layer. Declarative desired state (which machines belong to which
cluster, roles, versions) is still stored in Git/Postgres and only *applied* through this
port during provisioning/upgrade workflows — the port is a mechanism, not a second
source of truth.

## Consequences
- One place to add retries, timeouts, circuit breaking, mTLS credential handling, and
  OpenTelemetry spans for Talos calls.
- Swapping or upgrading the Talos client library touches one package.
- Business logic (upgrade planners, workflows) depends only on the interface, so it is
  trivially testable with the mock adapter.
- Talos credentials (PKI, certs) never leave the backend process, and never reach the
  browser (§48). Today they're loaded from a talosconfig file
  (`PLATFORM_TALOS_CONFIG_FILE`) at process startup; `LoadClientFromSecretStore` exists
  for loading from the SecretStore (ADR-0006) instead and is the intended production
  path once credential upload/management is built.

# Cluster API Integration

See [ADR-0002](../adr/0002-cluster-api-integration.md).

Clusters with `ProviderMode: CLUSTER_API` render CAPI manifests through
`ports.ClusterAPIProvider` (`internal/application/ports/clusterapi.go`). Two
implementations exist, selected via `PLATFORM_CLUSTERAPI_ADAPTER`:

- `internal/integrations/clusterapi.MockProvider` (default) — a minimal, deterministic
  Cluster + MachineDeployment renderer and always-Ready status, for local development.
- `internal/integrations/clusterapi.Client` (`PLATFORM_CLUSTERAPI_ADAPTER=real` +
  `PLATFORM_CLUSTERAPI_KUBECONFIG_FILE`) — the real implementation.

The platform never reconciles CAPI resources itself: it generates manifests, commits
them via the GitOps pipeline, and observes status read-only against the management
cluster.

## What the real client renders

- `Cluster` and `MachineDeployment` (`cluster.x-k8s.io/v1beta1`) — core CAPI, a stable,
  well-documented upstream schema.
- `TalosControlPlane` (`controlplane.cluster.x-k8s.io/v1alpha3`) and
  `TalosConfigTemplate` (`bootstrap.cluster.x-k8s.io/v1alpha3`) — from Sidero Labs'
  Cluster API Control Plane/Bootstrap Providers for Talos (CACPPT/CABPT). These two
  CRDs are less universally documented than core CAPI; treat the rendered fields as a
  starting point to verify against the CACPPT/CABPT version actually installed in your
  management cluster, not a guarantee of exact fidelity.

Provider-specific infrastructure CRs (what a real `infrastructureRef` would point at)
are deliberately **not** rendered: no infrastructure provider in this codebase is
CAPI-aware yet (Phase 6 is still mock-only for bare metal/Proxmox), so fabricating a
specific infra CRD shape would be more misleading than useful.

## What the real client observes

`GetClusterStatus` uses a real `k8s.io/client-go` dynamic client against the CAPI
management cluster's Kubernetes API: it reads the `Cluster` resource and every
`MachineDeployment`/`Machine`/`MachineHealthCheck` labeled
`cluster.x-k8s.io/cluster-name=<name>` (the label CAPI itself applies), read-only. It
never writes anything. `MachineHealthCheck` status (`currentHealthy`/
`expectedMachines`/`remediationsAllowed`) surfaces whether CAPI's own MHC controller
is currently allowed to remediate — the platform observes this, it never triggers
remediation itself (ADR-0002).

## Where this gets called from

`internal/workflows/capi_change.go`'s `commitClusterAPIChange`/`waitForClusterAPIReady`
are shared by the cluster provisioning, `CLUSTER_UPGRADE`, and `WORKER_SCALE`
workflows: for a `CLUSTER_API`-mode cluster, an upgrade or scale request renders the
new manifests and runs them through the same commit -> PR -> merge -> (Argo CD sync)
pipeline as provisioning, rather than doing anything directly. `DIRECT_TALOS`-mode
clusters skip this path entirely and use `TalosClient` directly instead (ADR-0001).

## What's not built yet

- **Remediation orchestration** — MachineHealthCheck status is observed (see above),
  but the platform doesn't and won't initiate replacing an unhealthy machine itself;
  that's CAPI's own MHC controller's job (ADR-0002). There is no platform-initiated
  "replace this machine" action, by design.
- **ApplicationSet-style multi-cluster templating** is out of scope here — see
  [../argocd/README.md](../argocd/README.md).

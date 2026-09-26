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
  Cluster API Control Plane/Bootstrap Providers for Talos (CACPPT/CABPT).
- `ProxmoxCluster` and `ProxmoxMachineTemplate`
  (`infrastructure.cluster.x-k8s.io/v1alpha1`) — from
  [cluster-api-provider-proxmox](https://github.com/ionos-cloud/cluster-api-provider-proxmox)
  ("CAPMOX") — **only when Proxmox infrastructure is configured** (see below), wired
  into `Cluster.spec.infrastructureRef`, `TalosControlPlane.spec.infrastructureTemplate`,
  and each worker pool's `MachineDeployment` template `infrastructureRef`. When not
  configured, these fields are simply omitted, exactly as before this existed.

These four CRDs are all less universally documented/stable than core CAPI; treat the
rendered fields as a starting point to verify against the CACPPT/CABPT/CAPMOX version
actually installed in your management cluster, not a guarantee of exact fidelity.

## Proxmox infrastructure CRs

Set via `internal/integrations/clusterapi.ProxmoxInfrastructure`, populated in
`cmd/platform-api/main.go` and `cmd/platform-worker/main.go` from the **same** global
Proxmox configuration the real `ports.InfrastructureProvider` client already uses
(`PLATFORM_PROXMOX_API_URL`, `PLATFORM_PROXMOX_NODE`, `PLATFORM_PROXMOX_TEMPLATE_VMID`)
whenever `PLATFORM_PROXMOX_ADAPTER=real` — the same "Phase 2-6 bootstrapping
simplification, one process-wide instance from global config, not a per-organization
`infraprovider.InfrastructureProvider` record" pattern documented in
[../infrastructure/README.md](../infrastructure/README.md), not a new inconsistency.

`ProxmoxCluster.spec.credentialsRef` points at a Kubernetes Secret named
`<cluster-name>-proxmox-credentials` by default — **the platform does not create
this Secret**. Unlike GitOps-committed manifests, a live credential Secret in the
management cluster is provisioned out of band by the operator, the same way every
other CAPI infrastructure provider's credentials are (this is standard CAPI operator
setup, not specific to this platform).

Bare metal has no comparably established CAPI infrastructure provider to target, so
it remains CAPI-unaware — `DIRECT_TALOS` mode is the path for bare-metal clusters.

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

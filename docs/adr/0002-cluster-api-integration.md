# ADR-0002: Cluster API as an Optional Provider Mode, Not the Only Path

## Status
Accepted

## Context
Cluster API (CAPI) gives a strong declarative lifecycle model for Kubernetes clusters,
but it requires a management cluster and CAPI controllers, and it is not universally the
right fit — a two-site bare-metal edge deployment may prefer direct Talos bootstrap
without the operational overhead of running CAPI. Forcing every cluster through CAPI
would violate the "don't create a competing control plane, but also don't force a tool
where it doesn't fit" principle.

## Decision
Every cluster record has a `provider_mode` of either `DIRECT_TALOS` or `CLUSTER_API`,
chosen at cluster-creation time based on the infrastructure provider and operator
preference (a cluster template can pin one).

- `DIRECT_TALOS`: the platform's cluster lifecycle workflow talks to Talos machines
  directly through the `TalosClient` port to bootstrap etcd, the control plane, and join
  workers. No CAPI resources are created.
- `CLUSTER_API`: the platform's job is reduced to *generating* CAPI manifests (Cluster,
  KubeadmControlPlane or Talos-native control plane resource, MachineDeployment,
  infrastructure-provider-specific CRs) and committing them to Git. Argo CD syncs these
  manifests into the CAPI management cluster; CAPI controllers do all reconciliation,
  scaling, remediation, and machine replacement.

The platform never runs its own reconciliation loop that duplicates what CAPI
controllers do. It only: generates manifests, observes CAPI resource status (read-only,
via the Kubernetes API against the management cluster), and surfaces that status in the
UI/API.

## Consequences
- Two cluster lifecycle code paths exist, selected by `provider_mode`; both implement
  the same `ClusterLifecycle` application-layer interface so the REST API and workflows
  are provider-agnostic.
- CAPI support can be added incrementally (Phase 5) without blocking Phase 2 (direct
  Talos) delivery.
- Operators can choose the simplest model that fits their site instead of being forced
  into CAPI's operational footprint everywhere.

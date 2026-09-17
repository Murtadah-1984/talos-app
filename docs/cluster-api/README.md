# Cluster API Integration

See [ADR-0002](../adr/0002-cluster-api-integration.md).

Clusters with `ProviderMode: CLUSTER_API` render CAPI manifests through
`ports.ClusterAPIProvider` (`internal/application/ports/clusterapi.go`), backed today
by `internal/integrations/clusterapi.MockProvider`. The platform never reconciles
CAPI resources itself — it generates manifests, commits them via the GitOps pipeline,
and observes status read-only against the management cluster.

Real CAPI manifest generation (Cluster, KubeadmControlPlane or a Talos-native control
plane resource, MachineDeployment, infrastructure-provider CRs) is Phase 5 — see
[../roadmap.md](../roadmap.md).

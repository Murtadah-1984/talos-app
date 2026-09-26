// Package ports defines every interface the application layer uses to reach
// external systems. Concrete implementations live under internal/integrations
// and internal/infrastructure; the application layer and domain never import
// them directly (ADR-0001, ADR-0006, ADR-0007).
package ports

import (
	"context"
	"time"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// MachineStatus is a point-in-time snapshot returned by GetMachineStatus.
type MachineStatus struct {
	Hostname     string
	TalosVersion string
	Uptime       time.Duration
	Ready        bool
}

// MachineConfiguration is the (redacted-for-transport) Talos machine config.
type MachineConfiguration struct {
	Cluster    string
	Role       string
	RawYAML    string
	AppliedRev string
}

// ApplyOptions controls how a machine configuration is applied.
type ApplyOptions struct {
	Mode   string // "reboot", "no-reboot", "staged"
	DryRun bool
}

// UpgradeOptions controls a Talos upgrade.
type UpgradeOptions struct {
	Image    string
	Preserve bool
	Stage    bool
	Force    bool
}

// VersionInfo reports Talos + Kubernetes versions observed on a machine.
type VersionInfo struct {
	TalosVersion      string
	KubernetesVersion string
}

// HealthStatus is the aggregate Talos health check result for a machine.
type HealthStatus struct {
	Healthy bool
	Details map[string]string
}

// EtcdHealth reports etcd member health for a control-plane machine.
type EtcdHealth struct {
	MemberID string
	Healthy  bool
	IsLeader bool
}

// ServiceInfo describes a single Talos-managed service (e.g. kubelet, etcd).
type ServiceInfo struct {
	Name    string
	State   string
	Healthy bool
}

// DiskInfo describes a disk visible to Talos.
type DiskInfo struct {
	Device    string
	SizeBytes int64
	Model     string
}

// NetworkInfo describes host networking as seen by Talos.
type NetworkInfo struct {
	Hostname   string
	Addresses  []string
	Interfaces []string
}

// ClusterMember is one node of a Talos cluster's membership, as reported by
// Talos's own discovery service (queried from a single already-known node,
// not from the platform's own machine records).
type ClusterMember struct {
	Hostname        string
	Addresses       []string
	ControlPlane    bool
	OperatingSystem string
}

// TalosClient is the single adapter boundary for all Talos API access
// (ADR-0001). Every method takes the machine's management endpoint; PKI
// material is resolved internally from the SecretStore, never passed in.
type TalosClient interface {
	GetMachineStatus(ctx context.Context, endpoint string) (MachineStatus, error)
	GetMachineConfiguration(ctx context.Context, endpoint string) (MachineConfiguration, error)
	ApplyMachineConfiguration(ctx context.Context, endpoint string, cfg MachineConfiguration, opts ApplyOptions) error
	// ApplyMaintenanceConfiguration pushes an initial configuration to a
	// freshly-booted node that has no Talos PKI trust yet — Talos's
	// "maintenance mode", an insecure, pre-PKI connection accepted only
	// because there is no cluster identity to authenticate against before
	// this call establishes one. fingerprint, when non-empty, pins the TLS
	// certificate fingerprint the node's maintenance-mode API is expected to
	// present (from `talosctl get certificate` or the platform's own
	// bare-metal/Proxmox provisioning record of the boot image), mitigating
	// MITM during this one bootstrap window; empty accepts any certificate,
	// matching Talos's own default posture since there is nothing to verify
	// against yet on a truly unknown node.
	ApplyMaintenanceConfiguration(ctx context.Context, endpoint, rawYAML, fingerprint string) error
	Reboot(ctx context.Context, endpoint string) error
	Shutdown(ctx context.Context, endpoint string) error
	Upgrade(ctx context.Context, endpoint string, opts UpgradeOptions) error
	GetVersion(ctx context.Context, endpoint string) (VersionInfo, error)
	GetHealth(ctx context.Context, endpoint string) (HealthStatus, error)
	GetServices(ctx context.Context, endpoint string) ([]ServiceInfo, error)
	GetDisks(ctx context.Context, endpoint string) ([]DiskInfo, error)
	GetNetworkInfo(ctx context.Context, endpoint string) (NetworkInfo, error)
	GetEtcdHealth(ctx context.Context, endpoint string) (EtcdHealth, error)
	// DiscoverClusterMembers queries endpoint for every member of its Talos
	// cluster (control plane and worker), via Talos's own discovery-service-
	// backed membership resource — letting the platform infer an existing
	// cluster's topology purely from one already-known node, for onboarding
	// a cluster it didn't provision itself (DIRECT_TALOS mode).
	DiscoverClusterMembers(ctx context.Context, endpoint string) ([]ClusterMember, error)
	// EnsureCredentials registers the Talos PKI (talosconfigYAML) to use for
	// endpoint, so every other method called with that same endpoint uses
	// these credentials instead of the adapter's single default talosconfig
	// — this is what lets one platform process reach more than one Talos
	// cluster's machines. Idempotent: registering the same endpoint again
	// (e.g. after credential rotation) just replaces what's stored.
	// Endpoints never explicitly registered fall back to the default
	// talosconfig the adapter was constructed with, so single-cluster
	// deployments (the common case) never need to call this at all.
	EnsureCredentials(ctx context.Context, endpoint string, talosconfigYAML []byte) error

	// Capability reports whether this adapter is backed by a real Talos
	// endpoint or is a mock/degraded placeholder (see capability states).
	Capability() shared.CapabilityState
}

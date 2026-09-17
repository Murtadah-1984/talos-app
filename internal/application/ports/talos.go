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

// TalosClient is the single adapter boundary for all Talos API access
// (ADR-0001). Every method takes the machine's management endpoint; PKI
// material is resolved internally from the SecretStore, never passed in.
type TalosClient interface {
	GetMachineStatus(ctx context.Context, endpoint string) (MachineStatus, error)
	GetMachineConfiguration(ctx context.Context, endpoint string) (MachineConfiguration, error)
	ApplyMachineConfiguration(ctx context.Context, endpoint string, cfg MachineConfiguration, opts ApplyOptions) error
	Reboot(ctx context.Context, endpoint string) error
	Shutdown(ctx context.Context, endpoint string) error
	Upgrade(ctx context.Context, endpoint string, opts UpgradeOptions) error
	GetVersion(ctx context.Context, endpoint string) (VersionInfo, error)
	GetHealth(ctx context.Context, endpoint string) (HealthStatus, error)
	GetServices(ctx context.Context, endpoint string) ([]ServiceInfo, error)
	GetDisks(ctx context.Context, endpoint string) ([]DiskInfo, error)
	GetNetworkInfo(ctx context.Context, endpoint string) (NetworkInfo, error)
	GetEtcdHealth(ctx context.Context, endpoint string) (EtcdHealth, error)

	// Capability reports whether this adapter is backed by a real Talos
	// endpoint or is a mock/degraded placeholder (see capability states).
	Capability() shared.CapabilityState
}

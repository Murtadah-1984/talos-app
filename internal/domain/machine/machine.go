// Package machine models a physical or virtual machine running Talos Linux,
// independent of which infrastructure provider created it.
package machine

import (
	"context"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// Role is the Talos/Kubernetes role a machine plays in its cluster.
type Role string

const (
	RoleControlPlane Role = "CONTROL_PLANE"
	RoleWorker       Role = "WORKER"
)

// Phase is the observed lifecycle phase of a machine.
type Phase string

const (
	PhaseDiscovered   Phase = "DISCOVERED"
	PhaseProvisioning Phase = "PROVISIONING"
	PhaseInstalling   Phase = "INSTALLING"
	PhaseReady        Phase = "READY"
	PhaseUpgrading    Phase = "UPGRADING"
	PhaseDraining     Phase = "DRAINING"
	PhaseCordoned     Phase = "CORDONED"
	PhaseUnhealthy    Phase = "UNHEALTHY"
	PhaseDeleting     Phase = "DELETING"
	PhaseDeleted      Phase = "DELETED"
)

// BMCProtocol is the out-of-band management protocol available on a machine,
// decoupled from Talos concerns per ADR-0007.
type BMCProtocol string

const (
	BMCNone    BMCProtocol = "NONE"
	BMCIPMI    BMCProtocol = "IPMI"
	BMCRedfish BMCProtocol = "REDFISH"
)

// BMC describes out-of-band management access to a bare-metal machine.
type BMC struct {
	Protocol BMCProtocol
	Address  string
	// CredentialRef points at a secret in the SecretStore; never a plaintext value.
	CredentialRef string
}

// Interface is a network interface on a machine.
type Interface struct {
	ID         shared.ID
	MachineID  shared.ID
	Name       string
	MACAddress string
	DHCP       bool
	Addresses  []string
	VLANs      []int
}

// Disk describes a storage device discovered on a machine.
type Disk struct {
	ID        shared.ID
	MachineID shared.ID
	Device    string
	SizeBytes int64
	Model     string
	IsSystem  bool
}

// Machine is a physical or virtual node, independent of provider or cluster.
type Machine struct {
	ID                shared.ID
	ClusterID         *shared.ID // nil until assigned to a cluster
	SiteID            shared.ID
	ProviderID        shared.ID // infraprovider.InfrastructureProvider
	ProviderMachineID string    // the provider's own identifier (e.g. a Proxmox VMID) — never the platform's own ID
	Hostname          string
	ManagementIP      string
	Role              Role
	Phase             Phase
	TalosVersion      string
	KubernetesVersion string
	CPU               int32
	MemoryBytes       int64
	BMC               *BMC
	Labels            map[string]string
	shared.Timestamps
}

// Repository persists machines and their interfaces/disks.
type Repository interface {
	Create(ctx context.Context, m *Machine) error
	Get(ctx context.Context, id shared.ID) (*Machine, error)
	List(ctx context.Context, filter Filter, page shared.Page) ([]*Machine, error)
	Update(ctx context.Context, m *Machine) error
	Delete(ctx context.Context, id shared.ID) error

	SetInterfaces(ctx context.Context, machineID shared.ID, ifaces []Interface) error
	SetDisks(ctx context.Context, machineID shared.ID, disks []Disk) error
}

// Filter narrows a machine list query. Zero values mean "no filter".
type Filter struct {
	ClusterID  *shared.ID
	SiteID     *shared.ID
	ProviderID *shared.ID
	Role       Role
	Phase      Phase
}

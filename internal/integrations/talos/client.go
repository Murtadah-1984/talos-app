// Package talos implements the ports.TalosClient adapter (ADR-0001). This
// file is the Phase 2 real implementation, backed by the official Talos Go
// client (github.com/siderolabs/talos/pkg/machinery/client). See mock.go for
// the deterministic stand-in used elsewhere in local development.
//
// LIMITATION (tracked in docs/roadmap.md): every method here is keyed only
// by machine endpoint, matching the ports.TalosClient interface, but a real
// multi-cluster deployment needs one credential set (talosconfig) per Talos
// cluster. Client is therefore scoped to a single cluster's PKI for now —
// multi-cluster credential resolution (per-cluster SecretRef lookup keyed by
// endpoint) is a follow-up, not yet threaded through the port interface.
package talos

import (
	"context"
	"fmt"

	cosiresource "github.com/cosi-project/runtime/pkg/resource"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	tclient "github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	configmachine "github.com/siderolabs/talos/pkg/machinery/config/machine"
	"github.com/siderolabs/talos/pkg/machinery/resources/cluster"
	configres "github.com/siderolabs/talos/pkg/machinery/resources/config"
	"github.com/siderolabs/talos/pkg/machinery/resources/network"
	"github.com/siderolabs/talos/pkg/machinery/resources/runtime"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// Client is the real ports.TalosClient adapter. Every method dials directly
// to the given endpoint (rather than proxying through another node), so
// results always describe that specific machine.
type Client struct {
	cfg *clientconfig.Config
}

// NewClient parses a talosconfig YAML document (as produced by `talosctl
// config` or a cluster's generated PKI bundle) and returns a Client that can
// reach any node trusted by that config's current context.
func NewClient(talosconfigYAML []byte) (*Client, error) {
	cfg, err := clientconfig.FromBytes(talosconfigYAML)
	if err != nil {
		return nil, fmt.Errorf("parsing talosconfig: %w", err)
	}
	return &Client{cfg: cfg}, nil
}

// LoadClientFromSecretStore resolves a talosconfig from the platform's
// SecretStore (ADR-0006) and builds a Client from it, so the raw PKI
// material never needs to pass through application code as a bare byte
// slice outside this one call site.
func LoadClientFromSecretStore(ctx context.Context, store ports.SecretStore, ref ports.SecretRef) (*Client, error) {
	data, err := store.Get(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("loading talosconfig from secret store: %w", err)
	}
	return NewClient(data)
}

func (c *Client) dial(ctx context.Context, endpoint string) (*tclient.Client, error) {
	cli, err := tclient.New(ctx, tclient.WithConfig(c.cfg), tclient.WithEndpoints(endpoint),
		tclient.WithGRPCDialOptions(grpc.WithStatsHandler(otelgrpc.NewClientHandler())))
	if err != nil {
		return nil, fmt.Errorf("connecting to Talos endpoint %s: %w", endpoint, err)
	}
	return cli, nil
}

func (c *Client) Capability() shared.CapabilityState {
	return shared.CapabilityAvailable
}

func (c *Client) GetMachineStatus(ctx context.Context, endpoint string) (ports.MachineStatus, error) {
	cli, err := c.dial(ctx, endpoint)
	if err != nil {
		return ports.MachineStatus{}, err
	}
	defer func() { _ = cli.Close() }()

	hostname, err := hostnameOf(ctx, cli)
	if err != nil {
		return ports.MachineStatus{}, err
	}

	verResp, err := cli.Version(ctx)
	if err != nil {
		return ports.MachineStatus{}, fmt.Errorf("getting version from %s: %w", endpoint, err)
	}
	var talosVersion string
	if len(verResp.Messages) > 0 && verResp.Messages[0].Version != nil {
		talosVersion = verResp.Messages[0].Version.Tag
	}

	ready, err := machineReady(ctx, cli)
	if err != nil {
		return ports.MachineStatus{}, err
	}

	// Uptime is not yet wired to a boot-time resource query — left zero
	// rather than approximated, so callers don't mistake it for real data.
	return ports.MachineStatus{Hostname: hostname, TalosVersion: talosVersion, Ready: ready}, nil
}

func (c *Client) GetMachineConfiguration(ctx context.Context, endpoint string) (ports.MachineConfiguration, error) {
	cli, err := c.dial(ctx, endpoint)
	if err != nil {
		return ports.MachineConfiguration{}, err
	}
	defer func() { _ = cli.Close() }()

	res, err := cli.COSI.Get(ctx, cosiresource.NewMetadata(configres.NamespaceName, configres.MachineConfigType, configres.ActiveID, cosiresource.VersionUndefined))
	if err != nil {
		return ports.MachineConfiguration{}, fmt.Errorf("getting machine configuration resource from %s: %w", endpoint, err)
	}
	mc, ok := res.(*configres.MachineConfig)
	if !ok {
		return ports.MachineConfiguration{}, fmt.Errorf("unexpected resource type %T for machine configuration", res)
	}

	raw, err := mc.Container().Bytes()
	if err != nil {
		return ports.MachineConfiguration{}, fmt.Errorf("encoding machine configuration: %w", err)
	}

	return ports.MachineConfiguration{
		Cluster:    cli.GetClusterName(),
		Role:       mc.Provider().Machine().Type().String(),
		RawYAML:    string(raw),
		AppliedRev: res.Metadata().Version().String(),
	}, nil
}

func (c *Client) ApplyMachineConfiguration(ctx context.Context, endpoint string, cfg ports.MachineConfiguration, opts ports.ApplyOptions) error {
	cli, err := c.dial(ctx, endpoint)
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()

	req := &machineapi.ApplyConfigurationRequest{
		Data:   []byte(cfg.RawYAML),
		Mode:   applyModeFor(opts.Mode),
		DryRun: opts.DryRun,
	}
	if _, err := cli.ApplyConfiguration(ctx, req); err != nil {
		return fmt.Errorf("applying machine configuration to %s: %w", endpoint, err)
	}
	return nil
}

// maintenanceFingerprints adapts a single optional fingerprint string into
// the slice WithMaintenanceMode expects, so an empty fingerprint means
// "accept any certificate" rather than a slice containing one empty entry.
func maintenanceFingerprints(fingerprint string) []string {
	if fingerprint == "" {
		return nil
	}
	return []string{fingerprint}
}

func (c *Client) ApplyMaintenanceConfiguration(ctx context.Context, endpoint, rawYAML, fingerprint string) error {
	cli, err := tclient.New(ctx,
		tclient.WithMaintenanceMode(endpoint, maintenanceFingerprints(fingerprint)),
		tclient.WithGRPCDialOptions(grpc.WithStatsHandler(otelgrpc.NewClientHandler())))
	if err != nil {
		return fmt.Errorf("connecting to Talos maintenance-mode endpoint %s: %w", endpoint, err)
	}
	defer func() { _ = cli.Close() }()

	if _, err := cli.ApplyConfiguration(ctx, &machineapi.ApplyConfigurationRequest{
		Data: []byte(rawYAML),
		Mode: machineapi.ApplyConfigurationRequest_AUTO,
	}); err != nil {
		return fmt.Errorf("applying maintenance-mode configuration to %s: %w", endpoint, err)
	}
	return nil
}

func applyModeFor(mode string) machineapi.ApplyConfigurationRequest_Mode {
	switch mode {
	case "no-reboot":
		return machineapi.ApplyConfigurationRequest_NO_REBOOT
	case "staged":
		return machineapi.ApplyConfigurationRequest_STAGED
	default:
		return machineapi.ApplyConfigurationRequest_AUTO
	}
}

func (c *Client) Reboot(ctx context.Context, endpoint string) error {
	cli, err := c.dial(ctx, endpoint)
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()

	if err := cli.Reboot(ctx); err != nil {
		return fmt.Errorf("rebooting %s: %w", endpoint, err)
	}
	return nil
}

func (c *Client) Shutdown(ctx context.Context, endpoint string) error {
	cli, err := c.dial(ctx, endpoint)
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()

	if err := cli.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutting down %s: %w", endpoint, err)
	}
	return nil
}

func (c *Client) Upgrade(ctx context.Context, endpoint string, opts ports.UpgradeOptions) error {
	cli, err := c.dial(ctx, endpoint)
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()

	// UpgradeWithOptions is marked deprecated in favor of LifecycleClient's
	// streaming Install/Upgrade RPCs, but that API has no equivalent for
	// Preserve/Stage/Force and is still new enough that talosctl and the
	// rest of the ecosystem use this RPC. Revisit once LifecycleClient
	// stabilizes and ports.UpgradeOptions can be redesigned around it.
	_, err = cli.UpgradeWithOptions(ctx, //nolint:staticcheck // see comment above
		tclient.WithUpgradeImage(opts.Image),
		tclient.WithUpgradePreserve(opts.Preserve),
		tclient.WithUpgradeStage(opts.Stage),
		tclient.WithUpgradeForce(opts.Force),
	)
	if err != nil {
		return fmt.Errorf("upgrading %s: %w", endpoint, err)
	}
	return nil
}

func (c *Client) GetVersion(ctx context.Context, endpoint string) (ports.VersionInfo, error) {
	cli, err := c.dial(ctx, endpoint)
	if err != nil {
		return ports.VersionInfo{}, err
	}
	defer func() { _ = cli.Close() }()

	resp, err := cli.Version(ctx)
	if err != nil {
		return ports.VersionInfo{}, fmt.Errorf("getting version from %s: %w", endpoint, err)
	}
	var talosVersion string
	if len(resp.Messages) > 0 && resp.Messages[0].Version != nil {
		talosVersion = resp.Messages[0].Version.Tag
	}
	// Kubernetes version is not exposed by the Talos API itself (it's a
	// Kubernetes API concern) — left blank rather than guessed; the
	// Kubernetes integration (internal/integrations/kubernetes, not yet
	// built) is the correct source for it.
	return ports.VersionInfo{TalosVersion: talosVersion}, nil
}

func (c *Client) GetHealth(ctx context.Context, endpoint string) (ports.HealthStatus, error) {
	cli, err := c.dial(ctx, endpoint)
	if err != nil {
		return ports.HealthStatus{}, err
	}
	defer func() { _ = cli.Close() }()

	ready, err := machineReady(ctx, cli)
	if err != nil {
		return ports.HealthStatus{}, err
	}

	details := map[string]string{"ready": fmt.Sprintf("%t", ready)}

	if svcResp, svcErr := cli.ServiceList(ctx); svcErr == nil {
		for _, msg := range svcResp.Messages {
			for _, svc := range msg.Services {
				if svc.Health != nil {
					details[svc.Id] = svc.State
				}
			}
		}
	}

	return ports.HealthStatus{Healthy: ready, Details: details}, nil
}

func (c *Client) GetServices(ctx context.Context, endpoint string) ([]ports.ServiceInfo, error) {
	cli, err := c.dial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cli.Close() }()

	resp, err := cli.ServiceList(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing services on %s: %w", endpoint, err)
	}

	var out []ports.ServiceInfo
	for _, msg := range resp.Messages {
		for _, svc := range msg.Services {
			healthy := svc.Health != nil && svc.Health.Healthy
			out = append(out, ports.ServiceInfo{Name: svc.Id, State: svc.State, Healthy: healthy})
		}
	}
	return out, nil
}

func (c *Client) GetDisks(ctx context.Context, endpoint string) ([]ports.DiskInfo, error) {
	cli, err := c.dial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cli.Close() }()

	resp, err := cli.Disks(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing disks on %s: %w", endpoint, err)
	}

	var out []ports.DiskInfo
	for _, msg := range resp.Messages {
		for _, d := range msg.Disks {
			out = append(out, ports.DiskInfo{Device: d.DeviceName, SizeBytes: int64(d.Size), Model: d.Model})
		}
	}
	return out, nil
}

func (c *Client) GetNetworkInfo(ctx context.Context, endpoint string) (ports.NetworkInfo, error) {
	cli, err := c.dial(ctx, endpoint)
	if err != nil {
		return ports.NetworkInfo{}, err
	}
	defer func() { _ = cli.Close() }()

	hostname, err := hostnameOf(ctx, cli)
	if err != nil {
		return ports.NetworkInfo{}, err
	}

	list, err := cli.COSI.List(ctx, cosiresource.NewMetadata(network.NamespaceName, network.AddressStatusType, "", cosiresource.VersionUndefined))
	if err != nil {
		return ports.NetworkInfo{}, fmt.Errorf("listing addresses on %s: %w", endpoint, err)
	}

	var addresses []string
	interfaceSet := map[string]struct{}{}
	for _, item := range list.Items {
		as, ok := item.(*network.AddressStatus)
		if !ok {
			continue
		}
		spec := as.TypedSpec()
		addresses = append(addresses, spec.Address.String())
		if spec.LinkName != "" {
			interfaceSet[spec.LinkName] = struct{}{}
		}
	}

	interfaces := make([]string, 0, len(interfaceSet))
	for name := range interfaceSet {
		interfaces = append(interfaces, name)
	}

	return ports.NetworkInfo{Hostname: hostname, Addresses: addresses, Interfaces: interfaces}, nil
}

func (c *Client) GetEtcdHealth(ctx context.Context, endpoint string) (ports.EtcdHealth, error) {
	cli, err := c.dial(ctx, endpoint)
	if err != nil {
		return ports.EtcdHealth{}, err
	}
	defer func() { _ = cli.Close() }()

	resp, err := cli.EtcdStatus(ctx)
	if err != nil {
		return ports.EtcdHealth{}, fmt.Errorf("getting etcd status from %s: %w", endpoint, err)
	}
	if len(resp.Messages) == 0 || resp.Messages[0].MemberStatus == nil {
		return ports.EtcdHealth{}, fmt.Errorf("no etcd member status returned by %s", endpoint)
	}

	member := resp.Messages[0].MemberStatus
	return ports.EtcdHealth{
		MemberID: fmt.Sprintf("%d", member.MemberId),
		Healthy:  len(member.Errors) == 0,
		IsLeader: member.MemberId == member.Leader,
	}, nil
}

func (c *Client) DiscoverClusterMembers(ctx context.Context, endpoint string) ([]ports.ClusterMember, error) {
	cli, err := c.dial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cli.Close() }()

	list, err := cli.COSI.List(ctx, cosiresource.NewMetadata(cluster.NamespaceName, cluster.MemberType, "", cosiresource.VersionUndefined))
	if err != nil {
		return nil, fmt.Errorf("listing cluster members from %s: %w", endpoint, err)
	}

	out := make([]ports.ClusterMember, 0, len(list.Items))
	for _, item := range list.Items {
		m, ok := item.(*cluster.Member)
		if !ok {
			continue
		}
		spec := m.TypedSpec()
		addrs := make([]string, 0, len(spec.Addresses))
		for _, a := range spec.Addresses {
			addrs = append(addrs, a.String())
		}
		out = append(out, ports.ClusterMember{
			Hostname:        spec.Hostname,
			Addresses:       addrs,
			ControlPlane:    spec.MachineType == configmachine.TypeControlPlane,
			OperatingSystem: spec.OperatingSystem,
		})
	}
	return out, nil
}

// hostnameOf queries the network.HostnameStatus COSI resource directly
// rather than going through a dedicated RPC, since Talos exposes host
// identity as a resource rather than a single-purpose API call.
func hostnameOf(ctx context.Context, cli *tclient.Client) (string, error) {
	res, err := cli.COSI.Get(ctx, cosiresource.NewMetadata(network.NamespaceName, network.HostnameStatusType, network.HostnameID, cosiresource.VersionUndefined))
	if err != nil {
		return "", fmt.Errorf("getting hostname resource: %w", err)
	}
	hs, ok := res.(*network.HostnameStatus)
	if !ok {
		return "", fmt.Errorf("unexpected resource type %T for hostname", res)
	}
	return hs.TypedSpec().Hostname, nil
}

// machineReady queries the runtime.MachineStatus COSI resource, which is
// Talos's own aggregated readiness signal for the node.
func machineReady(ctx context.Context, cli *tclient.Client) (bool, error) {
	res, err := cli.COSI.Get(ctx, cosiresource.NewMetadata(runtime.NamespaceName, runtime.MachineStatusType, runtime.MachineStatusID, cosiresource.VersionUndefined))
	if err != nil {
		return false, fmt.Errorf("getting machine status resource: %w", err)
	}
	ms, ok := res.(*runtime.MachineStatus)
	if !ok {
		return false, fmt.Errorf("unexpected resource type %T for machine status", res)
	}
	return ms.TypedSpec().Status.Ready, nil
}

var _ ports.TalosClient = (*Client)(nil)

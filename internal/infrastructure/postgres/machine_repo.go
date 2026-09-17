package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talos-platform/talos-platform/internal/domain/machine"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

type MachineRepository struct {
	pool *pgxpool.Pool
}

func NewMachineRepository(pool *pgxpool.Pool) *MachineRepository {
	return &MachineRepository{pool: pool}
}

const machineColumns = `id, cluster_id, site_id, provider_id, hostname, management_ip, role, phase,
	talos_version, kubernetes_version, cpu, memory_bytes, bmc_protocol, bmc_address, bmc_credential_ref,
	labels, created_at, updated_at`

func (r *MachineRepository) Create(ctx context.Context, m *machine.Machine) error {
	m.Touch()
	if m.ID == (shared.ID{}) {
		m.ID = shared.NewID()
	}
	labelsJSON, err := json.Marshal(m.Labels)
	if err != nil {
		return fmt.Errorf("marshaling machine labels: %w", err)
	}
	bmcProto, bmcAddr, bmcCred := machine.BMCNone, "", ""
	if m.BMC != nil {
		bmcProto, bmcAddr, bmcCred = m.BMC.Protocol, m.BMC.Address, m.BMC.CredentialRef
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO machines (id, cluster_id, site_id, provider_id, hostname, management_ip, role, phase,
			talos_version, kubernetes_version, cpu, memory_bytes, bmc_protocol, bmc_address, bmc_credential_ref,
			labels, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		m.ID, m.ClusterID, m.SiteID, m.ProviderID, m.Hostname, m.ManagementIP, m.Role, m.Phase,
		m.TalosVersion, m.KubernetesVersion, m.CPU, m.MemoryBytes, bmcProto, bmcAddr, bmcCred,
		labelsJSON, m.CreatedAt, m.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting machine: %w", err)
	}
	return nil
}

func (r *MachineRepository) Get(ctx context.Context, id shared.ID) (*machine.Machine, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+machineColumns+` FROM machines WHERE id = $1`, id)
	return scanMachine(row)
}

func (r *MachineRepository) List(ctx context.Context, filter machine.Filter, page shared.Page) ([]*machine.Machine, error) {
	where := []string{"1=1"}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if filter.ClusterID != nil {
		where = append(where, "cluster_id = "+arg(*filter.ClusterID))
	}
	if filter.SiteID != nil {
		where = append(where, "site_id = "+arg(*filter.SiteID))
	}
	if filter.ProviderID != nil {
		where = append(where, "provider_id = "+arg(*filter.ProviderID))
	}
	if filter.Role != "" {
		where = append(where, "role = "+arg(filter.Role))
	}
	if filter.Phase != "" {
		where = append(where, "phase = "+arg(filter.Phase))
	}
	limitArg, offsetArg := arg(page.Limit), arg(page.Offset)
	query := fmt.Sprintf(`SELECT %s FROM machines WHERE %s ORDER BY created_at DESC LIMIT %s OFFSET %s`,
		machineColumns, strings.Join(where, " AND "), limitArg, offsetArg)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing machines: %w", err)
	}
	defer rows.Close()

	var out []*machine.Machine
	for rows.Next() {
		m, err := scanMachine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *MachineRepository) Update(ctx context.Context, m *machine.Machine) error {
	m.Touch()
	labelsJSON, err := json.Marshal(m.Labels)
	if err != nil {
		return fmt.Errorf("marshaling machine labels: %w", err)
	}
	bmcProto, bmcAddr, bmcCred := machine.BMCNone, "", ""
	if m.BMC != nil {
		bmcProto, bmcAddr, bmcCred = m.BMC.Protocol, m.BMC.Address, m.BMC.CredentialRef
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE machines SET cluster_id=$2, site_id=$3, provider_id=$4, hostname=$5, management_ip=$6, role=$7,
			phase=$8, talos_version=$9, kubernetes_version=$10, cpu=$11, memory_bytes=$12, bmc_protocol=$13,
			bmc_address=$14, bmc_credential_ref=$15, labels=$16, updated_at=$17 WHERE id=$1`,
		m.ID, m.ClusterID, m.SiteID, m.ProviderID, m.Hostname, m.ManagementIP, m.Role, m.Phase,
		m.TalosVersion, m.KubernetesVersion, m.CPU, m.MemoryBytes, bmcProto, bmcAddr, bmcCred,
		labelsJSON, m.UpdatedAt)
	if err != nil {
		return fmt.Errorf("updating machine: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *MachineRepository) Delete(ctx context.Context, id shared.ID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM machines WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting machine: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *MachineRepository) SetInterfaces(ctx context.Context, machineID shared.ID, ifaces []machine.Interface) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM machine_interfaces WHERE machine_id = $1`, machineID); err != nil {
		return fmt.Errorf("clearing machine interfaces: %w", err)
	}
	for _, iface := range ifaces {
		addrJSON, _ := json.Marshal(iface.Addresses)
		vlanJSON, _ := json.Marshal(iface.VLANs)
		if _, err := tx.Exec(ctx,
			`INSERT INTO machine_interfaces (id, machine_id, name, mac_address, dhcp, addresses, vlans)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			shared.NewID(), machineID, iface.Name, iface.MACAddress, iface.DHCP, addrJSON, vlanJSON); err != nil {
			return fmt.Errorf("inserting machine interface: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *MachineRepository) SetDisks(ctx context.Context, machineID shared.ID, disks []machine.Disk) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM machine_disks WHERE machine_id = $1`, machineID); err != nil {
		return fmt.Errorf("clearing machine disks: %w", err)
	}
	for _, d := range disks {
		if _, err := tx.Exec(ctx,
			`INSERT INTO machine_disks (id, machine_id, device, size_bytes, model, is_system)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			shared.NewID(), machineID, d.Device, d.SizeBytes, d.Model, d.IsSystem); err != nil {
			return fmt.Errorf("inserting machine disk: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func scanMachine(row rowScanner) (*machine.Machine, error) {
	m := &machine.Machine{}
	var role, phase, bmcProto, bmcAddr, bmcCred string
	var labelsJSON []byte
	err := row.Scan(&m.ID, &m.ClusterID, &m.SiteID, &m.ProviderID, &m.Hostname, &m.ManagementIP, &role, &phase,
		&m.TalosVersion, &m.KubernetesVersion, &m.CPU, &m.MemoryBytes, &bmcProto, &bmcAddr, &bmcCred,
		&labelsJSON, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning machine: %w", err)
	}
	m.Role = machine.Role(role)
	m.Phase = machine.Phase(phase)
	if bmcProto != "" && bmcProto != string(machine.BMCNone) {
		m.BMC = &machine.BMC{Protocol: machine.BMCProtocol(bmcProto), Address: bmcAddr, CredentialRef: bmcCred}
	}
	if len(labelsJSON) > 0 {
		if err := json.Unmarshal(labelsJSON, &m.Labels); err != nil {
			return nil, fmt.Errorf("unmarshaling machine labels: %w", err)
		}
	}
	return m, nil
}

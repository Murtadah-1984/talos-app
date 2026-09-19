package baremetal

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// commandRunner abstracts process execution so IPMIController is testable
// without a real ipmitool binary or BMC.
type commandRunner interface {
	Run(ctx context.Context, name string, env []string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}
	return cmd.CombinedOutput()
}

// IPMIController implements PowerController by shelling out to `ipmitool`
// (the standard tool for IPMI-over-LAN power control — there is no mature
// pure-Go IPMI stack worth vendoring for this narrow use). The BMC password
// is passed via the IPMI_PASSWORD environment variable and ipmitool's -E
// flag rather than as a command-line argument, so it never appears in the
// process list or shell history.
type IPMIController struct {
	secrets ports.SecretStore
	runner  commandRunner
}

func NewIPMIController(secrets ports.SecretStore) *IPMIController {
	return &IPMIController{secrets: secrets, runner: execRunner{}}
}

func (c *IPMIController) run(ctx context.Context, bmcAddress, credentialRef string, action ...string) error {
	creds, err := resolveCredentials(ctx, c.secrets, credentialRef)
	if err != nil {
		return err
	}
	args := append([]string{"-I", "lanplus", "-H", bmcAddress, "-U", creds.Username, "-E"}, action...)
	out, err := c.runner.Run(ctx, "ipmitool", []string{"IPMI_PASSWORD=" + creds.Password}, args...)
	if err != nil {
		return fmt.Errorf("ipmitool %v against %s failed: %w (output: %s)", action, bmcAddress, err, string(out))
	}
	return nil
}

func (c *IPMIController) PowerOn(ctx context.Context, bmcAddress, credentialRef string) error {
	return c.run(ctx, bmcAddress, credentialRef, "chassis", "power", "on")
}

func (c *IPMIController) PowerOff(ctx context.Context, bmcAddress, credentialRef string) error {
	return c.run(ctx, bmcAddress, credentialRef, "chassis", "power", "off")
}

func (c *IPMIController) Reboot(ctx context.Context, bmcAddress, credentialRef string) error {
	return c.run(ctx, bmcAddress, credentialRef, "chassis", "power", "cycle")
}

func (c *IPMIController) Capability() shared.CapabilityState {
	return shared.CapabilityAvailable
}

var _ PowerController = (*IPMIController)(nil)

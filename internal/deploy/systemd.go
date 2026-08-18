package deploy

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/arglampedakis/orascout/pkg/annotations"
)

// systemctlCmd returns an exec.Cmd that drives systemctl with the right --user
// flag for the requested manager. mgr=="none" is a programming error and panics.
func systemctlCmd(ctx context.Context, mgr annotations.ServiceManager, args ...string) *exec.Cmd {
	switch mgr {
	case annotations.ManagerSystemdUser:
		return exec.CommandContext(ctx, "systemctl", append([]string{"--user"}, args...)...)
	case annotations.ManagerSystemd:
		return exec.CommandContext(ctx, "systemctl", args...)
	default:
		panic(fmt.Sprintf("systemctlCmd called with unsupported manager %q", mgr))
	}
}

// stopService stops unit via the right systemctl. Returns an error only if
// systemctl itself fails; a unit that's already stopped is not an error.
func stopService(ctx context.Context, mgr annotations.ServiceManager, unit string) error {
	if mgr == annotations.ManagerNone {
		return nil
	}
	cmd := systemctlCmd(ctx, mgr, "stop", unit)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl stop %s: %w (%s)", unit, err, string(out))
	}
	return nil
}

// startService starts unit via the right systemctl.
func startService(ctx context.Context, mgr annotations.ServiceManager, unit string) error {
	if mgr == annotations.ManagerNone {
		return nil
	}
	cmd := systemctlCmd(ctx, mgr, "start", unit)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl start %s: %w (%s)", unit, err, string(out))
	}
	return nil
}

package systemupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// checkUpdateSupervisor prevents the per-user updater from interrupting a
// system LaunchDaemon, which it cannot unload or safely replace.
func checkUpdateSupervisor(ctx context.Context) error {
	const plist = "/Library/LaunchDaemons/com.buildworld.server.plist"
	if _, err := os.Stat(plist); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect BuildWorld system LaunchDaemon: %w", err)
	}
	output, err := exec.CommandContext(ctx, "launchctl", "list", "com.buildworld.server").Output()
	if err != nil {
		return fmt.Errorf("cannot verify BuildWorld system LaunchDaemon; administrator must inspect it before updating: %w", err)
	}
	if systemLaunchDaemonMatches(string(output), os.Getpid()) {
		return ErrUnsupportedSupervisor
	}
	return nil
}

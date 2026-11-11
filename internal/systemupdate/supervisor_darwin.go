package systemupdate

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"syscall"
)

// checkUpdateSupervisor permits the system daemon only through the restricted
// root-owned bridge; a user LaunchAgent keeps its existing lifecycle.
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
	if !systemLaunchDaemonMatches(string(output), os.Getpid()) {
		return nil
	}
	for _, path := range []string{plist, systemSupervisorPath} {
		info, statErr := os.Lstat(path)
		if statErr != nil || (path == systemSupervisorPath && info.Size() > 4096) {
			return ErrUnsupportedSupervisor
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !trustedRootFile(info.Mode(), stat.Uid) {
			return ErrUnsupportedSupervisor
		}
	}
	bridge, err := os.ReadFile(systemSupervisorPath)
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(bridge)) != systemSupervisorSHA256 {
		return ErrUnsupportedSupervisor
	}
	current, err := user.Current()
	if err != nil {
		return ErrUnsupportedSupervisor
	}
	values := make(map[string]string)
	for _, key := range []string{"UserName", "RunAtLoad", "KeepAlive"} {
		value, err := exec.CommandContext(ctx, "/usr/libexec/PlistBuddy", "-c", "Print :"+key, plist).Output()
		if err != nil {
			return ErrUnsupportedSupervisor
		}
		values[key] = strings.TrimSpace(string(value))
	}
	if !supportedSystemSupervisorPlist(current.Username, values["UserName"], values["RunAtLoad"], values["KeepAlive"]) {
		return ErrUnsupportedSupervisor
	}
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", systemSupervisorPath)
	command.Stdin = strings.NewReader("status\n")
	status, err := command.Output()
	if err != nil || !systemLaunchDaemonPrintMatches(string(status), os.Getpid()) {
		return ErrUnsupportedSupervisor
	}
	return nil
}

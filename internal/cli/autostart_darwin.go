//go:build darwin

package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

func enableAutostart(server, config string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	paths := macOSAutostartPathsForHome(home)
	launcherDirectory := filepath.Dir(paths.launcher)
	if err := os.MkdirAll(launcherDirectory, 0o700); err != nil {
		return fmt.Errorf("create macOS autostart launcher directory: %w", err)
	}
	if err := os.Chmod(launcherDirectory, 0o700); err != nil {
		return fmt.Errorf("secure macOS autostart launcher directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.plist), 0o700); err != nil {
		return fmt.Errorf("create macOS LaunchAgents directory: %w", err)
	}

	workingDirectory := filepath.Dir(config)
	if err := os.MkdirAll(workingDirectory, 0o700); err != nil {
		return fmt.Errorf("create BuildWorld working directory: %w", err)
	}
	logPath := filepath.Join(workingDirectory, "server.log")
	if err := secureMacOSAutostartLog(logPath); err != nil {
		return fmt.Errorf("secure macOS autostart log: %w", err)
	}
	if err := writeMacOSAutostartFile(paths.launcher, macOSAutostartLauncher(server, config, workingDirectory), 0o700); err != nil {
		return fmt.Errorf("write macOS autostart launcher: %w", err)
	}
	if err := writeMacOSAutostartFile(paths.plist, macOSLaunchAgentPlist(paths.launcher, workingDirectory, logPath), 0o600); err != nil {
		return fmt.Errorf("write macOS LaunchAgent: %w", err)
	}
	uid := strconv.Itoa(os.Getuid())
	_ = exec.Command("launchctl", "bootout", "gui/"+uid, paths.plist).Run()
	if output, err := exec.Command("launchctl", "bootstrap", "gui/"+uid, paths.plist).CombinedOutput(); err != nil {
		return fmt.Errorf("load macOS LaunchAgent: %w: %s", err, output)
	}
	return nil
}

func secureMacOSAutostartLog(path string) error {
	logFile, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := logFile.Chmod(0o600); err != nil {
		_ = logFile.Close()
		return err
	}
	return logFile.Close()
}

func writeMacOSAutostartFile(path string, contents []byte, mode fs.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return nil
}

func disableAutostart() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	paths := macOSAutostartPathsForHome(home)
	_ = exec.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid()), paths.plist).Run()
	var removeErr error
	for _, path := range []string{paths.plist, paths.launcher} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			removeErr = errors.Join(removeErr, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	return removeErr
}

func stopAutostartService() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	path := macOSAutostartPathsForHome(home).plist
	if err := exec.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid()), path).Run(); err != nil {
		return false, nil
	}
	return true, nil
}

func startAutostartService() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	path := macOSAutostartPathsForHome(home).plist
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	if exec.Command("launchctl", "print", domain+"/"+launchAgentName).Run() == nil {
		if output, err := exec.Command("launchctl", "kickstart", "-k", domain+"/"+launchAgentName).CombinedOutput(); err != nil {
			return false, fmt.Errorf("start macOS LaunchAgent: %w: %s", err, output)
		}
		return true, nil
	}
	if output, err := exec.Command("launchctl", "bootstrap", domain, path).CombinedOutput(); err != nil {
		return false, fmt.Errorf("load macOS LaunchAgent: %w: %s", err, output)
	}
	return true, nil
}

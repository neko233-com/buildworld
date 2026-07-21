//go:build !windows

package processtree

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

type platformState struct{}

func startProcess(cmd *exec.Cmd, _ *platformState) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	return cmd.Start()
}

func terminateProcessTree(cmd *exec.Cmd, _ *platformState) error {
	// Setpgid makes the direct child the process-group leader. A negative PID
	// continues to address the group after the leader exits, as long as a child
	// remains in it and keeps an inherited output pipe open.
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.ESRCH) {
		killErr := cmd.Process.Kill()
		if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return errors.Join(err, killErr)
		}
		return err
	}
	killErr := cmd.Process.Kill()
	if errors.Is(killErr, os.ErrProcessDone) {
		return os.ErrProcessDone
	}
	return killErr
}

func releaseProcessTree(_ *platformState, _ bool) error {
	return nil
}

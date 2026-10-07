//go:build !windows

package cli

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func stopProcess(process *os.Process) error { return process.Signal(syscall.SIGTERM) }

func prepareBackground(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func sameProcessName(left, right string) bool { return left == right }

func processRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	_ = process.Release()
	return err == nil || errors.Is(err, syscall.EPERM)
}

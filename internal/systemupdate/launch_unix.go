//go:build darwin || linux

package systemupdate

import (
	"os"
	"os/exec"
	"syscall"
)

func launchDetached(script string, args []string, logPath string) error {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	command := exec.Command("/bin/sh", append([]string{script}, args...)...)
	command.Stdout = logFile
	command.Stderr = logFile
	command.Stdin = nil
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		logFile.Close()
		return err
	}
	go func() {
		_ = command.Wait()
		_ = logFile.Close()
	}()
	return nil
}

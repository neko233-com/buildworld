//go:build windows

package processtree

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var ntResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

type platformState struct {
	job windows.Handle
}

func startProcess(cmd *exec.Cmd, state *platformState) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create process job: %w", err)
	}
	state.job = job
	if err := setJobKillOnClose(job, true); err != nil {
		_ = windows.CloseHandle(job)
		state.job = 0
		return fmt.Errorf("configure process job: %w", err)
	}

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED
	if err := cmd.Start(); err != nil {
		_ = windows.CloseHandle(job)
		state.job = 0
		return err
	}

	var assignErr error
	var resumeErr error
	assigned := false
	handleErr := cmd.Process.WithHandle(func(raw uintptr) {
		process := windows.Handle(raw)
		assignErr = windows.AssignProcessToJobObject(job, process)
		if assignErr == nil {
			assigned = true
			resumeErr = resumeProcess(process)
		}
	})
	if handleErr == nil && assignErr == nil && resumeErr == nil {
		return nil
	}

	setupErr := errors.Join(handleErr, assignErr, resumeErr)
	if assigned {
		// The suspended process belongs to the kill-on-close job, so closing the
		// non-reusable handle atomically tears it down.
		_ = windows.CloseHandle(job)
		state.job = 0
	} else {
		// Assignment failed before the process could execute any user code.
		_ = cmd.Process.Kill()
		_ = windows.CloseHandle(job)
		state.job = 0
	}
	_ = cmd.Wait()
	return fmt.Errorf("attach process to cancellation job: %w", setupErr)
}

func resumeProcess(process windows.Handle) error {
	status, _, _ := ntResumeProcess.Call(uintptr(process))
	if status != 0 {
		return windows.NTStatus(status)
	}
	return nil
}

func setJobKillOnClose(job windows.Handle, enabled bool) error {
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if enabled {
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	}
	_, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	return err
}

func terminateProcessTree(_ *exec.Cmd, state *platformState) error {
	if state.job == 0 {
		return os.ErrProcessDone
	}
	job := state.job
	// Both operations address the non-reusable job handle. TerminateJobObject
	// wakes Wait immediately; KILL_ON_JOB_CLOSE remains an atomic fallback if
	// explicit termination fails.
	terminateErr := windows.TerminateJobObject(job, 1)
	closeErr := windows.CloseHandle(job)
	if closeErr == nil {
		state.job = 0
		return nil
	}
	if terminateErr == nil {
		return fmt.Errorf("close terminated process job: %w", closeErr)
	}
	return errors.Join(terminateErr, closeErr)
}

func releaseProcessTree(state *platformState, preserveDescendants bool) error {
	if state.job == 0 {
		return nil
	}
	job := state.job
	if preserveDescendants {
		if err := setJobKillOnClose(job, false); err != nil {
			terminateErr := windows.CloseHandle(job)
			if terminateErr == nil {
				state.job = 0
			}
			return errors.Join(fmt.Errorf("detach successful descendants: %w", err), terminateErr)
		}
	}
	err := windows.CloseHandle(job)
	if err == nil {
		state.job = 0
	}
	return err
}

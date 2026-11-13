//go:build !windows

package worker

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func hideProcess(command *exec.Cmd) {}

func hostProcesses() ([]processSample, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,ppid=,comm=").Output()
	if err != nil {
		return nil, err
	}
	values := []processSample{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		parent, _ := strconv.Atoi(fields[1])
		values = append(values, processSample{PID: pid, ParentPID: parent, Name: strings.Join(fields[2:], " ")})
	}
	return values, nil
}

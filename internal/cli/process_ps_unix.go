//go:build !windows && !linux

package cli

import (
	"bufio"
	"bytes"
	"os/exec"
	"strconv"
	"strings"
)

func listProcessIdentities() ([]processIdentity, error) {
	output, err := exec.Command("ps", "-A", "-o", "pid=", "-o", "comm=").Output()
	if err != nil {
		return nil, err
	}
	return parsePSProcessIdentities(output)
}

func parsePSProcessIdentities(output []byte) ([]processIdentity, error) {
	processes := make([]processIdentity, 0)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		separator := strings.IndexAny(line, " \t")
		if separator < 1 {
			continue
		}
		pid, err := strconv.Atoi(line[:separator])
		if err != nil || pid <= 0 {
			continue
		}
		name := strings.TrimSpace(line[separator:])
		if name != "" {
			processes = append(processes, processIdentity{PID: pid, Name: name})
		}
	}
	return processes, scanner.Err()
}

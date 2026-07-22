package cli

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStopMatchingBuildWorldServerProcessesOnlyStopsVerifiedName(t *testing.T) {
	target := expectedServerProcessName()
	processes := []processIdentity{
		{PID: 10, Name: "unrelated-service"},
		{PID: 20, Name: filepath.Join("old", target)},
		{PID: 30, Name: target},
		{PID: 40, Name: target},
		{PID: 20, Name: target},
	}
	var stopped []int
	count, err := stopMatchingBuildWorldServerProcesses(
		processes,
		target,
		40,
		30,
		func(pid int) error {
			stopped = append(stopped, pid)
			return nil
		},
		func(int) bool { return false },
	)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || !reflect.DeepEqual(stopped, []int{30, 20}) {
		t.Fatalf("stopped = %v, count = %d; want recorded PID 30 then PID 20", stopped, count)
	}
}

func TestStopMatchingBuildWorldServerProcessesIgnoresStaleUnrelatedPID(t *testing.T) {
	called := false
	count, err := stopMatchingBuildWorldServerProcesses(
		[]processIdentity{{PID: 8700, Name: "unrelated-service"}},
		expectedServerProcessName(),
		1,
		8700,
		func(int) error {
			called = true
			return errors.New("must not be called")
		},
		func(int) bool { return true },
	)
	if err != nil || count != 0 || called {
		t.Fatalf("unrelated recorded process was targeted: called=%v count=%d err=%v", called, count, err)
	}
}

func TestNormalizedProcessNameAcceptsReplacedUnixExecutable(t *testing.T) {
	if got := normalizedProcessName(filepath.Join("tmp", "buildworld-server") + " (deleted)"); got != "buildworld-server" {
		t.Fatalf("normalized process name = %q", got)
	}
}

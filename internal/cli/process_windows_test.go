//go:build windows

package cli

import "testing"

func TestWindowsProcessNameMatchIsCaseInsensitive(t *testing.T) {
	if !sameProcessName("BUILDWORLD-SERVER.EXE", "buildworld-server.exe") {
		t.Fatal("Windows process names should match case-insensitively")
	}
}

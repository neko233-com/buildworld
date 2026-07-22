//go:build !windows

package cli

import "testing"

func TestUnixProcessNameMatchIsCaseSensitive(t *testing.T) {
	if sameProcessName("BuildWorld-Server", "buildworld-server") {
		t.Fatal("Unix process names should match case-sensitively")
	}
}

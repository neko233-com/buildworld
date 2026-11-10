package systemupdate

import "testing"

func TestSystemLaunchDaemonMatchesCurrentServer(t *testing.T) {
	const system = `{
    "LimitLoadToSessionType" = "System";
    "PID" = 65118;
};`
	const user = `{
    "LimitLoadToSessionType" = "Aqua";
    "PID" = 65118;
};`
	for _, tc := range []struct {
		name, output string
		pid          int
		want         bool
	}{
		{"current system daemon", system, 65118, true},
		{"different process", system, 65119, false},
		{"user agent", user, 65118, false},
		{"missing PID", `"LimitLoadToSessionType" = "System";`, 65118, false},
		{"invalid PID", `"PID" = 0;`, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := systemLaunchDaemonMatches(tc.output, tc.pid); got != tc.want {
				t.Fatalf("systemLaunchDaemonMatches = %t, want %t", got, tc.want)
			}
		})
	}
}

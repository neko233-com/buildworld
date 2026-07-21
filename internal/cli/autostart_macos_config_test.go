package cli

import (
	"strings"
	"testing"
)

func TestMacOSAutostartUsesStableUserLauncher(t *testing.T) {
	paths := macOSAutostartPathsForHome("/Users/Example User")
	if paths.plist != "/Users/Example User/Library/LaunchAgents/com.buildworld.server.plist" {
		t.Fatalf("plist path = %q", paths.plist)
	}
	if paths.launcher != "/Users/Example User/Library/Application Support/buildworld/autostart.sh" {
		t.Fatalf("launcher path = %q", paths.launcher)
	}

	server := "/Users/Example User/.local/lib/buildworld/build'world-$`-server"
	config := "/Users/Example User/Library/Application Support/buildworld/config & prod.yaml"
	workingDirectory := "/Users/Example User/Library/Application Support/buildworld"
	launcher := string(macOSAutostartLauncher(server, config, workingDirectory))
	want := "#!/bin/sh\nset -eu\numask 077\n" +
		"cd '/Users/Example User/Library/Application Support/buildworld'\n" +
		"exec '/Users/Example User/.local/lib/buildworld/build'\"'\"'world-$`-server' -config '/Users/Example User/Library/Application Support/buildworld/config & prod.yaml'\n"
	if launcher != want {
		t.Fatalf("launcher = %q, want %q", launcher, want)
	}
	if strings.Contains(launcher, "set -x") {
		t.Fatal("launcher must not echo config-bearing command lines")
	}

	plist := string(macOSLaunchAgentPlist(paths.launcher, workingDirectory, workingDirectory+"/server.log"))
	if !strings.Contains(plist, "<string>/bin/sh</string>") || !strings.Contains(plist, "Application Support/buildworld/autostart.sh</string>") {
		t.Fatalf("plist does not invoke stable shell launcher: %s", plist)
	}
	if strings.Contains(plist, server) || strings.Contains(plist, config) {
		t.Fatal("plist must not expose or bind directly to server/config paths")
	}
}

func TestMacOSLaunchAgentEscapesXMLValues(t *testing.T) {
	plist := string(macOSLaunchAgentPlist(
		`/Users/A&B/<launcher>"'.sh`,
		`/Users/A&B/<state>"'`,
		`/Users/A&B/<server>"'.log`,
	))
	for _, escaped := range []string{"A&amp;B", "&lt;launcher&gt;&quot;&apos;.sh", "&lt;state&gt;&quot;&apos;", "&lt;server&gt;&quot;&apos;.log"} {
		if !strings.Contains(plist, escaped) {
			t.Errorf("plist missing escaped value %q: %s", escaped, plist)
		}
	}
}

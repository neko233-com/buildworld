package engine

import (
	"strings"
	"testing"
)

func TestJenkinsChangeRequestContextPreservesAndValidatesMetadata(t *testing.T) {
	encoded, err := AttachJenkinsChangeRequestContext(`{"COUNT":9007199254740993}`, JenkinsChangeRequestContext{
		ID: "42", Target: "main", Branch: "feature", Title: "A change", Author: "alice",
	})
	if err != nil || !strings.Contains(encoded, "9007199254740993") {
		t.Fatalf("context=%q err=%v", encoded, err)
	}
	got := readJenkinsChangeRequestContext(encoded, "42")
	if got == nil || got.Target != "main" || got.Author != "alice" {
		t.Fatalf("context=%+v", got)
	}
	if readJenkinsChangeRequestContext(encoded, "43") != nil {
		t.Fatal("different PR identity accepted")
	}
	if _, err := AttachJenkinsChangeRequestContext(encoded, JenkinsChangeRequestContext{ID: "42"}); err == nil {
		t.Fatal("reserved context may not be replaced")
	}
	for _, invalid := range []JenkinsChangeRequestContext{{}, {ID: "42", Title: "bad\x00value"}, {ID: "42", Branch: strings.Repeat("a", 2049)}} {
		if _, err := AttachJenkinsChangeRequestContext("", invalid); err == nil {
			t.Fatalf("invalid context accepted: %+v", invalid)
		}
	}
}

func TestJenkinsTriggerUserIsReservedAndNotAUserParameter(t *testing.T) {
	encoded, err := AttachJenkinsTriggerUser(`{"COUNT":9007199254740993}`, "alice")
	if err != nil || readJenkinsTriggerUser(encoded) != "alice" || !strings.Contains(encoded, "9007199254740993") {
		t.Fatalf("encoded=%q err=%v", encoded, err)
	}
	if _, err := AttachJenkinsTriggerUser(encoded, "mallory"); err == nil {
		t.Fatal("internal user overwritten")
	}
	if _, err := ResolveBuildParameters([]BuildParameter{{Name: jenkinsTriggerUserContextKey, Type: "string"}}, nil); err == nil {
		t.Fatal("reserved user parameter was declared")
	}
	if _, err := ResolveBuildParameters([]BuildParameter{{Name: jenkinsChangeRequestContextKey, Type: "string"}}, nil); err == nil {
		t.Fatal("reserved PR parameter was declared")
	}
	if _, exists := parseParams(encoded)[jenkinsTriggerUserContextKey]; exists {
		t.Fatal("internal user leaked to pipeline parameters")
	}
}

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

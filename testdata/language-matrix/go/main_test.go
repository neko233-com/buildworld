package main

import "testing"

func TestMessage(t *testing.T) {
	if got := message("matrix"); got != "hello, matrix" {
		t.Fatalf("message() = %q", got)
	}
}

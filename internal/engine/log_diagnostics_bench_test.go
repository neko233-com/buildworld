package engine

import (
	"strings"
	"testing"
)

func BenchmarkBuildLogDiagnostics(b *testing.B) {
	log := "[10:00:00] [] ::buildworld:plan {\"stages\":[{\"name\":\"Build\",\"steps\":[\"Compile\"]}]}\n" +
		"[10:00:00] [Build] === Stage: Build ===\n[10:00:00] [Build] --- Step: Compile ---\n" +
		strings.Repeat("[10:00:00] [Build] ordinary compiler output line\n", 18_000) +
		"[10:02:00] [Build] ERROR: compile failed\n[10:02:00] [] BUILD FAILED: compile failed\n"
	b.Run("timeline", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = ParseBuildTimeline("running", log)
		}
	})
	b.Run("problems", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = AnalyzeBuildProblems("failed", log)
		}
	})
}

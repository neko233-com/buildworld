package store

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAppendBuildLogRetainsShortBuildExactly(t *testing.T) {
	data, build := newBuildLogTestStore(t)
	if err := data.AppendBuildLog(build.ID, "first\n"); err != nil {
		t.Fatal(err)
	}
	if err := data.AppendBuildLog(build.ID, "second\n"); err != nil {
		t.Fatal(err)
	}
	stored, err := data.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Log != "first\nsecond\n" || IsBuildLogTruncated(stored.Log) {
		t.Fatalf("short log = %q", stored.Log)
	}
}

func TestGetBuildLogTailReturnsNewestLinesWithoutLoadingFullWindow(t *testing.T) {
	data, build := newBuildLogTestStore(t)
	full := "oldest\nmiddle\nnewest\n"
	if err := data.SetBuildLog(build.ID, full); err != nil {
		t.Fatal(err)
	}

	tail, err := data.GetBuildLogTail(build.ID, 2, 1_000)
	if err != nil {
		t.Fatal(err)
	}
	if tail.Log != "middle\nnewest\n" || !tail.WindowTruncated || tail.PersistedTruncated {
		t.Fatalf("tail = %#v", tail)
	}
}

func TestGetBuildLogTailPreservesUTF8AndCharacterWindow(t *testing.T) {
	data, build := newBuildLogTestStore(t)
	if err := data.SetBuildLog(build.ID, "旧日志\n最新日志\n"); err != nil {
		t.Fatal(err)
	}

	tail, err := data.GetBuildLogTail(build.ID, 10, 5)
	if err != nil {
		t.Fatal(err)
	}
	if tail.Log != "最新日志\n" || !tail.WindowTruncated || !utf8.ValidString(tail.Log) {
		t.Fatalf("utf8 tail = %#v", tail)
	}
}

func TestAppendBuildLogSustainedOutputRemainsBounded(t *testing.T) {
	data, build := newBuildLogTestStore(t)
	chunk := strings.Repeat("x", 32*1024-16) + "\nchunk-boundary\n"
	for range 96 {
		if err := data.AppendBuildLog(build.ID, chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := data.AppendBuildLog(build.ID, "newest-entry\n"); err != nil {
		t.Fatal(err)
	}
	stored, err := data.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count := utf8.RuneCountInString(stored.Log); count > BuildLogRetentionCharacters {
		t.Fatalf("retained %d characters, max %d", count, BuildLogRetentionCharacters)
	}
	if !IsBuildLogTruncated(stored.Log) || !strings.HasPrefix(stored.Log, BuildLogTruncationMarker) {
		t.Fatalf("retained log lacks truncation marker: %.160q", stored.Log)
	}
	if !strings.HasSuffix(stored.Log, "newest-entry\n") || !utf8.ValidString(stored.Log) {
		t.Fatalf("retained log lost newest valid UTF-8 output")
	}
}

func TestAppendBuildLogBoundsOneOversizedEntry(t *testing.T) {
	data, build := newBuildLogTestStore(t)
	entry := strings.Repeat("界", BuildLogAppendMaxBytes) + "newest"
	if err := data.AppendBuildLog(build.ID, entry); err != nil {
		t.Fatal(err)
	}
	stored, err := data.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !IsBuildLogTruncated(stored.Log) || !strings.Contains(stored.Log, BuildLogOversizedMarker) {
		t.Fatalf("oversized log lacks marker: %.160q", stored.Log)
	}
	if len(stored.Log) > BuildLogAppendMaxBytes+len(BuildLogTruncationMarker)+len(BuildLogOversizedMarker) {
		t.Fatalf("oversized persisted entry uses %d bytes", len(stored.Log))
	}
	if !utf8.ValidString(stored.Log) || !strings.HasSuffix(stored.Log, "newest") {
		t.Fatalf("oversized persisted entry corrupted its UTF-8 tail")
	}
}

func TestSetBuildLogAlsoEnforcesRetention(t *testing.T) {
	data, build := newBuildLogTestStore(t)
	log := strings.Repeat("月", BuildLogRetentionCharacters+100) + "newest"
	if err := data.SetBuildLog(build.ID, log); err != nil {
		t.Fatal(err)
	}
	stored, err := data.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if utf8.RuneCountInString(stored.Log) > BuildLogRetentionCharacters ||
		!strings.HasPrefix(stored.Log, BuildLogTruncationMarker) ||
		!strings.HasSuffix(stored.Log, "newest") {
		t.Fatal("SetBuildLog did not retain a marked newest tail")
	}
}

func newBuildLogTestStore(t *testing.T) (*Store, *Build) {
	t.Helper()
	data, cleanup := newTestStore(t)
	t.Cleanup(cleanup)
	project, err := data.CreateProject("log-retention", "", "", "git", "main", `{}`, 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return data, build
}

func BenchmarkAppendBuildLogAtRetentionLimit(b *testing.B) {
	data, err := New(filepath.Join(b.TempDir(), "build-log-benchmark.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = data.Close() })
	project, err := data.CreateProject("log-benchmark", "", "", "git", "main", `{}`, 0, nil, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	if err := data.SetBuildLog(build.ID, strings.Repeat("x", BuildLogRetentionCharacters)); err != nil {
		b.Fatal(err)
	}
	chunk := strings.Repeat("y", 64*1024)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if err := data.AppendBuildLog(build.ID, chunk); err != nil {
			b.Fatal(err)
		}
	}
}

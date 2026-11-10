package engine

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestJenkinsSCMChangesCaptureEachCommitAndRename(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	repo := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		output, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	run("init")
	run("config", "user.email", "test@example.invalid")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "old.txt"), []byte("unchanged content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "old.txt")
	run("commit", "-m", "baseline")
	baseline := run("rev-parse", "HEAD")
	run("mv", "old.txt", "new.txt")
	run("commit", "-m", "rename file", "-m", "Release note")
	run("commit", "--allow-empty", "-m", "empty marker")
	revision := run("rev-parse", "HEAD")
	changes, err := collectJenkinsSCMChanges(context.Background(), repo, revision, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || changes[0].Title != "empty marker" || len(changes[0].Paths) != 0 || changes[1].Title != "rename file" || !strings.Contains(changes[1].Comment, "Release note") {
		t.Fatalf("unexpected changes: %+v", changes)
	}
	paths := strings.Join(changes[1].Paths, ",")
	if !strings.Contains(paths, "old.txt") || !strings.Contains(paths, "new.txt") {
		t.Fatalf("rename must expose both paths: %s", paths)
	}
	first, err := collectJenkinsSCMChanges(context.Background(), repo, revision, "")
	if err != nil || len(first) != 0 {
		t.Fatalf("first checkout changes=%+v err=%v", first, err)
	}
}

func TestJenkinsPullRequestHeadHistory(t *testing.T) {
	for _, tc := range []struct {
		ref, id string
		ok      bool
	}{
		{"refs/pull/123/head", "123", true},
		{"refs/merge-requests/123/head", "123", true},
		{"refs/merge-requests/123/merge", "", false},
		{"refs/pull/123/merge", "", false},
		{"refs/pull/0/head", "", false},
		{"refs/pull/01/head", "", false},
		{"refs/pull/a/head", "", false},
		{"refs/heads/feature", "", false},
	} {
		id, ok := jenkinsPullRequestHead(tc.ref)
		if id != tc.id || ok != tc.ok {
			t.Fatalf("ref=%q id=%q ok=%v", tc.ref, id, ok)
		}
	}
	for _, record := range []store.JenkinsSCMCheckout{
		{HistoryKnown: true, ChangesJSON: `[]`},
		{HistoryKnown: true, ChangesJSON: `{"version":2,"changes":[]}`},
		{HistoryKnown: false, ChangesJSON: `{"version":1,"changes":[]}`},
	} {
		_, unknown, err := jenkinsPullRequestChanges([]store.JenkinsSCMCheckout{record})
		if err != nil || !unknown {
			t.Fatalf("unverified history accepted: %+v unknown=%v err=%v", record, unknown, err)
		}
	}
	jsonChanges, unknown, err := jenkinsPullRequestChanges([]store.JenkinsSCMCheckout{
		{HistoryKnown: true, ChangesJSON: `{"version":1,"changes":[{"revision":"a","title":"one","comment":"one","paths":["a.go"]}]}`},
		{HistoryKnown: true, ChangesJSON: `{"version":1,"changes":[{"revision":"b","title":"two","comment":"two","paths":["b.go"]}]}`},
	})
	if err != nil || unknown || !strings.Contains(jsonChanges, "a.go") || !strings.Contains(jsonChanges, "b.go") {
		t.Fatalf("aggregated=%s unknown=%v err=%v", jsonChanges, unknown, err)
	}
}

func TestJenkinsSCMConditionMatchers(t *testing.T) {
	changes := []jenkinsSCMChange{{Title: "feat: ship", Comment: "feat: ship\n\nRelease: approved\n", Paths: []string{"src/Feature.go", "old/feature.go"}}}
	encoded, err := json.Marshal(changes)
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{jenkinsSCMChangesContextKey: string(encoded)}
	for _, tc := range []struct {
		condition string
		want      bool
	}{
		{`jenkinsChangeSet("src/*.GO","GLOB","false")`, true},
		{`jenkinsChangeSet("src/*.GO","GLOB","true")`, false},
		{`jenkinsChangeSet("src/Feature.go","EQUALS","false")`, true},
		{`jenkinsChangeSet("src/feature.go","EQUALS","false")`, false},
		{`jenkinsChangeSet("src/.*\\.go","REGEXP","false")`, true},
		{`jenkinsChangeLog("feat:.*")`, true},
		{`jenkinsChangeLog(".*Release: approved.*")`, true},
		{`jenkinsChangeLog("fix:.*")`, false},
	} {
		got, err := EvaluateJenkinsWhenContext(tc.condition, nil, env, "main")
		if err != nil || got != tc.want {
			t.Fatalf("condition=%s got=%v want=%v err=%v", tc.condition, got, tc.want, err)
		}
	}
	if got, err := EvaluateJenkinsWhenContext(`jenkinsChangeSet("**/*.go","GLOB","false")`, nil, nil, "main"); err != nil || got {
		t.Fatalf("missing checkout matched: %v err=%v", got, err)
	}
	env[jenkinsSCMHistoryUnknownContextKey] = "true"
	if _, err := EvaluateJenkinsWhenContext(`jenkinsChangeSet("**/*.go","GLOB","false")`, nil, env, "main"); err == nil {
		t.Fatal("unknown prior checkout history must not be silently treated as empty")
	}
	delete(env, jenkinsSCMHistoryUnknownContextKey)
	env["CHANGE_ID"] = "123"
	if got, err := EvaluateJenkinsWhenContext(`jenkinsChangeSet("**/*.go","GLOB","false")`, nil, env, "main"); err != nil || !got {
		t.Fatalf("a manually supplied CHANGE_ID must not invalidate the current checkout: got=%v err=%v", got, err)
	}
	if got, err := EvaluateJenkinsWhenContext(`jenkinsChangeLog("feat:.*")`, nil, env, "main"); err != nil || !got {
		t.Fatalf("a manually supplied CHANGE_ID must not invalidate the current changelog: got=%v err=%v", got, err)
	}
}

func TestJenkinsSCMLogPreservesUnusualPathAndRejectsMalformedRecords(t *testing.T) {
	revision := strings.Repeat("a", 40)
	data := []byte("\x00" + revision + "\x00subject\n\x00\n\nunusual.txt\x00")
	changes, err := parseJenkinsSCMLog(data)
	if err != nil || len(changes) != 1 || len(changes[0].Paths) != 1 || changes[0].Paths[0] != "\nunusual.txt" {
		t.Fatalf("changes=%+v err=%v", changes, err)
	}
	if _, err := parseJenkinsSCMLog([]byte("\x00bad-revision\x00message\x00")); err == nil {
		t.Fatal("malformed changelog record accepted")
	}
}

package migration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
)

type advancingSCMFetcher struct {
	repository string
	once       sync.Once
	err        error
}

func (f *advancingSCMFetcher) FetchSCMPipeline(ctx context.Context, source engine.SCMPipelineSource) (string, engine.PipelineFormat, error) {
	return engine.FetchSCMPipeline(ctx, source)
}

func (f *advancingSCMFetcher) FetchSCMPipelineWithRevision(ctx context.Context, source engine.SCMPipelineSource) (string, engine.PipelineFormat, string, error) {
	content, format, revision, err := engine.FetchSCMPipelineWithRevision(ctx, source)
	if err != nil {
		return "", "", "", err
	}
	f.once.Do(func() {
		if f.err = os.WriteFile(filepath.Join(f.repository, "marker"), []byte("second"), 0o600); f.err != nil {
			return
		}
		if f.err = os.WriteFile(filepath.Join(f.repository, "Jenkinsfile"), []byte(scmRevisionPipeline("second")), 0o600); f.err != nil {
			return
		}
		for _, args := range [][]string{{"-C", f.repository, "add", "marker", "Jenkinsfile"}, {"-C", f.repository, "commit", "-m", "advance"}} {
			if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
				f.err = fmt.Errorf("git: %w: %s", err, output)
				return
			}
		}
	})
	return content, format, revision, f.err
}

func scmRevisionPipeline(marker string) string {
	branch := "main"
	if marker == "feature" {
		branch = "feature"
	}
	return fmt.Sprintf(`pipeline { agent any stages { stage('verify') { steps { sh 'set -eu; test "$(cat marker)" = %s; test "$GIT_COMMIT" = "$(git rev-parse HEAD)"; test "$BRANCH_NAME" = %s; test "$(git branch --show-current)" = %s; echo verified-%s' } } } }`, marker, branch, branch, marker)
}

func TestSCMPipelinePinsDefinitionAndWorkspaceAcrossAdvanceAndRestart(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	repository := filepath.Join(root, "source")
	runJenkinsMigrationGit(t, "init", repository)
	runJenkinsMigrationGit(t, "-C", repository, "config", "user.email", "test@example.invalid")
	runJenkinsMigrationGit(t, "-C", repository, "config", "user.name", "BuildWorld Test")
	if err := os.WriteFile(filepath.Join(repository, "marker"), []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "Jenkinsfile"), []byte(scmRevisionPipeline("first")), 0o600); err != nil {
		t.Fatal(err)
	}
	runJenkinsMigrationGit(t, "-C", repository, "add", "marker", "Jenkinsfile")
	runJenkinsMigrationGit(t, "-C", repository, "commit", "-m", "initial")
	runJenkinsMigrationGit(t, "-C", repository, "branch", "-M", "main")
	output, err := exec.Command("git", "-C", repository, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	firstRevision := strings.TrimSpace(string(output))
	runJenkinsMigrationGit(t, "-C", repository, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(repository, "marker"), []byte("feature"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "Jenkinsfile"), []byte(scmRevisionPipeline("feature")), 0o600); err != nil {
		t.Fatal(err)
	}
	runJenkinsMigrationGit(t, "-C", repository, "add", "marker", "Jenkinsfile")
	runJenkinsMigrationGit(t, "-C", repository, "commit", "-m", "feature")
	runJenkinsMigrationGit(t, "-C", repository, "checkout", "main")
	for _, tc := range []engine.SCMPipelineSource{
		{RepoURL: repository, Branch: "main", Path: "Jenkinsfile", Revision: "--bad"},
		{RepoURL: repository, Branch: "main", Path: "Jenkinsfile", Revision: strings.Repeat("0", 40)},
		{RepoURL: repository, Branch: "main", Path: "../Jenkinsfile"},
	} {
		if _, _, _, err := engine.FetchSCMPipelineWithRevision(context.Background(), tc); err == nil {
			t.Fatalf("unsafe or missing revision unexpectedly accepted: %+v", tc)
		}
	}

	database, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject("pin-scm", "", "", "git", "feature", "", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectPipelineSource(project.ID, "jenkinsfile", "scm", repository, "main", "Jenkinsfile"); err != nil {
		t.Fatal(err)
	}
	previous := engine.DefaultSCMFetcher
	engine.DefaultSCMFetcher = &advancingSCMFetcher{repository: repository}
	t.Cleanup(func() { engine.DefaultSCMFetcher = previous })
	runner := engine.NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	for i, tc := range []struct {
		branch, requested, expected, marker string
	}{
		{"main", "", firstRevision, "first"},
		{"main", firstRevision, firstRevision, "first"},
		{"main", "", "", "second"},
		{"feature", "", "", "feature"},
		{"", "", "", "second"},
	} {
		build, err := database.CreateBuild(project.ID, i+1, "manual", tc.branch, tc.requested, "", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		runner.Run(build.ID)
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			build, err = database.GetBuild(build.ID)
			if err != nil {
				t.Fatal(err)
			}
			if build.Status == "success" || build.Status == "failed" {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if build.Status != "success" || !strings.Contains(build.Log, "verified-"+tc.marker) {
			t.Fatalf("case %d: status=%s log=%s", i, build.Status, build.Log)
		}
		if len(build.CommitSHA) != 40 || (tc.expected != "" && build.CommitSHA != tc.expected) {
			t.Fatalf("case %d: commit=%q want=%q", i, build.CommitSHA, tc.expected)
		}
		if i == 2 && build.CommitSHA == firstRevision {
			t.Fatal("new build did not select the advanced branch head")
		}
	}
}

func TestSCMPipelineRestartUsesRecordedCommit(t *testing.T) {
	testSCMPipelineRestartUsesRecordedCommit(t, "main")
}

func TestSCMTagPipelineRestartUsesRecordedCommit(t *testing.T) {
	testSCMPipelineRestartUsesRecordedCommit(t, "refs/tags/release-restart")
}

func testSCMPipelineRestartUsesRecordedCommit(t *testing.T, ref string) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	repository := filepath.Join(root, "source")
	runJenkinsMigrationGit(t, "init", repository)
	runJenkinsMigrationGit(t, "-C", repository, "config", "user.email", "test@example.invalid")
	runJenkinsMigrationGit(t, "-C", repository, "config", "user.name", "BuildWorld Test")
	checkpoint := filepath.ToSlash(filepath.Join(root, "checkpoint"))
	when, tagCheck := "", ""
	if strings.HasPrefix(ref, "refs/tags/") {
		when = `when { allOf { buildingTag(); tag 'release-*' } }`
		tagCheck = `test "$TAG_NAME" = release-restart`
	}
	source := fmt.Sprintf(`pipeline { agent any stages { stage('verify') { %s steps { sh '''
set -eu
if [ ! -f '%s' ]; then
  touch '%s'
  while :; do sleep 1; done
fi
test "$(cat marker)" = first
%s
echo recovered-first
''' } } } }`, when, checkpoint, checkpoint, tagCheck)
	if err := os.WriteFile(filepath.Join(repository, "marker"), []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "Jenkinsfile"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	runJenkinsMigrationGit(t, "-C", repository, "add", "marker", "Jenkinsfile")
	runJenkinsMigrationGit(t, "-C", repository, "commit", "-m", "initial")
	runJenkinsMigrationGit(t, "-C", repository, "branch", "-M", "main")
	if strings.HasPrefix(ref, "refs/tags/") {
		runJenkinsMigrationGit(t, "-C", repository, "tag", "-a", "release-restart", "-m", "annotated")
	}
	database, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject("restart-scm", "", "", "git", "main", "", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectPipelineSource(project.ID, "jenkinsfile", "scm", repository, "main", "Jenkinsfile"); err != nil {
		t.Fatal(err)
	}
	build, err := database.CreateBuild(project.ID, 1, "manual", ref, "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	runner.Run(build.ID)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(checkpoint); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(checkpoint); err != nil {
		_ = runner.Shutdown(context.Background())
		t.Fatal("initial build did not reach its checkpoint")
	}
	if err := runner.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	before, err := database.GetBuild(build.ID)
	if err != nil || before.Status != "running" || len(before.CommitSHA) != 40 {
		t.Fatalf("interrupted build=%+v err=%v", before, err)
	}
	if err := os.WriteFile(filepath.Join(repository, "marker"), []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "Jenkinsfile"), []byte(scmRevisionPipeline("second")), 0o600); err != nil {
		t.Fatal(err)
	}
	runJenkinsMigrationGit(t, "-C", repository, "add", "marker", "Jenkinsfile")
	runJenkinsMigrationGit(t, "-C", repository, "commit", "-m", "advance")
	if strings.HasPrefix(ref, "refs/tags/") {
		runJenkinsMigrationGit(t, "-C", repository, "tag", "-f", "release-restart", "HEAD")
	}
	restarted := engine.NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	restarted.StartQueue(ctx)
	defer restarted.Shutdown(context.Background())
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		build, err = database.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if build.Status == "success" || build.Status == "failed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if build.Status != "success" || build.CommitSHA != before.CommitSHA || !strings.Contains(build.Log, "recovered-first") {
		t.Fatalf("restart status=%s commit=%q log=%s", build.Status, build.CommitSHA, build.Log)
	}
}

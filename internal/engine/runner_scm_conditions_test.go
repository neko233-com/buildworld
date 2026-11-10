package engine_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/engine"
	_ "github.com/neko233-com/buildworld/internal/migration"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestJenkinsSCMBuildFetchesPinnedHiddenRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	origin, repository := filepath.Join(root, "origin.git"), filepath.Join(root, "repository")
	runRunnerSCMGit(t, "init", "--bare", origin)
	runRunnerSCMGit(t, "init", repository)
	runRunnerSCMGit(t, "-C", repository, "config", "user.email", "test@example.invalid")
	runRunnerSCMGit(t, "-C", repository, "config", "user.name", "Test")
	jenkinsfile := filepath.Join(repository, "Jenkinsfile")
	if err := os.WriteFile(jenkinsfile, []byte(`pipeline { agent any stages { stage('build') { steps { sh 'echo WRONG_SOURCE' } } } }`), 0o644); err != nil {
		t.Fatal(err)
	}
	runRunnerSCMGit(t, "-C", repository, "add", "Jenkinsfile")
	runRunnerSCMGit(t, "-C", repository, "commit", "-m", "base")
	runRunnerSCMGit(t, "-C", repository, "branch", "-M", "main")
	runRunnerSCMGit(t, "-C", repository, "remote", "add", "origin", origin)
	runRunnerSCMGit(t, "-C", repository, "push", "origin", "main")
	runRunnerSCMGit(t, "--git-dir", origin, "symbolic-ref", "HEAD", "refs/heads/main")
	runRunnerSCMGit(t, "-C", repository, "checkout", "-b", "contribution")
	if err := os.WriteFile(jenkinsfile, []byte(`pipeline { agent any stages { stage('build') { steps { sh 'echo HIDDEN_REF_SOURCE' } } } }`), 0o644); err != nil {
		t.Fatal(err)
	}
	runRunnerSCMGit(t, "-C", repository, "add", "Jenkinsfile")
	runRunnerSCMGit(t, "-C", repository, "commit", "-m", "contribution")
	revisionBytes, err := exec.Command("git", "-C", repository, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	revision := strings.TrimSpace(string(revisionBytes))
	ref := "refs/pull/123/head"
	runRunnerSCMGit(t, "-C", repository, "push", "origin", "HEAD:"+ref)
	originPath := filepath.ToSlash(origin)
	if !strings.HasPrefix(originPath, "/") {
		originPath = "/" + originPath
	}
	originURL := (&url.URL{Scheme: "file", Path: originPath}).String()
	probe := filepath.Join(root, "ordinary-clone")
	runRunnerSCMGit(t, "clone", originURL, probe)
	if err := exec.Command("git", "-C", probe, "cat-file", "-e", revision+"^{commit}").Run(); err == nil {
		t.Fatal("fixture's hidden commit unexpectedly exists in a normal clone")
	}
	if _, _, _, err := engine.FetchSCMPipelineWithRevision(context.Background(), engine.SCMPipelineSource{
		RepoURL: originURL, Branch: ref, Revision: strings.Repeat("f", 40), Path: "Jenkinsfile", Format: engine.FormatJenkinsfile,
	}); err == nil {
		t.Fatal("missing pinned commit must not fall back to the hidden ref tip")
	}
	database, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject("hidden-ref", "", "", "git", "main", "", 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectPipelineSource(project.ID, "jenkinsfile", "scm", originURL, "main", "Jenkinsfile"); err != nil {
		t.Fatal(err)
	}
	build, err := database.CreateBuild(project.ID, 1, "manual", ref, revision, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	finished := waitForRunnerSCMBuild(t, database, build.ID, 20*time.Second)
	if finished.Status != "success" || finished.CommitSHA != revision || !strings.Contains(finished.Log, "HIDDEN_REF_SOURCE") || strings.Contains(finished.Log, "WRONG_SOURCE") {
		t.Fatalf("hidden ref status=%s commit=%s log=%s", finished.Status, finished.CommitSHA, finished.Log)
	}
}

func TestJenkinsPullRequestAggregatesPriorBuildChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	origin, repository := filepath.Join(root, "origin.git"), filepath.Join(root, "repository")
	runRunnerSCMGit(t, "init", "--bare", origin)
	runRunnerSCMGit(t, "init", repository)
	runRunnerSCMGit(t, "-C", repository, "config", "user.email", "test@example.invalid")
	runRunnerSCMGit(t, "-C", repository, "config", "user.name", "Test")
	jenkinsfile := filepath.Join(repository, "Jenkinsfile")
	initial := `pipeline { agent any stages { stage('build') { steps { sh 'echo INITIAL_BUILD' } } } }`
	if err := os.WriteFile(jenkinsfile, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	runRunnerSCMGit(t, "-C", repository, "add", "Jenkinsfile")
	runRunnerSCMGit(t, "-C", repository, "commit", "-m", "base")
	runRunnerSCMGit(t, "-C", repository, "branch", "-M", "main")
	runRunnerSCMGit(t, "-C", repository, "remote", "add", "origin", origin)
	runRunnerSCMGit(t, "-C", repository, "push", "origin", "main")
	runRunnerSCMGit(t, "--git-dir", origin, "symbolic-ref", "HEAD", "refs/heads/main")
	runRunnerSCMGit(t, "-C", repository, "checkout", "-b", "contribution")
	ref := "refs/pull/123/head"
	runRunnerSCMGit(t, "-C", repository, "push", "origin", "HEAD:"+ref)
	originPath := filepath.ToSlash(origin)
	if !strings.HasPrefix(originPath, "/") {
		originPath = "/" + originPath
	}
	originURL := (&url.URL{Scheme: "file", Path: originPath}).String()
	database, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject("pr-history", "", "", "git", "main", "", 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectPipelineSource(project.ID, "jenkinsfile", "scm", originURL, "main", "Jenkinsfile"); err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runBuild := func(number int, branch string) *store.Build {
		t.Helper()
		build, err := database.CreateBuild(project.ID, number, "manual", branch, "", "", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		runner.Run(build.ID)
		result := waitForRunnerSCMBuild(t, database, build.ID, 25*time.Second)
		if result.Status != "success" {
			t.Fatalf("build %d status=%s log=%s", number, result.Status, result.Log)
		}
		return result
	}
	runBuild(1, ref)
	if err := os.Mkdir(filepath.Join(repository, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "src", "service.go"), []byte("package service\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runRunnerSCMGit(t, "-C", repository, "add", "src/service.go")
	runRunnerSCMGit(t, "-C", repository, "commit", "-m", "feat: service")
	runRunnerSCMGit(t, "-C", repository, "push", "origin", "HEAD:"+ref)
	second := runBuild(2, ref)
	checkout, err := database.JenkinsSCMCheckoutForBuild(second.ID)
	if err != nil || checkout == nil || !strings.Contains(checkout.ChangesJSON, "src/service.go") || !strings.Contains(checkout.ChangesJSON, `"version":1`) {
		t.Fatalf("PR changes must be captured even without a when condition: checkout=%+v err=%v", checkout, err)
	}
	source := `pipeline { agent any options { skipDefaultCheckout() } stages {
stage('before') { when { changeset 'src/**/*.go' } steps { sh 'echo PRIOR_PR_CHANGE' } }
stage('checkout') { steps { checkout scm } }
stage('after') { when { allOf { changeset 'src/**/*.go'; changelog 'feat:.*' } } steps { sh 'echo AGGREGATED_PR_CHANGE' } }
stage('identity') { when { allOf { changeRequest id: '123'; branch 'PR-123' } } steps { sh 'echo PR_IDENTITY_MATCHED' } }
} }`
	if err := os.WriteFile(jenkinsfile, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	runRunnerSCMGit(t, "-C", repository, "add", "Jenkinsfile")
	runRunnerSCMGit(t, "-C", repository, "commit", "-m", "add conditions")
	runRunnerSCMGit(t, "-C", repository, "push", "origin", "HEAD:"+ref)
	third := runBuild(3, ref)
	for _, marker := range []string{"PRIOR_PR_CHANGE", "AGGREGATED_PR_CHANGE", "PR_IDENTITY_MATCHED"} {
		if !strings.Contains(third.Log, marker) {
			t.Fatalf("missing %s: %s", marker, third.Log)
		}
	}
	withoutCheckout := `pipeline { agent any options { skipDefaultCheckout() } stages { stage('skip') { steps { sh 'echo NO_CHECKOUT' } } } }`
	if err := os.WriteFile(jenkinsfile, []byte(withoutCheckout), 0o644); err != nil {
		t.Fatal(err)
	}
	runRunnerSCMGit(t, "-C", repository, "add", "Jenkinsfile")
	runRunnerSCMGit(t, "-C", repository, "commit", "-m", "skip checkout")
	runRunnerSCMGit(t, "-C", repository, "push", "origin", "HEAD:"+ref)
	fourth := runBuild(4, ref)
	var emptyCheckout *store.JenkinsSCMCheckout
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		emptyCheckout, err = database.JenkinsSCMCheckoutForBuild(fourth.ID)
		if err != nil || emptyCheckout != nil {
			break
		}
	}
	if err != nil || emptyCheckout == nil || emptyCheckout.Revision != "" {
		t.Fatalf("no-checkout build was not recorded: checkout=%+v err=%v", emptyCheckout, err)
	}
	if err := os.WriteFile(jenkinsfile, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	runRunnerSCMGit(t, "-C", repository, "add", "Jenkinsfile")
	runRunnerSCMGit(t, "-C", repository, "commit", "-m", "restore conditions")
	runRunnerSCMGit(t, "-C", repository, "push", "origin", "HEAD:"+ref)
	fifth := runBuild(5, ref)
	if !strings.Contains(fifth.Log, "PRIOR_PR_CHANGE") || !strings.Contains(fifth.Log, "AGGREGATED_PR_CHANGE") {
		t.Fatalf("no-checkout build hid PR history: %s", fifth.Log)
	}
	otherRef := "refs/pull/124/head"
	runRunnerSCMGit(t, "-C", repository, "push", "origin", "HEAD:"+otherRef)
	other := runBuild(6, otherRef)
	for _, marker := range []string{"PRIOR_PR_CHANGE", "AGGREGATED_PR_CHANGE", "PR_IDENTITY_MATCHED"} {
		if strings.Contains(other.Log, marker) {
			t.Fatalf("another PR inherited %s: %s", marker, other.Log)
		}
	}
	legacyRef := "refs/pull/125/head"
	runRunnerSCMGit(t, "-C", repository, "push", "origin", "HEAD:"+legacyRef)
	legacy, err := database.CreateBuild(project.ID, 7, "manual", legacyRef, "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.FinishBuild(legacy.ID, "success", 1); err != nil {
		t.Fatal(err)
	}
	current, err := database.CreateBuild(project.ID, 8, "manual", legacyRef, "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner.Run(current.ID)
	uncertain := waitForRunnerSCMBuild(t, database, current.ID, 25*time.Second)
	if uncertain.Status != "failed" || !strings.Contains(uncertain.Log, "checkout history is unavailable") || strings.Contains(uncertain.Log, "PRIOR_PR_CHANGE") {
		t.Fatalf("unknown PR history must fail closed: status=%s log=%s", uncertain.Status, uncertain.Log)
	}
}

func TestJenkinsSCMConditionsUseCheckoutHistory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	runRunnerSCMGit(t, "init", repository)
	runRunnerSCMGit(t, "-C", repository, "config", "user.email", "test@example.invalid")
	runRunnerSCMGit(t, "-C", repository, "config", "user.name", "Test")
	source := `pipeline { agent any
environment { CHANGE_ID = '123' }
options { skipDefaultCheckout() }
stages {
stage('before') { when { changeset 'src/**/*.go' } steps { sh 'echo BEFORE_CHECKOUT_INCORRECT' } }
stage('checkout') { steps { checkout scm } }
stage('changed') { when { allOf { changeRequest id: '123'; changeset pattern: 'src/**/*.go', caseSensitive: true; changelog 'feat:.*' } } steps { sh 'echo SCM_CHANGED_EXECUTED' } }
stage('missing') { when { changeset 'missing/**' } steps { sh 'echo SCM_MISSING_INCORRECT' } }
} }`
	if err := os.WriteFile(filepath.Join(repository, "Jenkinsfile"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	runRunnerSCMGit(t, "-C", repository, "add", "Jenkinsfile")
	runRunnerSCMGit(t, "-C", repository, "commit", "-m", "initial")
	runRunnerSCMGit(t, "-C", repository, "branch", "-M", "main")
	dbPath := filepath.Join(root, "buildworld.db")
	database, err := store.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { database.Close() }()
	project, err := database.CreateProject("scm-conditions", "", "", "git", "main", "", 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectPipelineSource(project.ID, "jenkinsfile", "scm", repository, "main", "Jenkinsfile"); err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	runBuild := func(number int, wantChanged bool) *store.Build {
		t.Helper()
		build, err := database.CreateBuild(project.ID, number, "manual", "main", "", "", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		runner.Run(build.ID)
		finished := waitForRunnerSCMBuild(t, database, build.ID, 20*time.Second)
		if finished.Status != "success" || strings.Contains(finished.Log, "BEFORE_CHECKOUT_INCORRECT") || strings.Contains(finished.Log, "SCM_MISSING_INCORRECT") || strings.Contains(finished.Log, "SCM_CHANGED_EXECUTED") != wantChanged {
			t.Fatalf("build=%d status=%s log=%s", number, finished.Status, finished.Log)
		}
		return finished
	}
	first := runBuild(1, false)
	if err := os.Mkdir(filepath.Join(repository, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "src", "service.go"), []byte("package service\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runRunnerSCMGit(t, "-C", repository, "add", "src/service.go")
	runRunnerSCMGit(t, "-C", repository, "commit", "-m", "feat: add service")
	second := runBuild(2, true)
	checkout, err := database.JenkinsSCMCheckoutForBuild(second.ID)
	if err != nil || checkout == nil || checkout.BaseRevision != first.CommitSHA || !checkout.HistoryKnown || !strings.Contains(checkout.ChangesJSON, "src/service.go") {
		t.Fatalf("checkout=%+v err=%v", checkout, err)
	}
	if err := runner.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Recovery replays the same build from an empty workspace. Its persisted
	// snapshot must not become visible before the checkout step re-executes.
	if err := database.UpdateBuildStatus(second.ID, "pending"); err != nil {
		t.Fatal(err)
	}
	runner = engine.NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	runner.Run(second.ID)
	replayed := waitForRunnerSCMBuild(t, database, second.ID, 20*time.Second)
	if replayed.Status != "success" || strings.Contains(replayed.Log, "BEFORE_CHECKOUT_INCORRECT") || strings.Count(replayed.Log, "SCM_CHANGED_EXECUTED") != 2 {
		t.Fatalf("replayed status=%s log=%s", replayed.Status, replayed.Log)
	}
	if err := runner.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	database, err = store.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	runner = engine.NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runBuild(3, false)
}

func TestJenkinsSCMConditionsUseDefaultCheckout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	runRunnerSCMGit(t, "init", repository)
	runRunnerSCMGit(t, "-C", repository, "config", "user.email", "test@example.invalid")
	runRunnerSCMGit(t, "-C", repository, "config", "user.name", "Test")
	source := `pipeline { agent any stages { stage('changed') { when { changeset '*.txt' } steps { sh 'echo DEFAULT_CHECKOUT_CHANGED' } } } }`
	if err := os.WriteFile(filepath.Join(repository, "Jenkinsfile"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	runRunnerSCMGit(t, "-C", repository, "add", "Jenkinsfile")
	runRunnerSCMGit(t, "-C", repository, "commit", "-m", "initial")
	runRunnerSCMGit(t, "-C", repository, "branch", "-M", "main")
	database, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject("default-scm-conditions", "", "", "git", "main", "", 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectPipelineSource(project.ID, "jenkinsfile", "scm", repository, "main", "Jenkinsfile"); err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	for number := 1; number <= 2; number++ {
		if number == 2 {
			if err := os.WriteFile(filepath.Join(repository, "change.txt"), []byte("changed"), 0o644); err != nil {
				t.Fatal(err)
			}
			runRunnerSCMGit(t, "-C", repository, "add", "change.txt")
			runRunnerSCMGit(t, "-C", repository, "commit", "-m", "change")
		}
		build, err := database.CreateBuild(project.ID, number, "manual", "main", "", "", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		runner.Run(build.ID)
		finished := waitForRunnerSCMBuild(t, database, build.ID, 20*time.Second)
		if finished.Status != "success" || strings.Contains(finished.Log, "DEFAULT_CHECKOUT_CHANGED") != (number == 2) {
			t.Fatalf("build=%d status=%s log=%s", number, finished.Status, finished.Log)
		}
	}
}

func TestJenkinsSCMConditionsFailClosedForUnknownLegacyHistory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	runRunnerSCMGit(t, "init", repository)
	runRunnerSCMGit(t, "-C", repository, "config", "user.email", "test@example.invalid")
	runRunnerSCMGit(t, "-C", repository, "config", "user.name", "Test")
	source := `pipeline { agent any stages { stage('release') { when { not { changeset '*.txt' } } steps { sh 'echo LEGACY_HISTORY_RELEASED' } } } }`
	if err := os.WriteFile(filepath.Join(repository, "Jenkinsfile"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	runRunnerSCMGit(t, "-C", repository, "add", "Jenkinsfile")
	runRunnerSCMGit(t, "-C", repository, "commit", "-m", "initial")
	runRunnerSCMGit(t, "-C", repository, "branch", "-M", "main")
	database, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject("legacy-scm-history", "", "", "git", "main", "", 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectPipelineSource(project.ID, "jenkinsfile", "scm", repository, "main", "Jenkinsfile"); err != nil {
		t.Fatal(err)
	}
	old, err := database.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.FinishBuild(old.ID, "success", 1); err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	for number := 2; number <= 3; number++ {
		name := fmt.Sprintf("change-%d.txt", number)
		if err := os.WriteFile(filepath.Join(repository, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		runRunnerSCMGit(t, "-C", repository, "add", name)
		runRunnerSCMGit(t, "-C", repository, "commit", "-m", "change")
		build, err := database.CreateBuild(project.ID, number, "manual", "main", "", "", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		runner.Run(build.ID)
		finished := waitForRunnerSCMBuild(t, database, build.ID, 20*time.Second)
		if strings.Contains(finished.Log, "LEGACY_HISTORY_RELEASED") {
			t.Fatalf("build=%d released with unknown or changed history: %s", number, finished.Log)
		}
		if number == 2 && (finished.Status != "failed" || !strings.Contains(finished.Log, "checkout history is unavailable")) {
			t.Fatalf("legacy history build status=%s log=%s", finished.Status, finished.Log)
		}
		if number == 3 && finished.Status != "success" {
			t.Fatalf("subsequent build status=%s log=%s", finished.Status, finished.Log)
		}
	}
}

package engine_test

import (
	"context"
	"fmt"
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
options { skipDefaultCheckout() }
stages {
stage('before') { when { changeset 'src/**/*.go' } steps { sh 'echo BEFORE_CHECKOUT_INCORRECT' } }
stage('checkout') { steps { checkout scm } }
stage('changed') { when { allOf { changeset pattern: 'src/**/*.go', caseSensitive: true; changelog 'feat:.*' } } steps { sh 'echo SCM_CHANGED_EXECUTED' } }
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

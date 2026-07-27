package engine_test

import (
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

func TestBuildRunnerReusesSCMWorkspaceForLegacyJenkinsClone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is not available")
	}
	// Prefer Git's bundled bash on Windows. WSL's placeholder bash.exe can be
	// present without a configured distribution and must not make this SCM
	// regression test falsely fail before the Jenkinsfile reaches its stage.
	t.Setenv("PATH", filepath.Dir(shPath)+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	runRunnerSCMGit(t, "init", repository)
	runRunnerSCMGit(t, "-C", repository, "config", "user.email", "buildworld-test@example.invalid")
	runRunnerSCMGit(t, "-C", repository, "config", "user.name", "BuildWorld Test")
	cloneURL := strings.ReplaceAll(filepath.ToSlash(repository), "'", "'\\''")
	jenkinsfile := `
pipeline {
  agent any
  stages {
    stage('Checkout Latest Main') {
      steps {
        deleteDir()
        checkout scm
        sh '''
          set -eu
          cd "$WORKSPACE"
          git clone --depth 1 --branch main '` + cloneURL + `' .
          test -f marker.txt
          echo 'MARKER_OK'
          git rev-parse --is-inside-work-tree
        '''
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(repository, "Jenkinsfile"), []byte(jenkinsfile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "marker.txt"), []byte("SCM workspace marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runRunnerSCMGit(t, "-C", repository, "add", "Jenkinsfile", "marker.txt")
	runRunnerSCMGit(t, "-C", repository, "commit", "-m", "initial")
	runRunnerSCMGit(t, "-C", repository, "branch", "-M", "main")

	database, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject("scm-workspace", "", "", "git", "main", "", 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectPipelineSource(project.ID, "jenkinsfile", "scm", repository, "main", "Jenkinsfile"); err != nil {
		t.Fatal(err)
	}
	build, err := database.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	runner := engine.NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	if err := runner.ConfigureExecutionPolicy(engine.ExecutionPolicy{
		DefaultTimeoutSec:        30,
		MaxConcurrentBuilds:      1,
		MaxConcurrentLocalBuilds: 1,
		RetryLimit:               0,
		FailFast:                 true,
		CPUPercent:               100,
	}); err != nil {
		t.Fatal(err)
	}
	runner.Run(build.ID)
	finished := waitForRunnerSCMBuild(t, database, build.ID, 10*time.Second)
	if finished.Status != "success" {
		t.Fatalf("SCM Jenkins build status = %q\n%s", finished.Status, finished.Log)
	}
	for _, expected := range []string{
		"[SCM Checkout] Checking out Jenkins SCM repository",
		"workspace already contains the SCM checkout; skipping legacy git clone into .",
		"[Checkout Latest Main] MARKER_OK",
		"[Checkout Latest Main] true",
	} {
		if !strings.Contains(finished.Log, expected) {
			t.Fatalf("SCM Jenkins build log missing %q:\n%s", expected, finished.Log)
		}
	}
}

func waitForRunnerSCMBuild(t *testing.T, database *store.Store, buildID int64, timeout time.Duration) *store.Build {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		build, err := database.GetBuild(buildID)
		if err != nil {
			t.Fatal(err)
		}
		if build.Status == "success" || build.Status == "failed" || build.Status == "cancelled" {
			return build
		}
		time.Sleep(20 * time.Millisecond)
	}
	build, err := database.GetBuild(buildID)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("SCM Jenkins build did not finish within %s; status=%q\n%s", timeout, build.Status, build.Log)
	return nil
}

func runRunnerSCMGit(t *testing.T, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
}

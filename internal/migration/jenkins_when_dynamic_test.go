package migration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestJenkinsWhenSeesPriorBuildOutputWithoutLeakingStageEnvironment(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BUILDWORLD_TEST_STAGE_ONLY_3191", "")
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := `pipeline { agent any
environment { MODE = 'base' }
stages {
stage('produce') { environment { BUILDWORLD_TEST_STAGE_ONLY_3191 = 'local' } steps { sh 'echo "::buildworld:set MODE=production"; echo "::buildworld:set BRANCH_NAME=release/v1"' } }
stage('condition') { when { allOf { environment name: 'MODE', value: 'production'; environment name: 'BUILDWORLD_TEST_STAGE_ONLY_3191', value: ''; branch 'release/**'; expression { env.MODE == 'production' } } } steps { sh 'echo UPDATED_WHEN_EXECUTED' } }
stage('old-condition') { when { environment name: 'MODE', value: 'base' } steps { sh 'echo STALE_WHEN_INCORRECT' } }
} }`
	project, err := db.CreateProject("dynamic-when-environment", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", `{}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(db, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		build, err = db.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if build.Status == "failed" || build.Status == "success" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if build.Status != "success" || !strings.Contains(build.Log, "UPDATED_WHEN_EXECUTED") || strings.Contains(build.Log, "STALE_WHEN_INCORRECT") {
		t.Fatalf("status=%s log=%s", build.Status, build.Log)
	}
}

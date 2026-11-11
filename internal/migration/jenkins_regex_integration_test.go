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

func TestJenkinsRegexRunsInLocalBuild(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "regex.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := `pipeline {
 agent any
 environment { BRANCH_NAME = 'release-44' }
 stages {
  stage('match') {
   when { branch pattern: 'release-(?=[0-9]{2}$)[0-9]+', comparator: 'REGEXP' }
   steps { sh 'echo regex-matched' }
  }
  stage('backref') {
   when { branch pattern: 'release-([0-9])\\1', comparator: 'REGEXP' }
   steps { sh 'echo regex-backref' }
  }
  stage('skip') {
   when { branch pattern: 'release-(?=[0-9]{3}$)[0-9]+', comparator: 'REGEXP' }
   steps { sh 'echo regex-wrong' }
  }
 }
}`
	project, err := db.CreateProject("regex", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
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
		if build.Status == "success" || build.Status == "failed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if build.Status != "success" || !strings.Contains(build.Log, "regex-matched") ||
		!strings.Contains(build.Log, "regex-backref") || strings.Contains(build.Log, "regex-wrong") {
		t.Fatalf("unexpected build status=%s log=%s", build.Status, build.Log)
	}
}

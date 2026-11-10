package migration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestSelfReleaseJenkinsfileRetainsRequiredParametersAndCommand(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(source), "..", "..", "Jenkinsfile"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := engine.ParseJenkinsfileConfig(string(data), "self-release")
	if err != nil {
		t.Fatal(err)
	}
	if !config.DisableConcurrent || len(config.Stages) != 1 || len(config.Stages[0].Steps) != 1 {
		t.Fatalf("unexpected self-release pipeline: %+v", config)
	}
	if !strings.Contains(config.Stages[0].Steps[0].Command, "go run ./cmd/selfrelease") {
		t.Fatalf("self-release command missing: %+v", config.Stages[0].Steps)
	}
	names := map[string]bool{}
	for _, parameter := range config.Parameters {
		names[parameter.Name] = true
	}
	if !names["RELEASE_VERSION"] || !names["SOURCE_COMMIT"] {
		t.Fatalf("required parameters missing: %+v", config.Parameters)
	}
}

func TestSelfReleaseJenkinsfilePassesParametersToBuiltinStep(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	stub := "#!/bin/sh\nprintf 'release=%s commit=%s\\n' \"$RELEASE_VERSION\" \"$SOURCE_COMMIT\"\n"
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, file, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "Jenkinsfile"))
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject("self-release", "", "", "git", "main", string(data), 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectPipelineSource(project.ID, "jenkinsfile", "inline", "", "", ""); err != nil {
		t.Fatal(err)
	}
	build, err := database.CreateBuild(project.ID, 1, "manual", "main", "", `{"RELEASE_VERSION":"1.2.3","SOURCE_COMMIT":"1234567890"}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(database, nil, filepath.Join(root, "workspace"), nil)
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	deadline := time.Now().Add(20 * time.Second)
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
	if build.Status != "success" || !strings.Contains(build.Log, "release=1.2.3 commit=1234567890") {
		t.Fatalf("status=%s log=%s", build.Status, build.Log)
	}
}

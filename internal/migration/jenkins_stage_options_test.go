package migration

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
	"github.com/neko233-com/buildworld/internal/store"
)

func TestJenkinsStageTimeoutRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		options string
		seconds int
	}{
		{"timeout(time: 2, unit: 'SECONDS')", 2},
		{"timeout(time: 3)", 180},
		{"timeout(unit: 'HOURS', time: 2)", 7200},
		{"timeout(time: 1, unit: 'DAYS')", 86400},
	} {
		t.Run(tc.options, func(t *testing.T) {
			source := fmt.Sprintf(`pipeline { agent any stages { stage('sync') { options { %s } when { branch 'main' } steps { sh 'git fetch origin' } } } }`, tc.options)
			config, err := engine.ParseJenkinsfileConfig(source, "timeout")
			if err != nil {
				t.Fatal(err)
			}
			if len(config.Stages) != 1 || config.Stages[0].TimeoutSec != tc.seconds {
				t.Fatalf("stages=%+v, want timeout %d", config.Stages, tc.seconds)
			}
		})
	}
}

func TestJenkinsUnsupportedStageGuardsNeverExecuteUnconditionally(t *testing.T) {
	for _, body := range []string{
		`input { message 'Deploy?' } steps { sh 'echo deploy' }`,
		`steps { input 'Deploy?'; sh 'echo deploy' }`,
		`steps { input /* require approval */ 'Deploy?'; sh 'echo deploy' }`,
		`steps { script { input message: 'Deploy?'; sh 'echo deploy' } }`,
		`options { lock(resource: 'production') } steps { sh 'echo deploy' }`,
		`options { timeout(time: 1, unit: 'MILLISECONDS') } steps { sh 'echo deploy' }`,
		`options { timeout(time: 1, activity: true) } steps { sh 'echo deploy' }`,
		`options { timeout(time: 0) } steps { sh 'echo deploy' }`,
		`options { timeout(time: 1); timeout(time: 2) } steps { sh 'echo deploy' }`,
		`post { always { echo 'cleanup' } } steps { sh 'echo deploy' }`,
		`parallel { stage('one') { steps { sh 'echo one' } } stage('two') { steps { sh 'echo two' } } }`,
		`stages { stage('inner') { steps { sh 'echo inner' } } }`,
		`agent none steps { sh 'echo deploy' }`,
		`agent { label 'another-host' } steps { sh 'echo deploy' }`,
	} {
		t.Run(body, func(t *testing.T) {
			source := fmt.Sprintf(`pipeline { agent any stages { stage('guarded') { %s } } }`, body)
			if _, err := engine.ParseJenkinsfileConfig(source, "guard"); err == nil {
				t.Fatalf("silently accepted %s", body)
			}
		})
	}
	if _, err := engine.ParseJenkinsfileConfig(`pipeline { agent any stages { stage('safe') { steps { sh 'echo input'; echo 'input' } } } }`, "safe"); err != nil {
		t.Fatalf("quoted input text is not an approval step: %v", err)
	}
	if _, err := engine.ParseJenkinsfileConfig(`pipeline { agent none stages { stage('unsafe') { steps { sh 'echo deploy' } } } }`, "unsafe"); err == nil {
		t.Fatal("agent none silently gained a local executor")
	}
	if _, err := engine.ParseJenkinsfileConfig(`pipeline { agent none stages { stage('safe') { agent any steps { sh 'echo safe' } } } }`, "safe"); err != nil {
		t.Fatalf("agent any must enable a local stage: %v", err)
	}
	if _, err := engine.ParseJenkinsfileConfig(`node { stage('deploy') { input 'Approve?'; sh 'echo deploy' } }`, "scripted"); err == nil {
		t.Fatal("scripted Jenkins input was silently bypassed")
	}
}

func TestJenkinsStageTimeoutStopsActualStep(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	data, err := store.New(filepath.Join(root, "timeout.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	source := `pipeline { agent any stages { stage('bounded') { options { timeout(time: 1, unit: 'SECONDS') } steps { sh 'sleep 8; echo must-not-finish' } } } }`
	project, err := data.CreateProject("timeout", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(data, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		build, err = data.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if build.Status == "failed" || build.Status == "success" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if build.Status != "failed" || !strings.Contains(strings.ToLower(build.Log), "timed out") || strings.Contains(build.Log, "must-not-finish") {
		t.Fatalf("timeout did not stop the stage: status=%s log=%s", build.Status, build.Log)
	}
}

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

func TestJenkinsWhenBooleanParametersAndRoundTrip(t *testing.T) {
	source := `pipeline { agent any
parameters { booleanParam(name: 'ENTRY_ONLY', defaultValue: false) }
stages { stage('deploy') { when { expression { !params.ENTRY_ONLY } } steps { sh 'echo deploy' } } } }`
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: source})
	if err != nil {
		t.Fatal(err)
	}
	config, err := engine.ParsePipelineConfig(result.Config)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Stages) != 1 || config.Stages[0].JenkinsWhen != "!params.ENTRY_ONLY" {
		t.Fatalf("unexpected stages: %#v", config.Stages)
	}
	for _, entryOnly := range []bool{true, false} {
		got, err := engine.ShouldRunStageWithParameters(config.Stages[0], nil, map[string]interface{}{"ENTRY_ONLY": entryOnly})
		if err != nil || got == entryOnly {
			t.Fatalf("ENTRY_ONLY=%v got=%v err=%v", entryOnly, got, err)
		}
	}
	if _, err := engine.ShouldRunStage(config.Stages[0], nil); err == nil {
		t.Fatal("execution without parameters must fail closed")
	}
}

func TestJenkinsWhenSDKReleaseStringChoice(t *testing.T) {
	source := `pipeline { agent any stages { stage('verify') { when { expression { return !params.SYNC_PLATFORM_APP_IDS && !params.SYNC_SHARED_DEPENDENCIES && params.RELEASE_ACTION == 'publish-latest' } } steps { sh 'echo verify' } } } }`
	config, err := engine.ParseJenkinsfileConfig(source, "sdk")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		action string
		sync   bool
		want   bool
	}{{"publish-latest", false, true}, {"restart-version", false, false}, {"publish-latest", true, false}} {
		params := map[string]interface{}{"RELEASE_ACTION": test.action, "SYNC_PLATFORM_APP_IDS": test.sync, "SYNC_SHARED_DEPENDENCIES": false}
		got, err := engine.ShouldRunStageWithParameters(config.Stages[0], nil, params)
		if err != nil || got != test.want {
			t.Fatalf("%+v got %v error %v", test, got, err)
		}
	}
}

func TestJenkinsWhenUnsupportedNeverBecomesUnconditional(t *testing.T) {
	for _, condition := range []string{
		`not { branch 'main' }`, `allOf { branch 'main'; expression { !params.ENTRY_ONLY } }`,
		`branch 'main'; branch 'release'`, `branch 'release/*'`, `expression { dangerous() }`,
		`expression { params.ENTRY_ONLY }; branch 'main'`, `environment name: 'ENV', value: 'prod'`,
		`expression { params.ACTION == "${params.ACTION}" }`,
	} {
		t.Run(condition, func(t *testing.T) {
			source := fmt.Sprintf(`pipeline { agent any stages { stage('deploy') { when { %s } steps { sh 'echo deploy' } } } }`, condition)
			_, err := NewJenkinsfileStrategy().Convert(Request{Source: source})
			if err == nil || !strings.Contains(err.Error(), "when") {
				t.Fatalf("condition accepted: %v", err)
			}
		})
	}
}

func TestConditionalJenkinsWatcherDoesNotRemoveTimeout(t *testing.T) {
	config := &engine.BuildConfig{Stages: []engine.Stage{{Name: "watch", JenkinsWhen: "!params.ENTRY_ONLY", Steps: []engine.Step{{Type: "shell", Command: "go run ./tools/deploygame_cli ops resume-game-monitor --monitor-only=true"}}}}}
	if err := engine.ValidatePipelineSemantics(config); err != nil {
		t.Fatal(err)
	}
	if config.AllowLongRunning {
		t.Fatal("conditional watcher disabled the default timeout")
	}
	config.AgentRequirements = []string{"remote"}
	if err := engine.ValidatePipelineSemantics(config); err == nil {
		t.Fatal("remote worker must not silently drop a Jenkins condition")
	}
}

func TestJenkinsWhenMissingAndWrongTypeFail(t *testing.T) {
	for _, params := range []map[string]interface{}{{}, {"ENTRY_ONLY": "false"}} {
		if _, err := engine.EvaluateJenkinsWhen("!params.ENTRY_ONLY", params); err == nil {
			t.Fatalf("accepted %#v", params)
		}
	}
}

func TestJenkinsWhenRunsThroughRealLocalRunner(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is not available")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := `pipeline { agent any
parameters { booleanParam(name: 'ENTRY_ONLY', defaultValue: false) }
stages { stage('guarded') { when { expression { !params.ENTRY_ONLY } } steps { sh 'echo guarded-command-executed' } }
stage('always') { steps { sh 'echo always-command-executed' } } } }`
	project, err := db.CreateProject("jenkins-when", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(db, nil, filepath.Join(root, "workspaces"), nil)
	runner.SetArtifactManager(engine.NewArtifactManager(db, filepath.Join(root, "artifacts")))
	defer runner.Shutdown(context.Background())
	for i, entryOnly := range []bool{true, false} {
		build, err := db.CreateBuild(project.ID, i+1, "manual", "main", "", fmt.Sprintf(`{"ENTRY_ONLY":%t}`, entryOnly), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
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
		if build.Status != "success" {
			t.Fatalf("entry=%v status=%s log=%s", entryOnly, build.Status, build.Log)
		}
		if got := strings.Contains(build.Log, "guarded-command-executed"); got == entryOnly {
			t.Fatalf("entry=%v log=%s", entryOnly, build.Log)
		}
		if !strings.Contains(build.Log, "always-command-executed") {
			t.Fatalf("unconditional stage missing: %s", build.Log)
		}
	}
}

package engine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/neko233-com/buildworld/internal/rpc/generated"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestUnknownPluginStepReturnsClearError(t *testing.T) {
	runner := &BuildRunner{executor: NewExecutor()}
	err := runner.execStep(context.Background(), Step{Type: "missing:step"}, t.TempDir(), &store.Project{}, &store.Build{}, nil, nil, "plugin", func(string) {})
	if err == nil || err.Error() != "unsupported step type: missing:step" {
		t.Fatalf("unknown plugin step error = %v", err)
	}
}

func TestCompletedStepErrorHonorsCancellationAtSuccessBoundary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := completedStepError(ctx, nil); err != context.Canceled {
		t.Fatalf("completed step error = %v, want context.Canceled", err)
	}
	commandErr := fmt.Errorf("command failed")
	if err := completedStepError(context.Background(), commandErr); err != commandErr {
		t.Fatalf("command error = %v, want original error", err)
	}
	if err := completedStepError(context.Background(), nil); err != nil {
		t.Fatalf("successful step error = %v, want nil", err)
	}
}

func TestReceiveRemoteArtifactVerifiesAndSaves(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.New(filepath.Join(tmp, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject("artifact-test", "", "", "git", "main", testEmptyPipelineSource, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := NewBuildRunner(db, nil, filepath.Join(tmp, "workspace"), nil)
	runner.SetArtifactManager(NewArtifactManager(db, filepath.Join(tmp, "artifacts")))
	pending := map[string]*remoteArtifactBuffer{}
	payload := []byte("remote artifact payload")
	if err := runner.receiveRemoteArtifact(build.ID, pending, &pb.ArtifactChunk{Name: "release.txt", Data: payload[:7]}); err != nil {
		t.Fatal(err)
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256(payload))
	if err := runner.receiveRemoteArtifact(build.ID, pending, &pb.ArtifactChunk{Name: "release.txt", Data: payload[7:], FinalChunk: true, Sha256: checksum}); err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatal("finalized artifact remained pending")
	}
	artifacts, err := db.ListArtifactsByBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("artifact count = %d, want 1", len(artifacts))
	}
	got, err := os.ReadFile(artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("artifact payload = %q, want %q", got, payload)
	}
}

func TestBuildRunnerDispatchesDurablePendingBuilds(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.New(filepath.Join(tmp, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject("queue-test", "", "", "git", "main", testEmptyPipelineSource, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := NewBuildRunner(db, nil, filepath.Join(tmp, "workspace"), nil)
	runner.dispatchPending()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current, err := db.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == "success" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	current, err := db.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("durable queue left build in %q", current.Status)
}

func TestResolveBuildParametersAppliesDefaultsAndCopiesInput(t *testing.T) {
	supplied := map[string]interface{}{"environment": "production"}
	resolved, err := ResolveBuildParameters([]BuildParameter{
		{Name: "environment", Type: "choice", Choices: []string{"staging", "production"}, Required: true},
		{Name: "release", Type: "boolean", Default: false},
		{Name: "retries", Type: "number", Default: 2},
	}, supplied)
	if err != nil {
		t.Fatalf("ResolveBuildParameters() error = %v", err)
	}
	if release, ok := resolved["release"].(bool); !ok || release {
		t.Fatalf("release default = %#v, want false", resolved["release"])
	}
	if retries, ok := resolved["retries"].(int); !ok || retries != 2 {
		t.Fatalf("retries default = %#v, want int(2)", resolved["retries"])
	}
	if _, exists := supplied["release"]; exists {
		t.Fatal("ResolveBuildParameters mutated the supplied map")
	}

	implicit, err := ResolveBuildParameters([]BuildParameter{
		{Name: "target", Type: "choice", Choices: []string{"staging", "production"}, Required: true},
		{Name: "dry_run", Type: "boolean", Required: true},
	}, nil)
	if err != nil {
		t.Fatalf("ResolveBuildParameters() implicit defaults error = %v", err)
	}
	if implicit["target"] != "staging" || implicit["dry_run"] != false {
		t.Fatalf("implicit defaults = %#v, want first choice and false", implicit)
	}
}

func TestResolveBuildParametersRejectsInvalidDefinitionsAndValues(t *testing.T) {
	tests := []struct {
		name        string
		definitions []BuildParameter
		supplied    map[string]interface{}
	}{
		{
			name:        "missing required",
			definitions: []BuildParameter{{Name: "token", Type: "password", Required: true}},
			supplied:    map[string]interface{}{},
		},
		{
			name:        "blank required",
			definitions: []BuildParameter{{Name: "token", Type: "password", Required: true}},
			supplied:    map[string]interface{}{"token": "   "},
		},
		{
			name:        "invalid choice",
			definitions: []BuildParameter{{Name: "target", Type: "choice", Choices: []string{"staging", "production"}}},
			supplied:    map[string]interface{}{"target": "development"},
		},
		{
			name: "duplicate declaration",
			definitions: []BuildParameter{
				{Name: "target", Type: "string"},
				{Name: "target", Type: "string"},
			},
			supplied: map[string]interface{}{},
		},
		{
			name:        "undeclared supplied value",
			definitions: []BuildParameter{{Name: "target", Type: "string"}},
			supplied:    map[string]interface{}{"target": "production", "extra": true},
		},
		{
			name:        "unsupported type",
			definitions: []BuildParameter{{Name: "target", Type: "arbitrary"}},
			supplied:    map[string]interface{}{"target": "production"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ResolveBuildParameters(test.definitions, test.supplied); err == nil {
				t.Fatal("ResolveBuildParameters() error = nil, want validation error")
			}
		})
	}
}

func TestTypeScriptBuildConfigParse(t *testing.T) {
	source := `import { definePipeline, parameter } from "@buildworld/pipeline"
export default definePipeline({
  name: "test-build",
  parameters: [
    parameter("env", "choice", { choices: ["dev", "prod"], required: true }),
    parameter("debug", "boolean", { default: false }),
  ],
  stages: [],
})`

	config, err := ParsePipelineConfig(source)
	if err != nil {
		t.Fatalf("ParsePipelineConfig() error = %v", err)
	}

	if config.Name != "test-build" {
		t.Errorf("Name = %s, want test-build", config.Name)
	}

	if len(config.Parameters) != 2 {
		t.Errorf("Parameters length = %d, want 2", len(config.Parameters))
	}
}

func TestYAMLBuildConfigParsesParametersAndJobs(t *testing.T) {
	config, err := ParseYAMLConfig(`
name: yaml-parameters
parameters:
  - name: target
    type: choice
    choices: [staging, production]
    default: staging
    required: true
  - name: api_token
    type: password
    is_secret: true
jobs:
  build:
    name: Build
    steps:
      - name: Package
        run: echo package
artifacts:
  - dist/**
agent_requirements:
  - os=windows
retention_completed: 15
`)
	if err != nil {
		t.Fatalf("ParseYAMLConfig() error = %v", err)
	}
	if len(config.Parameters) != 2 || config.Parameters[1].Name != "api_token" || !config.Parameters[1].IsSecret {
		t.Fatalf("parameters = %#v, want parsed secret parameter", config.Parameters)
	}
	if len(config.Stages) != 1 || len(config.Stages[0].Steps) != 1 || config.Stages[0].Steps[0].Command != "echo package" {
		t.Fatalf("stages = %#v, want one linear shell step", config.Stages)
	}
	if len(config.Artifacts) != 1 || config.RetentionCompleted != 15 {
		t.Fatalf("artifact/retention fields not preserved: %#v", config)
	}
}

func TestStageMatchesBranch(t *testing.T) {
	stage := Stage{Branches: []string{"main", "release"}}
	if !stageMatchesBranch(stage, "main") || !stageMatchesBranch(stage, "release") {
		t.Fatal("configured branches should run")
	}
	if stageMatchesBranch(stage, "feature/import") {
		t.Fatal("unconfigured branch should be skipped")
	}
	if !stageMatchesBranch(Stage{}, "feature/import") {
		t.Fatal("stage without a branch condition should run")
	}
}

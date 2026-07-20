package engine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/plugin"
	pb "github.com/neko233-com/buildworld/internal/rpc/generated"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestBuildManagerCreate(t *testing.T) {
	m := NewBuildManager()

	config := &BuildConfig{
		Name: "test-build",
		Parameters: []BuildParameter{
			{Name: "env", Type: "choice", Choices: []string{"dev", "prod"}, Required: true},
			{Name: "debug", Type: "boolean", Default: false},
		},
	}

	params := map[string]interface{}{
		"env": "dev",
	}

	build := m.CreateBuild(1, config, params)
	if build == nil {
		t.Fatal("CreateBuild() returned nil")
	}

	if build.Number != 1 {
		t.Errorf("Number = %d, want 1", build.Number)
	}

	if build.Status != "pending" {
		t.Errorf("Status = %s, want pending", build.Status)
	}
}

func TestPluginOutputsFlowToFollowingNodes(t *testing.T) {
	pluginRoot := t.TempDir()
	loader := plugin.NewLoader(pluginRoot)
	if err := loader.InstallPlugin("emit", "1.0.0", "test", "test", `registerStep("emit-output", function(ctx) { ctx.output("IMAGE", "registry.local/app:42"); ctx.env("REGION", "ap-southeast-1"); });`, "", "js", "", "", "none"); err != nil {
		t.Fatal(err)
	}
	if err := loader.Load("emit"); err != nil {
		t.Fatal(err)
	}
	runner := &BuildRunner{plugins: loader, executor: NewExecutor()}
	var lines []string
	if err := runner.execStep(context.Background(), Step{Type: "emit-output"}, pluginRoot, &store.Project{}, &store.Build{}, nil, nil, "plugin", func(line string) { lines = append(lines, line) }); err != nil {
		t.Fatal(err)
	}
	env := []string{}
	for _, line := range lines {
		env = appendBuildKV(env, line)
	}
	if got := resolveVars("deploy ${build.IMAGE} to ${build.REGION}", env, nil); got != "deploy registry.local/app:42 to ap-southeast-1" {
		t.Fatalf("plugin outputs = %q", got)
	}
}

func TestReceiveRemoteArtifactVerifiesAndSaves(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.New(filepath.Join(tmp, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject("artifact-test", "", "", "git", "main", `{"stages":[]}`, 0, nil, nil)
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
	project, err := db.CreateProject("queue-test", "", "", "git", "main", `{"stages":[]}`, 0, nil, nil)
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

func TestBuildManagerValidate(t *testing.T) {
	m := NewBuildManager()

	config := &BuildConfig{
		Parameters: []BuildParameter{
			{Name: "env", Type: "choice", Choices: []string{"dev", "prod"}, Required: true},
			{Name: "count", Type: "string"},
			{Name: "retries", Type: "number"},
		},
	}

	// Missing required param
	err := m.ValidateParameters(config.Parameters, map[string]interface{}{})
	if err == nil {
		t.Error("Validate() should fail for missing required param")
	}

	// Invalid choice
	err = m.ValidateParameters(config.Parameters, map[string]interface{}{"env": "staging"})
	if err == nil {
		t.Error("Validate() should fail for invalid choice")
	}

	// Valid params
	err = m.ValidateParameters(config.Parameters, map[string]interface{}{"env": "dev"})
	if err != nil {
		t.Errorf("Validate() error = %v", err)
	}

	err = m.ValidateParameters(config.Parameters, map[string]interface{}{"env": "dev", "retries": "3"})
	if err == nil {
		t.Error("Validate() should reject a string for a number parameter")
	}
}

func TestResolveBuildParametersAppliesDefaultsAndCopiesInput(t *testing.T) {
	supplied := map[string]interface{}{"environment": "production", "extension_value": "kept"}
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
	if resolved["extension_value"] != "kept" {
		t.Fatalf("unknown extension parameter was not preserved: %#v", resolved)
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

func TestBuildManagerGetBuilds(t *testing.T) {
	m := NewBuildManager()

	config := &BuildConfig{
		Name: "test-build",
		Parameters: []BuildParameter{
			{Name: "env", Type: "string"},
		},
	}

	m.CreateBuild(1, config, map[string]interface{}{"env": "dev"})
	m.CreateBuild(1, config, map[string]interface{}{"env": "prod"})
	m.CreateBuild(2, config, map[string]interface{}{"env": "dev"})

	builds := m.GetBuilds(1)
	if len(builds) != 2 {
		t.Errorf("GetBuilds(1) length = %d, want 2", len(builds))
	}

	builds = m.GetBuilds(2)
	if len(builds) != 1 {
		t.Errorf("GetBuilds(2) length = %d, want 1", len(builds))
	}
}

func TestBuildManagerGetBuild(t *testing.T) {
	m := NewBuildManager()

	config := &BuildConfig{
		Name: "test-build",
		Parameters: []BuildParameter{
			{Name: "env", Type: "string"},
		},
	}

	m.CreateBuild(1, config, map[string]interface{}{"env": "dev"})
	m.CreateBuild(1, config, map[string]interface{}{"env": "prod"})

	build := m.GetBuild(1, 1)
	if build == nil {
		t.Fatal("GetBuild(1, 1) returned nil")
	}

	if build.Number != 1 {
		t.Errorf("Number = %d, want 1", build.Number)
	}

	build = m.GetBuild(1, 2)
	if build == nil {
		t.Fatal("GetBuild(1, 2) returned nil")
	}

	if build.Number != 2 {
		t.Errorf("Number = %d, want 2", build.Number)
	}
}

func TestBuildConfigParse(t *testing.T) {
	jsonStr := `{
		"name": "test-build",
		"parameters": [
			{"name": "env", "type": "choice", "choices": ["dev", "prod"], "required": true},
			{"name": "debug", "type": "boolean", "default": false}
		]
	}`

	config, err := ParseBuildConfig(jsonStr)
	if err != nil {
		t.Fatalf("ParseBuildConfig() error = %v", err)
	}

	if config.Name != "test-build" {
		t.Errorf("Name = %s, want test-build", config.Name)
	}

	if len(config.Parameters) != 2 {
		t.Errorf("Parameters length = %d, want 2", len(config.Parameters))
	}
}

func TestYAMLBuildConfigParsesParametersAndLinearStages(t *testing.T) {
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
stages:
  - name: Build
    steps:
      - name: Package
        type: shell
        command: echo package
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

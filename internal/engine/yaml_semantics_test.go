package engine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestYAMLJobsPreserveDeclarationOrderAndStableDependencies(t *testing.T) {
	config, err := ParseYAMLConfig(`
name: ordered
jobs:
  deploy:
    needs: [test, lint]
    steps:
      - run: echo deploy
  test:
    needs: build
    steps:
      - run: echo test
  lint:
    needs:
      - build
    steps:
      - run: echo lint
  build:
    steps:
      - run: echo build
`)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, stage := range config.Stages {
		order = append(order, stage.ID)
	}
	if got := strings.Join(order, ","); got != "build,test,lint,deploy" {
		t.Fatalf("stable topological order = %q", got)
	}
	if got := strings.Join(config.Stages[3].DependsOn, ","); got != "test,lint" {
		t.Fatalf("list needs = %q", got)
	}
	if got := strings.Join(config.Stages[1].DependsOn, ","); got != "build" {
		t.Fatalf("scalar needs = %q", got)
	}

	declarationOnly, err := ParseYAMLConfig(`
jobs:
  zebra:
    steps: [{run: "echo zebra"}]
  alpha:
    steps: [{run: "echo alpha"}]
  middle:
    steps: [{run: "echo middle"}]
`)
	if err != nil {
		t.Fatal(err)
	}
	order = order[:0]
	for _, stage := range declarationOnly.Stages {
		order = append(order, stage.ID)
	}
	if got := strings.Join(order, ","); got != "zebra,alpha,middle" {
		t.Fatalf("declaration order = %q", got)
	}
}

func TestYAMLJobsRejectInvalidDependencyGraphs(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "unknown",
			yaml: "jobs:\n  build:\n    needs: missing\n    steps: [{run: echo build}]\n",
			want: "unknown job",
		},
		{
			name: "duplicate need",
			yaml: "jobs:\n  base:\n    steps: [{run: echo base}]\n  build:\n    needs: [base, base]\n    steps: [{run: echo build}]\n",
			want: "repeats dependency",
		},
		{
			name: "cycle",
			yaml: "jobs:\n  first:\n    needs: second\n    steps: [{run: echo first}]\n  second:\n    needs: first\n    steps: [{run: echo second}]\n",
			want: "dependency cycle",
		},
		{
			name: "duplicate job",
			yaml: "jobs:\n  build:\n    steps: [{run: echo one}]\n  build:\n    steps: [{run: echo two}]\n",
			want: "declared more than once",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseYAMLConfig(test.yaml)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestYAMLRejectsUnknownFieldsAndEmptyExecutionShapes(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "unknown root field",
			yaml: "jobz:\n  build:\n    steps: [{run: echo build}]\n",
			want: "field jobz not found",
		},
		{
			name: "singular job step field",
			yaml: "jobs:\n  build:\n    step:\n      - run: echo build\n",
			want: `unknown field "step"`,
		},
		{
			name: "misspelled run",
			yaml: "jobs:\n  build:\n    steps:\n      - name: build\n        runz: echo build\n",
			want: `unknown field "runz"`,
		},
		{
			name: "unknown defaults field",
			yaml: "jobs:\n  build:\n    defaults:\n      run:\n        working-directory: .\n        shel: sh\n    steps: [{run: echo build}]\n",
			want: `unknown field "shel"`,
		},
		{
			name: "empty jobs",
			yaml: "jobs: {}\n",
			want: "non-empty mapping",
		},
		{
			name: "null jobs",
			yaml: "jobs:\n",
			want: "non-empty mapping",
		},
		{
			name: "missing job steps",
			yaml: "jobs:\n  build:\n    runs-on: local\n",
			want: "at least one step",
		},
		{
			name: "empty job steps",
			yaml: "jobs:\n  build:\n    steps: []\n",
			want: "at least one step",
		},
		{
			name: "step without action",
			yaml: "jobs:\n  build:\n    steps:\n      - name: no action\n        env:\n          VALUE: ignored\n",
			want: "must define one supported action",
		},
		{
			name: "conflicting actions",
			yaml: "jobs:\n  build:\n    steps:\n      - uses: actions/checkout@v4\n        run: echo build\n",
			want: "cannot combine",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseYAMLConfig(test.yaml)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestYAMLSupportedStepFormsRemainCompatible(t *testing.T) {
	config, err := ParseYAMLConfig(`
jobs:
  build:
    steps:
      - uses: actions/checkout@v4
      - run: echo run
      - command: echo command
      - type: notify
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Stages) != 1 || len(config.Stages[0].Steps) != 4 {
		t.Fatalf("steps = %#v", config.Stages)
	}
	got := []string{
		config.Stages[0].Steps[0].Type,
		config.Stages[0].Steps[1].Type,
		config.Stages[0].Steps[2].Type,
		config.Stages[0].Steps[3].Type,
	}
	if strings.Join(got, ",") != "git,shell,shell,notify" {
		t.Fatalf("step types = %q", strings.Join(got, ","))
	}
}

func TestYAMLSafeConditionsUsesRunsOnAndDefaults(t *testing.T) {
	config, err := ParseYAMLConfig(`
defaults:
  run:
    shell: sh
    working-directory: shared
jobs:
  verify:
    runs-on: [self-hosted, macos]
    timeout-minutes: 2
    env:
      JOB_VALUE: job
    steps:
      - uses: actions/checkout@v4
      - name: verify
        if: ${{ success() && !failure() }}
        env:
          STEP_VALUE: step
        working-directory: nested
        run: echo ok
  package:
    runs-on: [macos, self-hosted]
    needs: verify
    if: always()
    steps:
      - run: echo package
`)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(config.AgentRequirements, ","); got != "macos,self-hosted" {
		t.Fatalf("agent requirements = %q", got)
	}
	verify := config.Stages[0]
	if verify.TimeoutSec != 120 || verify.Environment["JOB_VALUE"] != "job" {
		t.Fatalf("job timeout/env = %#v", verify)
	}
	if verify.Steps[0].Type != "git" {
		t.Fatalf("checkout step = %#v", verify.Steps[0])
	}
	step := verify.Steps[1]
	if step.Shell != "sh" || step.Config["working-directory"] != "nested" || step.Config["env.STEP_VALUE"] != "step" {
		t.Fatalf("run defaults/overrides = %#v", step)
	}

	local, err := ParseYAMLConfig("jobs:\n  build:\n    runs-on: local\n    steps: [{run: echo ok}]\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(local.AgentRequirements) != 0 {
		t.Fatalf("runs-on local mapped to remote requirements: %#v", local.AgentRequirements)
	}
	booleanCondition, err := ParseYAMLConfig("jobs:\n  disabled:\n    if: false\n    steps: [{run: echo disabled}]\n")
	if err != nil {
		t.Fatal(err)
	}
	if shouldRun, err := ShouldRunStage(booleanCondition.Stages[0], map[string]string{}); err != nil || shouldRun {
		t.Fatalf("boolean if result = %v, %v", shouldRun, err)
	}

	invalid := []string{
		"jobs:\n  one:\n    runs-on: local\n    steps: [{run: echo one}]\n  two:\n    runs-on: linux\n    steps: [{run: echo two}]\n",
		"jobs:\n  build:\n    steps:\n      - uses: docker/build-push-action@v6\n",
		"jobs:\n  build:\n    if: github.ref == 'main'\n    steps: [{run: echo build}]\n",
	}
	for _, source := range invalid {
		if _, err := ParseYAMLConfig(source); err == nil {
			t.Fatalf("invalid pipeline parsed successfully:\n%s", source)
		}
	}
}

func TestBuildRunnerYAMLDependencyFailureConditions(t *testing.T) {
	config := `
jobs:
  fail:
    steps:
      - name: expected failure
        run: exit 7
  blocked:
    needs: fail
    steps:
      - name: should not run
        run: echo SHOULD_NOT_RUN
  recover:
    needs: fail
    if: failure()
    steps:
      - name: recovery
        run: echo RECOVERY_RAN
  cleanup:
    needs: fail
    if: always()
    steps:
      - name: cleanup
        run: echo CLEANUP_RAN
  independent:
    steps:
      - name: independent
        run: echo INDEPENDENT_RAN
`
	finished, _ := runYAMLBuildForTest(t, config, false)
	if finished.Status != "failed" {
		t.Fatalf("build status = %q\n%s", finished.Status, finished.Log)
	}
	for _, expected := range []string{"RECOVERY_RAN", "CLEANUP_RAN", "INDEPENDENT_RAN", "dependency/if condition evaluated to false"} {
		if !strings.Contains(finished.Log, expected) {
			t.Fatalf("log does not contain %q:\n%s", expected, finished.Log)
		}
	}
	if strings.Contains(finished.Log, "SHOULD_NOT_RUN") {
		t.Fatalf("default dependent job ran after dependency failure:\n%s", finished.Log)
	}
}

func TestBuildRunnerYAMLAppliesEnvironmentAndWorkingDirectory(t *testing.T) {
	mkdirCommand := "mkdir -p nested"
	writeCommand := `printf '%s-%s' "$JOB_VALUE" "$STEP_VALUE" > value.txt`
	if runtime.GOOS == "windows" {
		mkdirCommand = "mkdir nested"
		writeCommand = "echo %JOB_VALUE%-%STEP_VALUE%>value.txt"
	}
	config := `
artifacts:
  - nested/value.txt
jobs:
  verify:
    runs-on: local
    env:
      JOB_VALUE: job
    defaults:
      run:
        working-directory: nested
    steps:
      - name: create directory
        working-directory: .
        run: ` + mkdirCommand + `
      - name: write value
        env:
          STEP_VALUE: step
        run: ` + writeCommand + `
      - name: notify safely
        type: notify
`
	finished, database := runYAMLBuildForTest(t, config, true)
	if finished.Status != "success" {
		t.Fatalf("build status = %q\n%s", finished.Status, finished.Log)
	}
	artifacts, err := database.ListArtifactsByBuild(finished.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %#v", artifacts)
	}
	content, err := os.ReadFile(artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(content)) != "job-step" {
		t.Fatalf("environment output = %q", content)
	}
	if !strings.Contains(finished.Log, "Notification skipped: no notification service is configured") || strings.Contains(finished.Log, "unsupported step type") {
		t.Fatalf("local notify did not use safe native semantics:\n%s", finished.Log)
	}
}

func TestBuildRunnerYAMLRejectsEscapingWorkingDirectoryAndTimesOutStage(t *testing.T) {
	escapeConfig := `
jobs:
  escape:
    steps:
      - name: escape
        working-directory: ..
        run: echo escaped
`
	escaped, _ := runYAMLBuildForTest(t, escapeConfig, true)
	if escaped.Status != "failed" || !strings.Contains(escaped.Log, "escapes workspace") {
		t.Fatalf("escape build = %q\n%s", escaped.Status, escaped.Log)
	}

	sleepCommand := "sleep 2"
	if runtime.GOOS == "windows" {
		sleepCommand = "ping -n 4 127.0.0.1 >nul"
	}
	timeoutConfig := `
jobs:
  timeout:
    timeout-minutes: 0.001
    steps:
      - name: wait
        run: ` + sleepCommand + `
`
	timedOut, _ := runYAMLBuildForTest(t, timeoutConfig, false)
	if timedOut.Status != "failed" || !strings.Contains(timedOut.Log, "Stage timed out after 1s") {
		t.Fatalf("timeout build = %q\n%s", timedOut.Status, timedOut.Log)
	}
}

func runYAMLBuildForTest(t *testing.T, config string, failFast bool) (*store.Build, *store.Store) {
	t.Helper()
	temporary := t.TempDir()
	database, err := store.New(filepath.Join(temporary, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	project, err := database.CreateProject("yaml-semantics", "", "", "git", "main", config, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := database.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := NewBuildRunner(database, nil, filepath.Join(temporary, "workspace"), nil)
	runner.SetArtifactManager(NewArtifactManager(database, filepath.Join(temporary, "artifacts")))
	if err := runner.ConfigureExecutionPolicy(ExecutionPolicy{
		DefaultTimeoutSec:        30,
		MaxConcurrentBuilds:      1,
		MaxConcurrentLocalBuilds: 1,
		RetryLimit:               0,
		FailFast:                 failFast,
		CPUPercent:               100,
	}); err != nil {
		t.Fatal(err)
	}
	runner.run(context.Background(), build.ID)
	finished, err := database.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	return finished, database
}

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
	if len(config.Stages) != 1 || config.Stages[0].JenkinsWhen != "(!params.ENTRY_ONLY)" {
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
		`not { branch 'main'; branch 'release' }`, `allOf { }`,
		`branch pattern: 'main', unexpected: 'value', comparator: 'REGEXP'`, `expression { dangerous() }`,
		`environment name: 'ENV'`,
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

func TestJenkinsWhenGroovyTruthAndStringBoolean(t *testing.T) {
	for _, tc := range []struct {
		expr   string
		params map[string]interface{}
		want   bool
	}{
		{"!params.MISSING", nil, true},
		{"params.VALUE", map[string]interface{}{"VALUE": "false"}, true},
		{"!params.VALUE", map[string]interface{}{"VALUE": "false"}, false},
		{"params.VALUE.toString().toBoolean()", map[string]interface{}{"VALUE": "false"}, false},
		{"params.VALUE.toString().toBoolean()", map[string]interface{}{"VALUE": " Y "}, true},
		{"params.VALUE.toString().toBoolean()", map[string]interface{}{"VALUE": true}, true},
	} {
		got, err := engine.EvaluateJenkinsWhen(tc.expr, tc.params)
		if err != nil || got != tc.want {
			t.Fatalf("%s: got %v: %v", tc.expr, got, err)
		}
	}
}

func TestJenkinsWhenNestedConditions(t *testing.T) {
	source := `pipeline { agent any stages { stage('deploy') {
when { anyOf { branch 'release/**'; branch 'main' }; environment name: 'DEPLOY_TO', value: 'production'; not { expression { params.SKIP.toString().toBoolean() } } }
steps { sh 'echo deploy' } } } }`
	config, err := engine.ParseJenkinsfileConfig(source, "nested")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		branch, env, skip string
		want              bool
	}{
		{"main", "production", "false", true}, {"RELEASE/v1/x", "production", "0", true},
		{"main", "staging", "false", false}, {"feature/x", "production", "false", false},
		{"main", "production", "true", false},
	} {
		got, err := engine.ShouldRunStageWithJenkinsContext(config.Stages[0], nil, map[string]interface{}{"SKIP": tc.skip}, map[string]string{"DEPLOY_TO": tc.env}, tc.branch)
		if err != nil || got != tc.want {
			t.Fatalf("%+v: got %v: %v", tc, got, err)
		}
	}
}

func TestJenkinsWhenEnvironmentMissingAndEmpty(t *testing.T) {
	for _, tc := range []struct {
		value string
		env   map[string]string
		want  bool
	}{
		{"", nil, true},
		{"", map[string]string{"RELEASE_MODE": ""}, true},
		{"production", nil, false},
		{"", map[string]string{"RELEASE_MODE": "production"}, false},
		{"production", map[string]string{"RELEASE_MODE": "production"}, true},
		{"production", map[string]string{"RELEASE_MODE": "Production"}, false},
	} {
		source := fmt.Sprintf(`pipeline { agent any stages { stage('deploy') { when { environment name: 'RELEASE_MODE', value: '%s' } steps { sh 'echo deploy' } } } }`, tc.value)
		config, err := engine.ParseJenkinsfileConfig(source, "environment")
		if err != nil {
			t.Fatal(err)
		}
		got, err := engine.ShouldRunStageWithJenkinsContext(config.Stages[0], nil, nil, tc.env, "main")
		if err != nil || got != tc.want {
			t.Fatalf("value=%q env=%v: got %v want %v: %v", tc.value, tc.env, got, tc.want, err)
		}
	}
}

func TestJenkinsWhenBranchComparators(t *testing.T) {
	for _, tc := range []struct {
		condition, branch string
		want              bool
	}{
		{`branch pattern: 'release/[0-9]+', comparator: 'REGEXP'`, "release/12", true},
		{`branch pattern: 'release/[0-9]+', comparator: 'REGEXP'`, "release/a12", false},
		{`branch pattern: 'Main', comparator: 'EQUALS'`, "main", false},
		{`branch pattern: '项目/**', comparator: 'GLOB'`, "项目/a/b", true},
		{`branch 'release/*'`, "release/a/b", false},
		{`branch 'release/**'`, "release/a/b", true},
		{`branch('main')`, "MAIN", true},
		{`branch(pattern: 'Main', comparator: 'EQUALS')`, "main", false},
		{`branch ''`, "", true},
		{`branch(pattern: '', comparator: 'EQUALS')`, "", true},
		{`branch ''`, "main", false},
		{`branch '*'`, "", false},
	} {
		source := fmt.Sprintf(`pipeline { agent any stages { stage('test') { when { %s } steps { sh 'echo test' } } } }`, tc.condition)
		config, err := engine.ParseJenkinsfileConfig(source, "test")
		if err != nil {
			t.Fatalf("%s: %v", tc.condition, err)
		}
		got, err := engine.ShouldRunStageWithJenkinsContext(config.Stages[0], nil, nil, nil, tc.branch)
		if err != nil || got != tc.want {
			t.Fatalf("%+v: got %v: %v", tc, got, err)
		}
	}
}

func TestJenkinsWhenParenthesizedEnvironment(t *testing.T) {
	source := `pipeline { agent any stages { stage('test') { when { environment(name: 'DEPLOY_TO', value: 'prod') } steps { sh 'echo test' } } } }`
	config, err := engine.ParseJenkinsfileConfig(source, "test")
	if err != nil {
		t.Fatal(err)
	}
	got, err := engine.ShouldRunStageWithJenkinsContext(config.Stages[0], nil, nil, map[string]string{"DEPLOY_TO": "prod"}, "main")
	if err != nil || !got {
		t.Fatalf("got %v: %v", got, err)
	}
}

func TestJenkinsWhenTagConditions(t *testing.T) {
	for _, tc := range []struct {
		condition, tag string
		want           bool
	}{
		{`buildingTag()`, "v1.2", true},
		{`buildingTag()`, "", false},
		{`tag ''`, "v1.2", true},
		{`tag ''`, "", false},
		{`tag 'release-*'`, "RELEASE-1", true},
		{`tag(pattern: 'v[0-9]+', comparator: 'REGEXP')`, "v12", true},
		{`tag pattern: 'v1', comparator: 'EQUALS'`, "V1", false},
		{`allOf { buildingTag(); tag 'release-*' }`, "release-1", true},
		{`not { buildingTag() }`, "", true},
	} {
		source := fmt.Sprintf(`pipeline { agent any stages { stage('test') { when { %s } steps { sh 'echo test' } } } }`, tc.condition)
		config, err := engine.ParseJenkinsfileConfig(source, "test")
		if err != nil {
			t.Fatalf("%s: %v", tc.condition, err)
		}
		got, err := engine.ShouldRunStageWithJenkinsContext(config.Stages[0], nil, nil, map[string]string{"TAG_NAME": tc.tag}, "main")
		if err != nil || got != tc.want {
			t.Fatalf("%+v: got %v: %v", tc, got, err)
		}
	}
}

func TestJenkinsWhenRunsThroughRealLocalRunner(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is not available")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	const absentVariable = "BUILDWORLD_JENKINS_WHEN_EXPECTED_ABSENT_871093"
	if _, exists := os.LookupEnv(absentVariable); exists {
		t.Skip("test variable is unexpectedly set")
	}
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := `pipeline { agent any
parameters { booleanParam(name: 'ENTRY_ONLY', defaultValue: false) }
environment {
DEPLOY_TO = 'production'
TAG_NAME = 'release-1'
}
stages { stage('guarded') { environment { DEPLOY_TO = 'stage' } when { allOf { branch 'main'; environment name: 'DEPLOY_TO', value: 'production'; expression { !params.ENTRY_ONLY } } } steps { sh 'test "$DEPLOY_TO" = stage; echo guarded-command-executed' } }
stage('tagged') { when { allOf { buildingTag(); tag 'release-*' } } steps { sh 'echo tag-command-executed' } }
stage('missing-env') { when { environment name: 'BUILDWORLD_JENKINS_WHEN_EXPECTED_ABSENT_871093', value: '' } steps { sh 'echo missing-env-executed' } }
stage('always') { steps { sh 'echo always-command-executed' } } } }`
	project, err := db.CreateProject("jenkins-when", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(db, nil, filepath.Join(root, "workspaces"), nil)
	runner.SetArtifactManager(engine.NewArtifactManager(db, filepath.Join(root, "artifacts")))
	defer runner.Shutdown(context.Background())
	for i, tc := range []struct {
		entryOnly   bool
		branch      string
		wantGuarded bool
	}{{true, "main", false}, {false, "main", true}, {false, "feature", false}} {
		build, err := db.CreateBuild(project.ID, i+1, "manual", tc.branch, "", fmt.Sprintf(`{"ENTRY_ONLY":%t}`, tc.entryOnly), nil, nil)
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
			t.Fatalf("case=%+v status=%s log=%s", tc, build.Status, build.Log)
		}
		if got := strings.Contains(build.Log, "guarded-command-executed"); got != tc.wantGuarded {
			t.Fatalf("case=%+v log=%s", tc, build.Log)
		}
		if !strings.Contains(build.Log, "always-command-executed") {
			t.Fatalf("unconditional stage missing: %s", build.Log)
		}
		if !strings.Contains(build.Log, "tag-command-executed") {
			t.Fatalf("tag stage missing: %s", build.Log)
		}
		if !strings.Contains(build.Log, "missing-env-executed") {
			t.Fatalf("missing environment condition stage missing: %s", build.Log)
		}
	}
}

func TestJenkinsBranchUsesLocalBranchWhenBranchNameEmpty(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, tc := range []struct {
		name, localBranch      string
		wantRelease, wantEmpty bool
	}{
		{"local", "release/v1", true, false}, {"empty", "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			db, err := store.New(filepath.Join(root, "buildworld.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			source := fmt.Sprintf(`pipeline { agent any
environment {
BRANCH_NAME = ''
GIT_LOCAL_BRANCH = '%s'
}
stages {
stage('release') { when { branch 'release/**' } steps { sh 'echo local-branch-executed' } }
stage('main') { when { branch 'main' } steps { sh 'echo build-branch-incorrect' } }
stage('empty') { when { branch '' } steps { sh 'echo empty-branch-executed' } }
} }`, tc.localBranch)
			project, err := db.CreateProject("jenkins-branch", "", "", "git", "main", source, 0, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", "{}", nil, nil)
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
			if build.Status != "success" || strings.Contains(build.Log, "build-branch-incorrect") || strings.Contains(build.Log, "local-branch-executed") != tc.wantRelease || strings.Contains(build.Log, "empty-branch-executed") != tc.wantEmpty {
				t.Fatalf("branch=%q status=%s log=%s", tc.localBranch, build.Status, build.Log)
			}
		})
	}
}

func TestJenkinsStageEnvironmentIsNotGlobal(t *testing.T) {
	source := `pipeline { agent any stages {
stage('one') { environment { DEPLOY_TO = 'stage' } steps { sh 'echo one' } }
stage('two') { steps { sh 'echo two' } } } }`
	config, err := engine.ParseJenkinsfileConfig(source, "scope")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := config.Environment["DEPLOY_TO"]; exists {
		t.Fatal("stage variable leaked globally")
	}
	if config.Stages[0].Environment["DEPLOY_TO"] != "stage" {
		t.Fatalf("missing stage environment: %#v", config.Stages[0])
	}
	if _, exists := config.Stages[1].Environment["DEPLOY_TO"]; exists {
		t.Fatal("stage variable leaked into the next stage")
	}
}

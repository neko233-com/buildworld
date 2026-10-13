package engine

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

const configuredTargetsFixture = `{"schemaVersion":1,"targets":[{"channel":"alpha","environment":"release","channelLabel":"Alpha"},{"channel":"alpha","environment":"test","channelLabel":"Alpha"},{"channel":"beta","environment":"audit"},{"channel":"third-store","environment":"preview-17"}]}`

func scopedTargetConfigFixture() *BuildConfig {
	return &BuildConfig{Parameters: []BuildParameter{
		{Name: "RELEASE_CHANNEL", Type: "choice", Choices: []string{"obsolete"}, Default: "alpha"},
		{Name: "RELEASE_ENVIRONMENT", Type: "string", Default: "release"},
		{Name: "RELEASE_ACTION", Type: "choice", Choices: []string{"restart-version", "publish-latest"}},
		{Name: "RELEASE_VERSION", Type: "string"},
		{Name: "CONFIRM_PUBLISH_LATEST", Type: "boolean", Default: false},
	}}
}

func TestReleaseTargetEnvironmentCannotOverrideValidatedOperatorParameters(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "release-environment.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("release", "", "", "git", "main", "jobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	project.PipelineFormat = "jenkinsfile"
	for _, name := range []string{"RELEASE_CHANNEL", "RELEASE_ENVIRONMENT", "RELEASE_ACTION", "RELEASE_VERSION", "CONFIRM_PUBLISH_LATEST", "ordinary"} {
		if err := data.SetEnvVar("global", nil, name, "global-override", false, "fixture"); err != nil {
			t.Fatal(err)
		}
		if err := data.SetEnvVar("project", &project.ID, name, "project-override", false, "fixture"); err != nil {
			t.Fatal(err)
		}
	}
	build := &store.Build{ProjectID: project.ID, Number: 1, Parameters: `{"RELEASE_CHANNEL":"alpha","RELEASE_ENVIRONMENT":"test","RELEASE_ACTION":"restart-version","RELEASE_VERSION":"0.1.21","CONFIRM_PUBLISH_LATEST":false,"ordinary":"operator-value"}`}
	runner := &BuildRunner{store: data}
	config := scopedTargetConfigFixture()
	config.Environment = map[string]string{"RELEASE_CHANNEL": "pipeline-override"}
	environment := runner.buildEnvAt(build, config, project, "")
	actual := map[string]string{}
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			actual[key] = value
		}
	}
	for name, want := range map[string]string{"RELEASE_CHANNEL": "alpha", "RELEASE_ENVIRONMENT": "test", "RELEASE_ACTION": "restart-version", "RELEASE_VERSION": "0.1.21", "CONFIRM_PUBLISH_LATEST": "false", "ordinary": "project-override"} {
		if actual[name] != want {
			t.Fatal("release target authority or ordinary Jenkins precedence changed", name)
		}
	}
}

func TestConfiguredReleaseTargetsSupplyArbitraryChannelsAndLinkedEnvironments(t *testing.T) {
	config := scopedTargetConfigFixture()
	if err := ApplyProjectReleaseTargets(config, []*store.EnvVar{{Scope: "project", Name: ReleaseTargetsVariable, Value: configuredTargetsFixture}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config.Parameters[0].Choices, []string{"alpha", "beta", "third-store"}) || config.Parameters[1].Default != "test" {
		t.Fatal("configured candidates were not authoritative or test was not preferred")
	}
	for _, pair := range []struct{ channel, environment string }{{"alpha", "test"}, {"alpha", "release"}, {"beta", "audit"}, {"third-store", "preview-17"}} {
		resolved, err := ResolveBuildParameters(config.Parameters, map[string]interface{}{"RELEASE_CHANNEL": pair.channel, "RELEASE_ENVIRONMENT": pair.environment, "RELEASE_VERSION": "0.1.21"})
		if err != nil || ValidateConfiguredReleaseTarget(config, resolved) != nil {
			t.Fatal("configured arbitrary target rejected", err)
		}
	}
	for _, pair := range []struct{ channel, environment string }{{"alpha", "audit"}, {"beta", "test"}, {"third-store", "release"}, {"missing", "test"}} {
		if ValidateConfiguredReleaseTarget(config, map[string]interface{}{"RELEASE_CHANNEL": pair.channel, "RELEASE_ENVIRONMENT": pair.environment}) == nil {
			t.Fatal("a cartesian or unknown target was accepted")
		}
	}
}

func TestConfiguredReleaseCatalogRejectsInvalidOrSecretDeclarationsWithoutDisclosure(t *testing.T) {
	for _, raw := range []string{"", "null", `{`, `{"schemaVersion":2,"targets":[]}`, `{"schemaVersion":1,"targets":[]}`, `{"schemaVersion":1,"targets":[{"channel":"../private-sentinel","environment":"test"}]}`, `{"schemaVersion":1,"targets":[{"channel":"alpha","environment":"Test"}]}`, `{"schemaVersion":1,"targets":[{"channel":"alpha","environment":"test","password":"private-sentinel"}]}`, `{"schemaVersion":1,"targets":[{"channel":"alpha","environment":"test"},{"channel":"alpha","environment":"test"}]}`, configuredTargetsFixture + `{}`} {
		if _, err := ParseReleaseTargetCatalog(raw); err == nil || strings.Contains(err.Error(), "private-sentinel") {
			t.Fatal("invalid public catalog accepted or raw value exposed")
		}
	}
	for _, fault := range []string{"secret-matrix", "global-matrix", "secret-channel", "string-channel", "secret-environment", "wrong-environment-type", "missing-environment", "duplicate-channel", "shared"} {
		t.Run(fault, func(t *testing.T) {
			config := scopedTargetConfigFixture()
			variable := &store.EnvVar{Scope: "project", Name: ReleaseTargetsVariable, Value: configuredTargetsFixture}
			switch fault {
			case "secret-matrix":
				variable.IsSecret = true
			case "global-matrix":
				variable.Scope = "global"
			case "secret-channel":
				config.Parameters[0].IsSecret = true
			case "string-channel":
				config.Parameters[0].Type = "string"
			case "secret-environment":
				config.Parameters[1].IsSecret = true
			case "wrong-environment-type":
				config.Parameters[1].Type = "password"
			case "missing-environment":
				config.Parameters = config.Parameters[:1]
			case "duplicate-channel":
				config.Parameters = append(config.Parameters, config.Parameters[0])
			case "shared":
				config.Parameters = append(config.Parameters, BuildParameter{Name: "RELEASE_SHARED_SERVICE", Type: "string"})
			}
			before := append([]BuildParameter(nil), config.Parameters...)
			if err := ApplyProjectReleaseTargets(config, []*store.EnvVar{variable}); err == nil || strings.Contains(err.Error(), "private-sentinel") || !reflect.DeepEqual(before, config.Parameters) || config.ReleaseTargets != nil {
				t.Fatal("catalog bypassed declaration guards, exposed raw input, or changed rejected config")
			}
		})
	}
}

func TestConfiguredReleaseMissingOrRemovedTargetsFailClosedAndSharedRemainsGeneric(t *testing.T) {
	config := scopedTargetConfigFixture()
	selected := map[string]interface{}{"RELEASE_CHANNEL": "alpha", "RELEASE_ENVIRONMENT": "test"}
	if err := ValidateConfiguredReleaseTarget(config, selected); err == nil {
		t.Fatal("missing catalog permitted scoped deployment")
	}
	if err := ApplyProjectReleaseTargets(config, []*store.EnvVar{{Scope: "project", Name: ReleaseTargetsVariable, Value: configuredTargetsFixture}}); err != nil {
		t.Fatal(err)
	}
	config.ReleaseTargets.Targets = config.ReleaseTargets.Targets[2:]
	if err := ValidateConfiguredReleaseTarget(config, selected); err == nil {
		t.Fatal("removed target remained authorized for retry or queued execution")
	}
	shared := &BuildConfig{Parameters: []BuildParameter{{Name: "RELEASE_SHARED_SERVICE", Type: "string", Default: "a-public-service"}}}
	if err := ValidateConfiguredReleaseTarget(shared, map[string]interface{}{"RELEASE_SHARED_SERVICE": "a-public-service"}); err != nil {
		t.Fatal("shared service unexpectedly required a channel matrix")
	}
}

func TestConfiguredReleaseParameterNamesUseTheResolverNormalization(t *testing.T) {
	config := scopedTargetConfigFixture()
	config.Parameters[0].Name = " RELEASE_CHANNEL "
	config.Parameters[1].Name = " RELEASE_ENVIRONMENT "
	if !ScopedReleaseParameters(config.Parameters) || ValidateConfiguredReleaseTarget(config, map[string]interface{}{"RELEASE_CHANNEL": "alpha", "RELEASE_ENVIRONMENT": "test"}) == nil {
		t.Fatal("parameter whitespace bypassed the mandatory target catalog")
	}
	if err := ApplyProjectReleaseTargets(config, []*store.EnvVar{{Scope: "project", Name: ReleaseTargetsVariable, Value: configuredTargetsFixture}}); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveBuildParameters(config.Parameters, map[string]interface{}{"RELEASE_VERSION": "0.1.21"})
	if err != nil || ValidateConfiguredReleaseTarget(config, resolved) != nil {
		t.Fatal("catalog and parameter resolver disagree on normalized names", err)
	}
	config.Parameters = append(config.Parameters, BuildParameter{Name: " RELEASE_SHARED_SERVICE ", Type: "string"})
	if ApplyProjectReleaseTargets(config, []*store.EnvVar{{Scope: "project", Name: ReleaseTargetsVariable, Value: configuredTargetsFixture}}) == nil {
		t.Fatal("parameter whitespace bypassed the shared-mode catalog rejection")
	}
}

func TestReleaseTargetStageAndStepCannotReplacePersistedSelection(t *testing.T) {
	parameters := map[string]interface{}{"RELEASE_CHANNEL": "third-store", "RELEASE_ENVIRONMENT": "audit-2", "RELEASE_ACTION": "restart-version", "RELEASE_VERSION": "literal-${OTHER}", "CONFIRM_PUBLISH_LATEST": false}
	stage := AppendPipelineEnvironment([]string{"OTHER=expanded", "ordinary=base"}, map[string]string{"RELEASE_CHANNEL": "different", "RELEASE_ENVIRONMENT": "release", "ordinary": "stage"})
	step := AppendStepEnvironment(stage, map[string]string{"env.RELEASE_ACTION": "publish-latest", "env.RELEASE_VERSION": "latest", "env.CONFIRM_PUBLISH_LATEST": "true", "env.ordinary": "step"})
	actual := map[string]string{}
	counts := map[string]int{}
	for _, entry := range releaseParameterEnvironment(step, parameters) {
		name, value, _ := strings.Cut(entry, "=")
		actual[name] = value
		counts[name]++
	}
	for name, want := range map[string]string{"RELEASE_CHANNEL": "third-store", "RELEASE_ENVIRONMENT": "audit-2", "RELEASE_ACTION": "restart-version", "RELEASE_VERSION": "literal-${OTHER}", "CONFIRM_PUBLISH_LATEST": "false", "ordinary": "step"} {
		if actual[name] != want || counts[name] != 1 {
			t.Fatal("stage/step changed the target, interpolated its version, or altered ordinary precedence", name)
		}
	}
	shared := releaseParameterEnvironment([]string{"RELEASE_SHARED_SERVICE=other-service", "ordinary=kept"}, map[string]interface{}{"RELEASE_SHARED_SERVICE": "configured-service"})
	if !reflect.DeepEqual(shared, []string{"ordinary=kept", "RELEASE_SHARED_SERVICE=configured-service"}) {
		t.Fatal("shared-service selection was not protected at the same shell boundary")
	}
}

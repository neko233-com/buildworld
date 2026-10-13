package engine

import "testing"

func TestReleaseEnvironmentAcceptsPublicStringWithPipelineDefaultAndSafeNames(t *testing.T) {
	parameters := []BuildParameter{{Name: "RELEASE_ENVIRONMENT", Type: "string", Default: "test"}}
	resolved, err := ResolveBuildParameters(parameters, nil)
	if err != nil || resolved["RELEASE_ENVIRONMENT"] != "test" {
		t.Fatalf("pipeline default = %v, %v", resolved, err)
	}
	for _, environment := range []string{"audit", "check", "qa-17"} {
		if _, err := ResolveBuildParameters(parameters, map[string]interface{}{"RELEASE_ENVIRONMENT": environment}); err != nil {
			t.Fatalf("configured name %q rejected: %v", environment, err)
		}
	}
	for _, environment := range []string{"", "../release", "Release", " test", "test\n", "1test", "qa_test", "abcdefghijklmnopqrstuvwxyz1234567"} {
		if _, err := ResolveBuildParameters(parameters, map[string]interface{}{"RELEASE_ENVIRONMENT": environment}); err == nil {
			t.Fatalf("unsafe environment %q accepted", environment)
		}
	}
	parameters[0].IsSecret = true
	if _, err := ResolveBuildParameters(parameters, nil); err == nil {
		t.Fatal("secret environment accepted")
	}
	parameters[0].IsSecret, parameters[0].Type = false, "text"
	if _, err := ResolveBuildParameters(parameters, nil); err == nil {
		t.Fatal("multiline environment accepted")
	}
}

func TestReleaseTargetParametersUsePublicChoicesAndTestDefault(t *testing.T) {
	parameters := []BuildParameter{
		{Name: "RELEASE_CHANNEL", Type: "choice", Choices: []string{"wechat", "taptap"}, Required: true},
		{Name: "RELEASE_ENVIRONMENT", Type: "choice", Choices: []string{"test", "release"}, Required: true},
	}
	resolved, err := ResolveBuildParameters(parameters, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resolved["RELEASE_CHANNEL"] != "wechat" || resolved["RELEASE_ENVIRONMENT"] != "test" {
		t.Fatalf("defaults = %+v", resolved)
	}
	parameters[0].IsSecret = true
	if _, err := ResolveBuildParameters(parameters, nil); err == nil {
		t.Fatal("secret release target accepted")
	}
	parameters[0].IsSecret = false
	parameters[0].Type = "string"
	if _, err := ResolveBuildParameters(parameters, nil); err == nil {
		t.Fatal("free-form release target accepted")
	}
}

func TestReleaseActionsRequireVersionOrFreshPublishConfirmation(t *testing.T) {
	parameters := []BuildParameter{
		{Name: "RELEASE_CHANNEL", Type: "choice", Choices: []string{"wechat", "taptap"}},
		{Name: "RELEASE_ENVIRONMENT", Type: "choice", Choices: []string{"test", "release"}},
		{Name: "RELEASE_ACTION", Type: "choice", Choices: []string{"restart-version", "publish-latest"}},
		{Name: "RELEASE_VERSION", Type: "string"},
		{Name: "CONFIRM_PUBLISH_LATEST", Type: "boolean"},
	}
	if _, err := ResolveBuildParameters(parameters, nil); err == nil {
		t.Fatal("default restart without version accepted")
	}
	if _, err := ResolveBuildParameters(parameters, map[string]interface{}{"RELEASE_ACTION": "publish-latest"}); err == nil {
		t.Fatal("latest without confirmation accepted")
	}
	if _, err := ResolveBuildParameters(parameters, map[string]interface{}{"RELEASE_ACTION": "publish-latest", "CONFIRM_PUBLISH_LATEST": "true"}); err == nil {
		t.Fatal("text confirmation accepted")
	}
	if _, err := ResolveBuildParameters(parameters, map[string]interface{}{"RELEASE_ACTION": "publish-latest", "CONFIRM_PUBLISH_LATEST": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveBuildParameters(parameters, map[string]interface{}{"RELEASE_VERSION": "0.1.123"}); err != nil {
		t.Fatal(err)
	}
}

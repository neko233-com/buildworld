package engine

import (
	"strings"
	"testing"
)

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

func sharedReleaseParameters() []BuildParameter {
	return []BuildParameter{
		{Name: "RELEASE_SHARED_SERVICE", Type: "string", Default: "sf-minigame-shared"},
		{Name: "RELEASE_ACTION", Type: "choice", Choices: []string{"restart-version", "publish-latest"}},
		{Name: "RELEASE_VERSION", Type: "string"},
		{Name: "CONFIRM_PUBLISH_LATEST", Type: "boolean", Default: false},
	}
}

func TestSharedReleaseTargetsUseDeclaredPublicServiceInsteadOfScope(t *testing.T) {
	for _, action := range []string{"publish-latest", "restart-version"} {
		parameters := sharedReleaseParameters()
		supplied := map[string]interface{}{"RELEASE_ACTION": action, "RELEASE_VERSION": "0.0.66", "CONFIRM_PUBLISH_LATEST": action == "publish-latest"}
		resolved, err := ResolveBuildParameters(parameters, supplied)
		if err != nil || resolved["RELEASE_SHARED_SERVICE"] != "sf-minigame-shared" {
			t.Fatalf("shared %s = %v, %v", action, resolved, err)
		}
		for _, name := range []string{"RELEASE_CHANNEL", "RELEASE_ENVIRONMENT"} {
			if _, ok := resolved[name]; ok {
				t.Fatalf("shared request gained inferred %s", name)
			}
		}
	}
	parameters := sharedReleaseParameters()
	parameters[0].Default = nil
	if _, err := ResolveBuildParameters(parameters, map[string]interface{}{"RELEASE_SHARED_SERVICE": strings.Repeat("a", 32), "RELEASE_VERSION": "0.0.66"}); err != nil {
		t.Fatal("valid maximum service ID rejected", err)
	}
}

func TestSharedReleaseParametersFailClosed(t *testing.T) {
	for _, name := range []string{"missing", "blank", "null", "wrong-value-type", "unsafe-path", "uppercase", "whitespace", "too-long", "wrong-declaration-type", "choice-declaration", "secret", "mixed-channel", "mixed-environment", "undeclared-channel", "undeclared-environment", "undeclared-service", "latest-unconfirmed", "latest-text-confirm", "restart-empty-version", "secret-confirm", "secret-version"} {
		t.Run(name, func(t *testing.T) {
			parameters := sharedReleaseParameters()
			supplied := map[string]interface{}{"RELEASE_ACTION": "restart-version", "RELEASE_VERSION": "0.0.66"}
			sensitive := "private-invalid-service-value"
			switch name {
			case "missing":
				parameters[0].Default = nil
			case "blank":
				supplied["RELEASE_SHARED_SERVICE"] = ""
			case "null":
				supplied["RELEASE_SHARED_SERVICE"] = nil
			case "wrong-value-type":
				supplied["RELEASE_SHARED_SERVICE"] = 42
			case "unsafe-path":
				supplied["RELEASE_SHARED_SERVICE"] = "../" + sensitive
			case "uppercase":
				supplied["RELEASE_SHARED_SERVICE"] = "Shared"
			case "whitespace":
				supplied["RELEASE_SHARED_SERVICE"] = " shared"
			case "too-long":
				supplied["RELEASE_SHARED_SERVICE"] = strings.Repeat("a", 33)
			case "wrong-declaration-type":
				parameters[0].Type = "password"
			case "choice-declaration":
				parameters[0].Type = "choice"
				parameters[0].Choices = []string{"sf-minigame-shared"}
			case "secret":
				parameters[0].IsSecret = true
				parameters[0].Default = sensitive
			case "mixed-channel":
				parameters = append(parameters, BuildParameter{Name: "RELEASE_CHANNEL", Type: "choice", Choices: []string{"wechat"}})
			case "mixed-environment":
				parameters = append(parameters, BuildParameter{Name: "RELEASE_ENVIRONMENT", Type: "string"})
			case "undeclared-channel":
				supplied["RELEASE_CHANNEL"] = "wechat"
			case "undeclared-environment":
				supplied["RELEASE_ENVIRONMENT"] = "test"
			case "undeclared-service":
				parameters = parameters[1:]
				supplied["RELEASE_SHARED_SERVICE"] = "sf-minigame-shared"
			case "latest-unconfirmed":
				supplied["RELEASE_ACTION"] = "publish-latest"
			case "latest-text-confirm":
				supplied["RELEASE_ACTION"] = "publish-latest"
				supplied["CONFIRM_PUBLISH_LATEST"] = "true"
			case "restart-empty-version":
				supplied["RELEASE_VERSION"] = ""
			case "secret-confirm":
				parameters[3].IsSecret = true
			case "secret-version":
				parameters[2].IsSecret = true
			}
			_, err := ResolveBuildParameters(parameters, supplied)
			if err == nil {
				t.Fatal("unsafe/missing/mixed release contract accepted")
			}
			if strings.Contains(err.Error(), sensitive) {
				t.Fatal("error revealed service value")
			}
		})
	}
}

func TestScopedReleaseCannotUseSharedParameterToBypassTargetRequirement(t *testing.T) {
	parameters := []BuildParameter{{Name: "RELEASE_ACTION", Type: "choice", Choices: []string{"restart-version", "publish-latest"}}, {Name: "RELEASE_VERSION", Type: "string"}, {Name: "CONFIRM_PUBLISH_LATEST", Type: "boolean"}}
	for _, supplied := range []map[string]interface{}{
		{"RELEASE_VERSION": "0.0.66"},
		{"RELEASE_ACTION": "publish-latest", "CONFIRM_PUBLISH_LATEST": true},
		{"RELEASE_SHARED_SERVICE": "sf-minigame-shared", "RELEASE_VERSION": "0.0.66"},
	} {
		if _, err := ResolveBuildParameters(parameters, supplied); err == nil {
			t.Fatal("release accepted without declared shared service or complete scope")
		}
	}
}

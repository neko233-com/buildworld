package engine

import (
	"strings"
	"testing"
)

func TestParseMarkdownConfigCreatesPlatformAdditions(t *testing.T) {
	source := "# Release\n\n## Variables\n- REGISTRY: registry.internal/app\n\n## Pipeline\n### Package\n```default shell\nnpm run build\n```\n```macos shell\n./sign.sh\n```\n"
	cfg, err := ParseMarkdownConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment["REGISTRY"] != "registry.internal/app" {
		t.Fatalf("environment = %#v", cfg.Environment)
	}
	if len(cfg.Stages) != 1 || cfg.Stages[0].Steps[0].Command != "npm run build" {
		t.Fatalf("stages = %#v", cfg.Stages)
	}
	if got := cfg.Stages[0].Steps[0].PlatformAdditions["macos"]; got != "./sign.sh" {
		t.Fatalf("macos addition = %q", got)
	}
}

func TestBuildKVOutputCanBeReadByFollowingNode(t *testing.T) {
	env := appendBuildKV(nil, "::buildworld:set IMAGE=registry.local/app:42\n")
	got := resolveVars("deploy ${build.IMAGE}", env, nil)
	if got != "deploy registry.local/app:42" {
		t.Fatalf("KV substitution = %q", got)
	}
}

func TestParseMarkdownConfigCreatesScheduleTrigger(t *testing.T) {
	source := "# Release\n\n## Schedule\n- cron: \"15 4 * * 1\"\n\n## Pipeline\n### Build\n```default shell\necho done\n```\n"
	cfg, err := ParseMarkdownConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Triggers) != 2 || cfg.Triggers[1].Type != "schedule" || cfg.Triggers[1].Config["cron"] != "15 4 * * 1" {
		t.Fatalf("triggers = %#v", cfg.Triggers)
	}
}

func TestParseMarkdownConfigCreatesArtifactPatterns(t *testing.T) {
	source := "# Release\n\n## Artifacts\n- dist/*.zip\n- reports/junit.xml\n\n## Pipeline\n### Build\n```default shell\necho done\n```\n"
	cfg, err := ParseMarkdownConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(cfg.Artifacts); got != 2 || cfg.Artifacts[0] != "dist/*.zip" || cfg.Artifacts[1] != "reports/junit.xml" {
		t.Fatalf("artifacts = %#v", cfg.Artifacts)
	}
}

func TestParseMarkdownConfigCreatesAgentRequirements(t *testing.T) {
	source := "# Release\n\n## Agents\n- labels: macos, signing\n- pool: release\n\n## Pipeline\n### Build\n```default shell\necho done\n```\n"
	cfg, err := ParseMarkdownConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"macos", "signing", "pool=release"}
	if len(cfg.AgentRequirements) != len(want) {
		t.Fatalf("agent requirements = %#v", cfg.AgentRequirements)
	}
	for index, requirement := range want {
		if cfg.AgentRequirements[index] != requirement {
			t.Fatalf("agent requirements = %#v", cfg.AgentRequirements)
		}
	}
}

func TestParseMarkdownConfigOrdersNeedsGraph(t *testing.T) {
	cfg, err := ParseMarkdownConfig("# Graph\n\n## Pipeline\n\n### Publish\n\n- needs: Package\n\n```default shell\necho publish\n```\n\n### Test\n\n```default shell\necho test\n```\n\n### Package\n\n- needs: Test\n\n```default shell\necho package\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	got := []string{cfg.Stages[0].Name, cfg.Stages[1].Name, cfg.Stages[2].Name}
	if strings.Join(got, ",") != "Test,Package,Publish" {
		t.Fatalf("ordered stages = %v", got)
	}
}

func TestParseMarkdownConfigRejectsCyclicNeeds(t *testing.T) {
	_, err := ParseMarkdownConfig("# Graph\n\n## Pipeline\n\n### First\n- needs: Second\n```default shell\necho first\n```\n\n### Second\n- needs: First\n```default shell\necho second\n```\n")
	if err == nil || !strings.Contains(err.Error(), "cycle detected") {
		t.Fatalf("expected cyclic graph error, got %v", err)
	}
}

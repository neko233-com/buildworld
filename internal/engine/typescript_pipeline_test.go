package engine

import (
	"strings"
	"testing"
)

func TestParseTypeScriptPipeline(t *testing.T) {
	config, err := ParsePipelineConfig(`import { definePipeline, shell, stage, trigger, parameter, watchService } from "@buildworld/pipeline"

export default definePipeline({
  name: "release",
  environment: { NODE_ENV: "production" },
  parameters: [parameter("target", "choice", { choices: ["staging", "production"], default: "staging" })],
  on: [trigger("cron", { expression: "0 2 * * *" })],
  agentRequirements: ["macos"],
	  allowLongRunning: true,
  stages: [
    stage("Verify", shell("tests", "npm test")),
    stage("Deploy", [shell("release", "./deploy.sh")], { dependsOn: ["Verify"] }),
	    stage("Observe", watchService("server log", { pidFile: "server.pid", logFile: "server.log", heartbeatSeconds: 30 })),
  ],
} as const)
`)
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != "release" || config.Environment["NODE_ENV"] != "production" {
		t.Fatalf("config identity = %#v", config)
	}
	if len(config.Stages) != 3 || config.Stages[1].DependsOn[0] != "Verify" || config.Stages[0].Steps[0].Command != "npm test" {
		t.Fatalf("stages = %#v", config.Stages)
	}
	if len(config.AgentRequirements) != 1 || config.AgentRequirements[0] != "macos" || len(config.Triggers) != 1 || !config.AllowLongRunning || config.Stages[2].Steps[0].Type != "service_watch" {
		t.Fatalf("pipeline helpers did not normalize: %#v", config)
	}
}

func TestFormatTypeScriptPipelineRoundTripsBuildConfig(t *testing.T) {
	original := &BuildConfig{
		Name:             "migrated",
		AllowLongRunning: true,
		Environment:      map[string]string{"TARGET_DIR": "/srv/game"},
		Stages: []Stage{{
			Name: "Observe",
			Steps: []Step{{
				Name: "server log", Type: "service_watch", Config: map[string]string{"pid_file": "server.pid", "log_file": "server.log"},
			}},
		}},
	}
	source, err := FormatTypeScriptPipeline(original)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePipelineConfig(source)
	if err != nil {
		t.Fatalf("formatted TypeScript did not parse: %v\n%s", err, source)
	}
	if parsed.Name != original.Name || !parsed.AllowLongRunning || parsed.Stages[0].Steps[0].Type != "service_watch" || parsed.Environment["TARGET_DIR"] != "/srv/game" {
		t.Fatalf("round trip = %#v", parsed)
	}
}

func TestParseTypeScriptPipelineRejectsRuntimeAccess(t *testing.T) {
	_, err := ParsePipelineConfig(`import { definePipeline } from "@buildworld/pipeline"
export default definePipeline({ stages: [] }); process.exit(1)`)
	if err == nil || !strings.Contains(err.Error(), "declarative") {
		t.Fatalf("runtime access error = %v", err)
	}
}

func TestParseTypeScriptPipelineRejectsOtherImports(t *testing.T) {
	_, err := ParsePipelineConfig(`import { readFile } from "node:fs"
export default definePipeline({ stages: [] })`)
	if err == nil || !strings.Contains(err.Error(), "only import") {
		t.Fatalf("import error = %v", err)
	}
}

func TestParseTypeScriptPipelineAllowsShellSyntaxAndComments(t *testing.T) {
	config, err := ParsePipelineConfig(`import { definePipeline, shell, stage } from "@buildworld/pipeline"
// A shell command may use a while loop; it is not TypeScript control flow.
export default definePipeline({ stages: [stage("Run", shell("serve", "while true; do echo tick; sleep 1; done"))] })`)
	if err != nil || config.Stages[0].Steps[0].Command == "" {
		t.Fatalf("shell source should be accepted: config=%#v err=%v", config, err)
	}
}

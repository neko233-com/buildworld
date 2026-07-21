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

func TestParseTypeScriptPipelineRequiresValueImportedHelpers(t *testing.T) {
	tests := []struct {
		name   string
		source string
		helper string
	}{
		{
			name:   "missing definePipeline import",
			source: `export default definePipeline({ stages: [] })`,
			helper: "definePipeline",
		},
		{
			name: "type-only import declaration",
			source: `import type { definePipeline } from "@buildworld/pipeline"
export default definePipeline({ stages: [] })`,
			helper: "definePipeline",
		},
		{
			name: "type-only import specifier",
			source: `import { type definePipeline } from "@buildworld/pipeline"
export default definePipeline({ stages: [] })`,
			helper: "definePipeline",
		},
		{
			name: "missing nested helper import",
			source: `import { definePipeline, stage } from "@buildworld/pipeline"
export default definePipeline({ stages: [stage("Build", shell("run", "echo ok"))] })`,
			helper: "shell",
		},
		{
			name: "type-only nested helper import",
			source: `import { definePipeline, stage, type shell } from "@buildworld/pipeline"
export default definePipeline({ stages: [stage("Build", shell("run", "echo ok"))] })`,
			helper: "shell",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseTypeScriptPipeline(test.source)
			if err == nil || !strings.Contains(err.Error(), `helper "`+test.helper+`" must be imported as a value`) {
				t.Fatalf("error = %v, want value-import error for %s", err, test.helper)
			}
		})
	}
}

func TestParseTypeScriptPipelineAcceptsMixedValueAndTypeImports(t *testing.T) {
	config, err := ParseTypeScriptPipeline(`import { definePipeline, shell, stage, type Pipeline } from "@buildworld/pipeline"
export default definePipeline({
  stages: [stage("Build", shell("run", "echo ok"))],
}) satisfies Pipeline`)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Stages) != 1 || config.Stages[0].Steps[0].Command != "echo ok" {
		t.Fatalf("config = %#v", config)
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

func TestParseTypeScriptPipelinePreservesSecurityWordsInsideStrings(t *testing.T) {
	source := "import { definePipeline, shell, stage, type Pipeline, type PostCondition, type ApprovalRole } from '@buildworld/pipeline'\n" +
		"export default definePipeline({ stages: [stage('Run', shell('script', `echo \"export default\"; echo \"as const\"; echo \"satisfies Pipeline\"`))] }) satisfies Pipeline"
	config, err := ParsePipelineConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	want := "echo \"export default\"; echo \"as const\"; echo \"satisfies Pipeline\""
	if got := config.Stages[0].Steps[0].Command; got != want {
		t.Fatalf("command was modified:\n got: %q\nwant: %q", got, want)
	}
}

func TestParseTypeScriptPipelineNormalizesCamelCaseAndApproval(t *testing.T) {
	config, err := ParsePipelineConfig(`import { definePipeline, parameter, shell, stage, watchService } from "@buildworld/pipeline"
export default definePipeline({
  approval: { strategy: "single", requiredRoles: ["admin"], allowRequester: false },
  parameters: [parameter("token", "password", { isSecret: true })],
  stages: [
    stage("Checkout", shell("checkout", "echo checkout")),
    stage("Build", shell("compile", "go build", { platformAdditions: { macos: "./sign.sh" } }), {
      dependsOn: ["Checkout"],
      workingDirectory: "services/api",
      timeoutSec: 120,
    }),
    stage("Watch", watchService("server", { targetDir: "/srv/app", pidFile: "server.pid", logFile: "server.log", heartbeatSeconds: 30 })),
  ],
})`)
	if err != nil {
		t.Fatal(err)
	}
	if config.Approval == nil || config.Approval.Strategy != "single" || config.Approval.AllowRequester == nil || *config.Approval.AllowRequester {
		t.Fatalf("approval = %#v", config.Approval)
	}
	if len(config.Approval.RequiredRoles) != 1 || config.Approval.RequiredRoles[0] != "admin" {
		t.Fatalf("approval roles = %#v", config.Approval.RequiredRoles)
	}
	if !config.Parameters[0].IsSecret {
		t.Fatalf("parameter = %#v", config.Parameters[0])
	}
	build := config.Stages[1]
	if build.DependsOn[0] != "Checkout" || build.Steps[0].PlatformAdditions["macos"] != "./sign.sh" {
		t.Fatalf("stage normalization = %#v", build)
	}
	if build.WorkingDirectory != "services/api" || build.TimeoutSec != 120 {
		t.Fatalf("stage camelCase aliases = %#v", build)
	}
	watch := config.Stages[2].Steps[0].Config
	if watch["target_dir"] != "/srv/app" || watch["pid_file"] != "server.pid" || watch["log_file"] != "server.log" || watch["heartbeat_seconds"] != "30" {
		t.Fatalf("watch config = %#v", watch)
	}
}

func TestParseTypeScriptPipelineRejectsExecutableAST(t *testing.T) {
	cases := map[string]string{
		"this":                   `export default definePipeline({ stages: [], description: this })`,
		"eval":                   `export default definePipeline({ stages: [], description: eval("1") })`,
		"Function":               `export default definePipeline({ stages: [], description: Function("return 1")() })`,
		"constructor member":     `export default definePipeline({ stages: [], description: definePipeline.constructor("return 1") })`,
		"computed member":        `export default definePipeline({ stages: [], description: definePipeline["name"] })`,
		"computed property":      `export default definePipeline({ stages: [], ["name"]: "bad" })`,
		"constructor property":   `export default definePipeline({ stages: [], constructor: "bad" })`,
		"getter":                 `export default definePipeline({ get stages() { return [] } })`,
		"spread":                 `export default definePipeline({ ...{ stages: [] } })`,
		"arrow":                  `export default definePipeline({ stages: [], description: (() => "bad")() })`,
		"new":                    `export default definePipeline({ stages: [], description: new Date() })`,
		"conditional":            `export default definePipeline({ stages: true ? [] : [] })`,
		"assignment":             `export default definePipeline({ stages: (globalThis.x = []) })`,
		"extra statement":        `export default definePipeline({ stages: [] }); while (true) {}`,
		"if statement":           `export default definePipeline({ stages: [] }); if (true) { throw "bad" }`,
		"variable declaration":   `const pipeline = definePipeline({ stages: [] }); export default pipeline`,
		"import alias":           `import { definePipeline as unsafe } from "@buildworld/pipeline"; export default definePipeline({ stages: [] })`,
		"unsupported assertion":  `export default definePipeline({ stages: [] } as any)`,
		"unsupported satisfies":  `export default definePipeline({ stages: [] }) satisfies Unknown`,
		"member after satisfies": `export default definePipeline({ stages: [] }) satisfies Pipeline["stages"]`,
	}
	cases["template interpolation"] = "export default definePipeline({ stages: [], description: `${process.exit(1)}` })"

	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseTypeScriptPipeline(source); err == nil {
				t.Fatal("unsafe pipeline was accepted")
			}
		})
	}
}

func TestParseTypeScriptPipelineAcceptsOnlyStaticTemplates(t *testing.T) {
	config, err := ParseTypeScriptPipeline("import { definePipeline } from '@buildworld/pipeline'\nexport default definePipeline({ name: `static\\nname`, stages: [] } as const)")
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != "static\nname" {
		t.Fatalf("name = %q", config.Name)
	}
}

func TestParseTypeScriptPipelineEnforcesASTLimits(t *testing.T) {
	prefix := "import { definePipeline } from '@buildworld/pipeline'\n"
	t.Run("parser syntax depth", func(t *testing.T) {
		nested := strings.Repeat("(", typeScriptPipelineMaxSyntaxDepth+1) + "null" + strings.Repeat(")", typeScriptPipelineMaxSyntaxDepth+1)
		_, err := ParseTypeScriptPipeline(prefix + "export default definePipeline({ stages: [" + nested + "] })")
		if err == nil || !strings.Contains(err.Error(), "syntax depth") {
			t.Fatalf("syntax depth error = %v", err)
		}
	})

	t.Run("depth", func(t *testing.T) {
		nested := strings.Repeat("[", typeScriptPipelineMaxDepth+5) + "null" + strings.Repeat("]", typeScriptPipelineMaxDepth+5)
		_, err := ParseTypeScriptPipeline(prefix + "export default definePipeline({ stages: " + nested + " })")
		if err == nil || !strings.Contains(err.Error(), "depth") {
			t.Fatalf("depth error = %v", err)
		}
	})

	t.Run("nodes", func(t *testing.T) {
		items := strings.Repeat("null,", typeScriptPipelineMaxNodes+10)
		_, err := ParseTypeScriptPipeline(prefix + "export default definePipeline({ stages: [" + items + "] })")
		if err == nil || !strings.Contains(err.Error(), "node count") {
			t.Fatalf("node error = %v", err)
		}
	})

	t.Run("output", func(t *testing.T) {
		payload := strings.Repeat(`"`, 150_000)
		source := prefix + "export default definePipeline({ agentRequirements: [`" + payload + "`], stages: [] })"
		_, err := ParseTypeScriptPipeline(source)
		if err == nil || !strings.Contains(err.Error(), "output exceeds") {
			t.Fatalf("output error = %v", err)
		}
	})
}

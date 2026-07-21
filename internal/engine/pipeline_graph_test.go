package engine

import (
	"strings"
	"testing"
)

func TestParsePipelineConfigOrdersForwardDependenciesAcrossSupportedFormats(t *testing.T) {
	sources := map[string]string{
		"typescript": `import { definePipeline, shell, stage } from "@buildworld/pipeline"
export default definePipeline({
  stages: [
    stage("Deploy", shell("deploy", "echo deploy"), { dependsOn: ["Build"] }),
    stage("Build", shell("build", "echo build")),
  ],
})`,
		"yaml": `jobs:
  deploy:
    name: Deploy
    needs: [build]
    steps:
      - run: echo deploy
  build:
    name: Build
    steps:
      - run: echo build
`,
	}

	for format, source := range sources {
		t.Run(format, func(t *testing.T) {
			config, err := ParsePipelineConfig(source)
			if err != nil {
				t.Fatal(err)
			}
			if got := config.Stages[0].Name + "," + config.Stages[1].Name; got != "Build,Deploy" {
				t.Fatalf("stage order = %q, want Build,Deploy", got)
			}
		})
	}
}

func TestParsePipelineConfigRejectsInvalidStageGraphsBeforeExecution(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "yaml unknown dependency",
			source: `jobs:
  deploy:
    needs: [missing]
    steps: [{run: echo deploy}]
`,
			want: "unknown job",
		},
		{
			name: "typescript dependency cycle",
			source: `import { definePipeline, shell, stage } from "@buildworld/pipeline"
export default definePipeline({ stages: [
  stage("First", shell("first", "echo first"), { dependsOn: ["Second"] }),
  stage("Second", shell("second", "echo second"), { dependsOn: ["First"] }),
] })`,
			want: "dependency cycle",
		},
		{
			name: "yaml duplicate stage name",
			source: `jobs:
  first:
    name: Build
    steps: [{run: echo one}]
  second:
    name: Build
    steps: [{run: echo two}]
`,
			want: "declared more than once",
		},
		{
			name: "yaml duplicate dependency",
			source: `jobs:
  build:
    steps: [{run: echo build}]
  deploy:
    needs: [build, build]
    steps: [{run: echo deploy}]
`,
			want: "repeats dependency",
		},
		{
			name: "typescript empty stage name",
			source: `import { definePipeline, shell, stage } from "@buildworld/pipeline"
export default definePipeline({ stages: [stage(" ", shell("one", "echo one"))] })`,
			want: "empty name",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParsePipelineConfig(test.source)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

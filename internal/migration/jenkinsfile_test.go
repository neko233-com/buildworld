package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/engine"
)

const serverJenkinsfileFixture = `
pipeline {
    agent any
    environment {
        TARGET_DIR = "/srv/server-project"
        GIT_REPO_URL = "https://example.invalid/server-project.git"
        PATH = "${env.PATH}:/usr/local/go/bin"
    }

    // options {
    //     disableConcurrentBuilds()
    // }

    stages {
        stage('停止旧服务器进程') {
            steps {
                script {
                    echo "停止旧服务器"
                    sh """
                        cd ${TARGET_DIR} || exit 0
                        OLD_PID=\$(cat game-server.pid.txt)
                        if [ -n "\${OLD_PID}" ]; then
                            kill -TERM \${OLD_PID} || true
                        fi
                    """
                }
            }
        }
        stage('代码更新') {
            steps {
                script {
                    if (!fileExists(env.TARGET_DIR)) {
                        echo "目标目录不存在，克隆仓库"
                        sh """
                            mkdir -p \$(dirname ${TARGET_DIR})
                            git clone ${GIT_REPO_URL} ${TARGET_DIR}
                        """
                    } else {
                        echo "更新代码"
                        dir(env.TARGET_DIR) {
                            sh """
                                git fetch --all
                                git reset --hard origin/main
                                git clean -fd
                                git pull
                            """
                        }
                    }
                }
            }
        }
        stage('更新 Team-Resources') {
            steps {
                sh """
                    cd ${TARGET_DIR}
                    ./_scripts/deploy/update-team-resources.sh
                """
            }
        }
        stage('校验 BusinessConfig') {
            steps {
                sh """
                    cd ${TARGET_DIR}
                    python3 validate-business-config.py
                """
            }
        }
        stage('构建与启动') {
            steps {
                script {
                    sh """
                        cd ${TARGET_DIR}
                        go test ./...
                        go build -o server-game-sf ./server_game/cmd
                        nohup ./server-game-sf > server.log 2>&1 &
                        SERVER_PID=\$!
                        echo \${SERVER_PID} > game-server.pid.txt
                    """
                }
            }
        }
    }
    post {
        always {
            echo "构建流程结束"
        }
    }
}`

func TestJenkinsfileStrategyConvertsServerDeploymentPipeline(t *testing.T) {
	result, err := NewJenkinsfileStrategy().Convert(Request{
		Source: serverJenkinsfileFixture,
		Name:   "example-server-project-go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != ResultVersion || result.SourceFormat != "jenkinsfile" || result.TargetFormat != "buildworld-json" {
		t.Fatalf("result metadata = %#v", result)
	}
	if result.Summary.StageCount != 5 || result.Summary.EnvironmentCount != 3 {
		t.Fatalf("summary = %#v", result.Summary)
	}
	if result.Hints.RepositoryURL != "https://example.invalid/server-project.git" || result.Hints.DefaultBranch != "main" {
		t.Fatalf("hints = %#v", result.Hints)
	}
	if strings.Contains(result.Config, `\u003e`) || strings.Contains(result.Config, `\u0026`) {
		t.Fatalf("shell operators were HTML-escaped in pretty JSON")
	}
	if !hasWarning(result.Warnings, "post_review_required") || hasWarning(result.Warnings, "options_review_required") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}

	config, err := engine.ParsePipelineConfig(result.Config)
	if err != nil {
		t.Fatalf("migrated config is invalid: %v", err)
	}
	if config.Environment["PATH"] != "${PATH}:/usr/local/go/bin" {
		t.Fatalf("PATH = %q", config.Environment["PATH"])
	}
	if len(config.Stages) != 5 {
		t.Fatalf("stages = %d", len(config.Stages))
	}
	stopCommand := config.Stages[0].Steps[0].Command
	if !strings.Contains(stopCommand, "OLD_PID=$(cat game-server.pid.txt)") || strings.Contains(stopCommand, `\${OLD_PID}`) {
		t.Fatalf("Jenkins GString escaping was not normalized:\n%s", stopCommand)
	}
	updateCommand := config.Stages[1].Steps[0].Command
	for _, expected := range []string{
		`if [ ! -e "${TARGET_DIR}" ]; then`,
		`git clone ${GIT_REPO_URL} ${TARGET_DIR}`,
		`cd "${TARGET_DIR}"`,
		`git reset --hard origin/main`,
		"else",
		"fi",
	} {
		if !strings.Contains(updateCommand, expected) {
			t.Fatalf("update command does not contain %q:\n%s", expected, updateCommand)
		}
	}
	startCommand := config.Stages[4].Steps[0].Command
	if !strings.Contains(startCommand, "SERVER_PID=$!") || !strings.Contains(startCommand, "echo ${SERVER_PID}") {
		t.Fatalf("background process variables were not preserved:\n%s", startCommand)
	}
}

func TestJenkinsfileRegistryRejectsUnknownFormats(t *testing.T) {
	_, err := NewDefaultRegistry().Convert("unknown-ci", Request{Source: "pipeline {}"})
	if err == nil || !strings.Contains(err.Error(), "unsupported pipeline source format") {
		t.Fatalf("error = %v", err)
	}
}

func TestJenkinsfileStrategyConvertsDifferentPackagingStyles(t *testing.T) {
	tests := []struct {
		name              string
		file              string
		stages            int
		environment       int
		stageNames        []string
		commandFragments  []string
		agentRequirement  string
		requiredWarnings  []string
		repositoryURLHint string
	}{
		{
			name:              "hot refresh server configuration",
			file:              "hot-refresh-server.Jenkinsfile",
			stages:            3,
			environment:       6,
			stageNames:        []string{"飞书通知", "更新 Team-Resources", "校验 BusinessConfig"},
			commandFragments:  []string{"feishu-robot-game-server-config-refresh.py", "chmod +x ./_scripts/deploy/update-team-resources.sh", "validate-business-config.py --config-dir"},
			requiredWarnings:  []string{"post_review_required"},
			repositoryURLHint: "http://192.0.2.42:3000/example/example-server-project-go.git",
		},
		{
			name:             "TypeScript Node package",
			file:             "typescript-node-package.Jenkinsfile",
			stages:           3,
			environment:      2,
			stageNames:       []string{"Install", "Test", "Package"},
			commandFragments: []string{`cd "web"`, "npm ci", "npm test -- --run", "web-dist.tgz"},
			agentRequirement: "linux-node",
			requiredWarnings: []string{"post_review_required"},
		},
		{
			name:             "Java Maven package",
			file:             "java-maven-package.Jenkinsfile",
			stages:           3,
			environment:      2,
			stageNames:       []string{"Compile", "Unit tests", "Package JAR"},
			commandFragments: []string{"mvn -B -DskipTests compile", "mvn -B test", "mvn -B -DskipTests package"},
			requiredWarnings: []string{"options_review_required"},
		},
		{
			name:             "C sharp dotnet package",
			file:             "dotnet-package.Jenkinsfile",
			stages:           3,
			environment:      2,
			stageNames:       []string{"Restore", "Test", "Publish"},
			commandFragments: []string{"dotnet restore BuildWorld.sln", "dotnet test BuildWorld.sln", "dotnet publish src/BuildWorld/BuildWorld.csproj"},
			agentRequirement: "linux-dotnet",
		},
		{
			name:             "Unity or Tuanjie client package",
			file:             "unity-tuanjie-package.Jenkinsfile",
			stages:           3,
			environment:      2,
			stageNames:       []string{"Validate project", "Run EditMode tests", "Build client"},
			commandFragments: []string{"-executeMethod CI.Validate", "-runTests -testPlatform EditMode", "-executeMethod CI.BuildLinux64"},
			agentRequirement: "mac-unity",
			requiredWarnings: []string{"post_review_required"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("testdata", "jenkinsfiles", test.file))
			if err != nil {
				t.Fatal(err)
			}
			result, err := NewJenkinsfileStrategy().Convert(Request{
				Source: string(source),
				Name:   strings.TrimSuffix(test.file, filepath.Ext(test.file)),
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Summary.StageCount != test.stages || result.Summary.EnvironmentCount != test.environment {
				t.Fatalf("summary = %#v, want %d stages and %d environment variables", result.Summary, test.stages, test.environment)
			}
			if result.Hints.RepositoryURL != test.repositoryURLHint {
				t.Fatalf("repository hint = %q, want %q", result.Hints.RepositoryURL, test.repositoryURLHint)
			}
			for _, code := range test.requiredWarnings {
				if !hasWarning(result.Warnings, code) {
					t.Fatalf("warning %q missing from %#v", code, result.Warnings)
				}
			}

			config, err := engine.ParsePipelineConfig(result.Config)
			if err != nil {
				t.Fatalf("migrated config is invalid: %v", err)
			}
			if test.agentRequirement != "" {
				if len(config.AgentRequirements) != 1 || config.AgentRequirements[0] != test.agentRequirement {
					t.Fatalf("agent requirements = %#v, want %q", config.AgentRequirements, test.agentRequirement)
				}
			}
			if len(config.Stages) != len(test.stageNames) {
				t.Fatalf("stages = %d, want %d", len(config.Stages), len(test.stageNames))
			}
			for index, name := range test.stageNames {
				if config.Stages[index].Name != name {
					t.Fatalf("stage %d = %q, want %q", index, config.Stages[index].Name, name)
				}
			}
			var commands strings.Builder
			for _, stage := range config.Stages {
				for _, step := range stage.Steps {
					commands.WriteString(step.Command)
					commands.WriteByte('\n')
				}
			}
			for _, fragment := range test.commandFragments {
				if !strings.Contains(commands.String(), fragment) {
					t.Fatalf("generated commands do not contain %q:\n%s", fragment, commands.String())
				}
			}
			if config.Environment["PATH"] != "" && strings.Contains(config.Environment["PATH"], "${env.") {
				t.Fatalf("Jenkins environment reference was not normalized: %q", config.Environment["PATH"])
			}
		})
	}
}

func hasWarning(warnings []Warning, code string) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}

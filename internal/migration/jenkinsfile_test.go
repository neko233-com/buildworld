package migration

import (
	"fmt"
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
	if result.Version != ResultVersion || result.SourceFormat != "jenkinsfile" || result.TargetFormat != "buildworld-typescript" {
		t.Fatalf("result metadata = %#v", result)
	}
	if result.Summary.StageCount != 5 || result.Summary.EnvironmentCount != 3 {
		t.Fatalf("summary = %#v", result.Summary)
	}
	if result.Hints.RepositoryURL != "https://example.invalid/server-project.git" || result.Hints.DefaultBranch != "main" {
		t.Fatalf("hints = %#v", result.Hints)
	}
	if !strings.Contains(result.Config, "@buildworld/pipeline") || !strings.Contains(result.Config, "export default definePipeline") {
		t.Fatalf("migration did not emit a TypeScript pipeline:\n%s", result.Config)
	}
	if strings.Contains(result.Config, `\u003e`) || strings.Contains(result.Config, `\u0026`) {
		t.Fatalf("shell operators were HTML-escaped in TypeScript source")
	}
	if hasWarning(result.Warnings, "post_review_required") || hasWarning(result.Warnings, "options_review_required") {
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
	if got := config.Post["always"]; len(got) != 1 || !strings.Contains(got[0].Command, "构建流程结束") {
		t.Fatalf("post always = %#v", got)
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

func TestJenkinsfileStrategyPreservesBranchWhenCondition(t *testing.T) {
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: `
pipeline {
  agent any
  stages {
    stage('Deploy') {
      when { branch 'main' }
      steps { sh 'deploy-production' }
    }
  }
}`})
	if err != nil {
		t.Fatal(err)
	}
	config, err := engine.ParsePipelineConfig(result.Config)
	if err != nil {
		t.Fatal(err)
	}
	if got := config.Stages[0].Branches; len(got) != 1 || got[0] != "main" {
		t.Fatalf("stage branches = %#v", got)
	}
}

func TestJenkinsfileStrategyConvertsScriptedPipeline(t *testing.T) {
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: `
node {
  def TARGET_DIR = '/srv/game'
  def PYTHON_SCRIPT = 'restart.py'
  stage('Notify') {
    sh 'python3 notify.py'
  }
  stage('Restart') {
    dir(TARGET_DIR) {
      sh "python3 ${PYTHON_SCRIPT}"
    }
  }
}`})
	if err != nil {
		t.Fatal(err)
	}
	config, err := engine.ParsePipelineConfig(result.Config)
	if err != nil {
		t.Fatal(err)
	}
	if config.Environment["TARGET_DIR"] != "/srv/game" || config.Environment["PYTHON_SCRIPT"] != "restart.py" {
		t.Fatalf("scripted environment = %#v", config.Environment)
	}
	if len(config.Stages) != 2 {
		t.Fatalf("stages = %#v", config.Stages)
	}
	command := config.Stages[1].Steps[0].Command
	for _, expected := range []string{`cd "${TARGET_DIR}"`, `python3 ${PYTHON_SCRIPT}`} {
		if !strings.Contains(command, expected) {
			t.Fatalf("scripted stage missing %q:\n%s", expected, command)
		}
	}
}

func TestJenkinsfileStrategyBackgroundsJenkinsServiceMonitors(t *testing.T) {
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: `
pipeline {
  agent any
  stages {
    stage('Serve') {
      steps {
        sh '''
          go run ./cmd/server/main.go 2>&1 | tee "server.log"
        '''
      }
    }
    stage('Monitor') {
      steps {
        sh '''
          tail -f ${LOG_FILE} 2>/dev/null &
          TAIL_PID=$!
          while true; do
            sleep 5
          done
        '''
      }
    }
  }
}`})
	if err != nil {
		t.Fatal(err)
	}
	config, err := engine.ParsePipelineConfig(result.Config)
	if err != nil {
		t.Fatal(err)
	}
	serve := config.Stages[0].Steps[0].Command
	monitor := config.Stages[1].Steps[0].Command
	if !strings.Contains(serve, "nohup go run ./cmd/server/main.go > \"server.log\" 2>&1 &") || !strings.Contains(serve, "buildworld-service.pid") {
		t.Fatalf("foreground service was not backgrounded:\n%s", serve)
	}
	if strings.Contains(monitor, "while true") || !strings.Contains(monitor, "infinite tail monitor was removed") {
		t.Fatalf("infinite monitor was not removed:\n%s", monitor)
	}
}

func TestJenkinsfileStrategyConvertsPIDTailMonitorToNativeWatch(t *testing.T) {
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: `
pipeline {
  agent any
  environment {
    TARGET_DIR = "/srv/game"
    PID_FILE = "server.pid"
    LOG_FILE = "server.log"
  }
  stages {
    stage('Monitor') {
      steps {
        sh """
          cd ${TARGET_DIR}
          SERVER_PID=\$(cat ${PID_FILE})
          tail -f ${LOG_FILE} 2>/dev/null &
          TAIL_PID=\$!
          while true; do
            if ! kill -0 \${SERVER_PID} 2>/dev/null; then
              tail -50 ${LOG_FILE}
              exit 1
            fi
            sleep 5
          done
        """
      }
    }
  }
}`})
	if err != nil {
		t.Fatal(err)
	}
	config, err := engine.ParsePipelineConfig(result.Config)
	if err != nil {
		t.Fatal(err)
	}
	step := config.Stages[0].Steps[0]
	if step.Type != "service_watch" || !config.AllowLongRunning {
		t.Fatalf("monitor = %#v, allowLongRunning=%v", step, config.AllowLongRunning)
	}
	if step.Config["target_dir"] != "${build.TARGET_DIR}" || step.Config["pid_file"] != "${build.PID_FILE}" || step.Config["log_file"] != "${build.LOG_FILE}" {
		t.Fatalf("watch config = %#v", step.Config)
	}
}

func TestJenkinsfileStrategyPreservesExistingCheckoutWithoutJenkinsCredential(t *testing.T) {
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: `
pipeline {
  agent any
  stages {
    stage('Sync') { steps { sh '''
      git fetch --all
      git reset --hard origin/main
      git pull
    ''' } }
  }
}`})
	if err != nil {
		t.Fatal(err)
	}
	config, err := engine.ParsePipelineConfig(result.Config)
	if err != nil {
		t.Fatal(err)
	}
	command := config.Stages[0].Steps[0].Command
	for _, expected := range []string{
		`git fetch --all || echo "BuildWorld: git fetch unavailable; using existing checkout"`,
		`git reset --hard origin/main || git reset --hard HEAD`,
		`git pull || echo "BuildWorld: git pull unavailable; using existing checkout"`,
	} {
		if !strings.Contains(command, expected) {
			t.Fatalf("missing %q:\n%s", expected, command)
		}
	}
}

func TestJenkinsfileStrategyFindsMovedTeamResourcesScript(t *testing.T) {
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: `pipeline { agent any stages { stage('Resources') { steps { sh '''
if [ -f ./update-team-resources.sh ]; then
  chmod +x ./update-team-resources.sh
  ./update-team-resources.sh
else
  echo "skip"
fi
''' } } } }`})
	if err != nil {
		t.Fatal(err)
	}
	config, err := engine.ParsePipelineConfig(result.Config)
	if err != nil {
		t.Fatal(err)
	}
	command := config.Stages[0].Steps[0].Command
	if !strings.Contains(command, "elif [ -f ./_scripts/deploy/update-team-resources.sh ]; then") || !strings.Contains(command, "./_scripts/deploy/update-team-resources.sh") {
		t.Fatalf("moved Team-Resources script was not mapped:\n%s", command)
	}
}

func TestJenkinsfileStrategyFallsBackForDirectTeamResourcesScript(t *testing.T) {
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: `pipeline { agent any stages { stage('Resources') { steps { sh '''
cd ${TARGET_DIR}
chmod +x ./update-team-resources.sh
./update-team-resources.sh
''' } } } }`})
	if err != nil {
		t.Fatal(err)
	}
	config, err := engine.ParsePipelineConfig(result.Config)
	if err != nil {
		t.Fatal(err)
	}
	command := config.Stages[0].Steps[0].Command
	for _, expected := range []string{
		"if [ -f ./update-team-resources.sh ]; then",
		"elif [ -f ./_scripts/deploy/update-team-resources.sh ]; then",
		"./_scripts/deploy/update-team-resources.sh",
	} {
		if !strings.Contains(command, expected) {
			t.Fatalf("direct Team-Resources script missing %q:\n%s", expected, command)
		}
	}
}

func TestJenkinsfileStrategyConvertsMacPipelineConfigXML(t *testing.T) {
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: `<?xml version="1.1" encoding="UTF-8"?>
<flow-definition>
  <description>Unity package</description>
  <definition class="org.jenkinsci.plugins.workflow.cps.CpsFlowDefinition">
    <script>pipeline {
      agent any
      parameters {
        string(name: 'VERSION', defaultValue: '1.0.0', description: 'release version')
        choice(name: 'PLATFORM', choices: ['WebGL', 'Android'], description: 'target')
        booleanParam(name: 'REBUILD_ALL', defaultValue: false, description: 'clean build')
      }
      triggers { cron('0 11 * * *') }
      stages {
        stage('Package') {
          steps {
            script {
              def root = '/srv/unity'
              def cmd = "${root}/build.py --version ${params.VERSION}"
              sh cmd
            }
          }
        }
      }
      post { always { archiveArtifacts artifacts: 'artifacts/*.zip, logs/*.log', allowEmptyArchive: true } }
    }</script>
  </definition>
</flow-definition>`})
	if err != nil {
		t.Fatal(err)
	}
	config, err := engine.ParsePipelineConfig(result.Config)
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != "Unity package" || len(config.Parameters) != 3 {
		t.Fatalf("config name/parameters = %q/%#v", config.Name, config.Parameters)
	}
	if config.Parameters[1].Type != "choice" || strings.Join(config.Parameters[1].Choices, ",") != "WebGL,Android" {
		t.Fatalf("choice parameter = %#v", config.Parameters[1])
	}
	if len(config.Triggers) != 2 || config.Triggers[1].Type != "schedule" || config.Triggers[1].Config["cron"] != "0 11 * * *" {
		t.Fatalf("triggers = %#v", config.Triggers)
	}
	if strings.Join(config.Artifacts, ",") != "artifacts/*.zip,logs/*.log" {
		t.Fatalf("artifacts = %#v", config.Artifacts)
	}
	if command := config.Stages[0].Steps[0].Command; !strings.Contains(command, "/srv/unity/build.py --version ${VERSION}") {
		t.Fatalf("dynamic command = %s", command)
	}
}

// Set BUILDWORLD_JENKINS_CONFIG_DIR to a copied JENKINS_HOME/jobs directory
// to validate every inline Pipeline config.xml before importing it. Keeping the
// source external avoids committing production job definitions or credentials.
func TestExternalJenkinsConfigXMLFixtures(t *testing.T) {
	root := os.Getenv("BUILDWORLD_JENKINS_CONFIG_DIR")
	if root == "" {
		t.Skip("BUILDWORLD_JENKINS_CONFIG_DIR not set")
	}
	var jobs int
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "config.xml" {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(source), "CpsFlowDefinition") {
			return nil
		}
		result, err := NewJenkinsfileStrategy().Convert(Request{Source: string(source), Name: strings.TrimSuffix(filepath.Base(filepath.Dir(path)), filepath.Ext(path))})
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		config, err := engine.ParsePipelineConfig(result.Config)
		if err != nil || len(config.Stages) == 0 {
			return fmt.Errorf("%s: invalid migrated pipeline: %w", path, err)
		}
		if len(result.Warnings) != 0 {
			return fmt.Errorf("%s: migration still requires review: %#v", path, result.Warnings)
		}
		jobs++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if jobs == 0 {
		t.Fatal("no inline Jenkins Pipeline config.xml fixtures found")
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
			requiredWarnings:  nil,
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
			requiredWarnings: nil,
		},
		{
			name:             "Java Maven package",
			file:             "java-maven-package.Jenkinsfile",
			stages:           3,
			environment:      2,
			stageNames:       []string{"Compile", "Unit tests", "Package JAR"},
			commandFragments: []string{"mvn -B -DskipTests compile", "mvn -B test", "mvn -B -DskipTests package"},
			requiredWarnings: nil,
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
			requiredWarnings: nil,
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
			if test.file == "java-maven-package.Jenkinsfile" {
				if config.TimeoutSec != 1200 || !config.DisableConcurrent {
					t.Fatalf("mapped options = timeout %d concurrent %t", config.TimeoutSec, config.DisableConcurrent)
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

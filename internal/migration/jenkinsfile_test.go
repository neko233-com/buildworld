package migration

import (
	"fmt"
	"os"
	"os/exec"
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

func TestTranslateJenkinsPIDStopStageUsesDevelopmentShutdownRules(t *testing.T) {
	command, ok := translateJenkinsPIDStopStage(`
script {
    def oldPid = sh(returnStdout: true, script: 'cat "$PID_FILE"').trim()
    sh "kill -TERM ${oldPid}"
}`)
	if !ok {
		t.Fatal("expected PID stop stage to translate")
	}
	for _, expected := range []string{
		`MAX_WAIT_SECONDS=10`,
		`POLL_INTERVAL_MS=200`,
		`Graceful shutdown countdown: ${REMAINING_SECONDS}s remaining.`,
		`Last 240 lines of previous server log:`,
		`Sending SIGKILL to previous server PID=$OLD_PID after graceful shutdown timeout.`,
		`belongs to a non-game process; removing stale PID file without signaling it.`,
	} {
		if !strings.Contains(command, expected) {
			t.Fatalf("translated stop command is missing %q:\n%s", expected, command)
		}
	}
	if strings.Contains(command, "seq 1 325") || strings.Contains(command, "65 seconds") {
		t.Fatalf("translated stop command still contains the obsolete 65-second timeout:\n%s", command)
	}
}

func TestTranslateJenkinsPIDStopStageRemovesStalePIDFile(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("POSIX shell is not available")
	}
	command, ok := translateJenkinsPIDStopStage(`
script {
    def oldPid = sh(returnStdout: true, script: 'cat "$PID_FILE"').trim()
    sh "kill -TERM ${oldPid}"
}`)
	if !ok {
		t.Fatal("expected PID stop stage to translate")
	}
	targetDir := t.TempDir()
	pidFile := filepath.Join(targetDir, "game-server.pid.txt")
	if err := os.WriteFile(pidFile, []byte("999999999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	shell := exec.Command("sh", "-c", command)
	shell.Dir = targetDir
	shell.Env = append(os.Environ(),
		"TARGET_DIR="+filepath.ToSlash(targetDir),
		"PID_FILE=game-server.pid.txt",
		"BINARY_NAME=server-game-sf",
	)
	output, err := shell.CombinedOutput()
	if err != nil {
		t.Fatalf("stale PID handling failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "no longer exists; removing stale PID file") {
		t.Fatalf("stale PID diagnostic missing:\n%s", output)
	}
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Fatalf("stale PID file remains: %v", err)
	}
}

func TestTranslateJenkinsPIDStopStagePrintsLogTailBeforeForcedTermination(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("POSIX shell is not available")
	}
	command, ok := translateJenkinsPIDStopStage(`
script {
    def oldPid = sh(returnStdout: true, script: 'cat "$PID_FILE"').trim()
    sh "kill -TERM ${oldPid}"
}`)
	if !ok {
		t.Fatal("expected PID stop stage to translate")
	}
	targetDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(targetDir, "game-server.pid.txt"), []byte("321\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "server.log"), []byte("LAST_LOG_LINE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(targetDir, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mockedProcessControl := `
kill() {
    case "$1" in
        -0) [ -f "$STATE_DIR/killed" ] && return 1; return 0 ;;
        -TERM) : > "$STATE_DIR/term"; return 0 ;;
        -KILL) : > "$STATE_DIR/killed"; return 0 ;;
        *) return 1 ;;
    esac
}
ps() { printf '%s\n' 'server-game-sf'; }
sleep() { printf '%s\n' "$1" >> "$STATE_DIR/sleeps"; }
`
	shell := exec.Command("sh", "-c", mockedProcessControl+command)
	shell.Dir = targetDir
	shell.Env = append(os.Environ(),
		"STATE_DIR="+filepath.ToSlash(stateDir),
		"TARGET_DIR="+filepath.ToSlash(targetDir),
		"PID_FILE=game-server.pid.txt",
		"LOG_FILE=server.log",
		"BINARY_NAME=server-game-sf",
		// A stale deployment environment must not extend the fixed 10-second
		// development shutdown window. sleep is mocked, so this remains fast.
		"GRACEFUL_SHUTDOWN_MAX_WAIT_SECONDS=65",
		"GRACEFUL_SHUTDOWN_POLL_INTERVAL_MS=5000",
	)
	output, err := shell.CombinedOutput()
	if err != nil {
		t.Fatalf("forced termination path failed: %v\n%s", err, output)
	}
	for _, expected := range []string{
		"Sending SIGTERM to previous server PID=321",
		"Graceful shutdown countdown: 10s remaining.",
		"Last 240 lines of previous server log:",
		"LAST_LOG_LINE",
		"Sending SIGKILL to previous server PID=321",
	} {
		if !strings.Contains(string(output), expected) {
			t.Fatalf("forced termination output missing %q:\n%s", expected, output)
		}
	}
	for _, name := range []string{"term", "killed"} {
		if _, err := os.Stat(filepath.Join(stateDir, name)); err != nil {
			t.Fatalf("missing %s signal marker: %v", name, err)
		}
	}
	sleepCalls, err := os.ReadFile(filepath.Join(stateDir, "sleeps"))
	if err != nil {
		t.Fatalf("read sleep calls: %v", err)
	}
	for _, value := range strings.Fields(string(sleepCalls)) {
		if value != "0.2" {
			t.Fatalf("poll sleep = %q, want 0.2 seconds", value)
		}
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

func TestJenkinsfileStrategyWarnsForMacOSProtectedDirectories(t *testing.T) {
	declarativeShell := func(command string) string {
		return `pipeline {
  agent any
  stages {
    stage('Build') {
      steps { sh '''` + command + `''' }
    }
  }
}`
	}
	scriptedShell := func(command string) string {
		return `node {
  stage('Build') { sh '''` + command + `''' }
}`
	}
	tests := []struct {
		name   string
		source string
		want   int
	}{
		{
			name: "absolute Desktop environment",
			source: `pipeline {
  agent any
  environment { TARGET_DIR = '/Users/buildworld/Desktop/project' }
  stages { stage('Build') { steps { sh 'make package' } } }
}`,
			want: 1,
		},
		{
			name:   "absolute Documents shell token",
			source: declarativeShell("cd /Users/buildworld/Documents/project\nmake package\n"),
			want:   1,
		},
		{
			name:   "absolute Downloads shell token",
			source: declarativeShell("cd /Users/buildworld/Downloads/project\nmake package\n"),
			want:   1,
		},
		{
			name:   "quoted dollar HOME prefix",
			source: declarativeShell("cd \"$HOME\"/Desktop/project\n"),
			want:   1,
		},
		{
			name:   "quoted braced HOME prefix",
			source: declarativeShell("cd \"${HOME}\"/Documents/project\n"),
			want:   1,
		},
		{
			name:   "scripted dollar HOME path",
			source: scriptedShell("cd $HOME/Downloads && make package\n"),
			want:   1,
		},
		{
			name:   "scripted tilde path",
			source: scriptedShell("cd ~/Desktop/project && make package\n"),
			want:   1,
		},
		{
			name: "declarative references are deduplicated",
			source: declarativeShell(`cd /Users/buildworld/Desktop/project
cp result "$HOME"/Documents/result
cp result $HOME/Downloads/result
`),
			want: 1,
		},
		{
			name:   "DesktopBackup is a sibling",
			source: declarativeShell("cd /Users/buildworld/DesktopBackup/project\n"),
			want:   0,
		},
		{
			name:   "quoted Desktop Project is a sibling",
			source: declarativeShell("cd \"/Users/buildworld/Desktop Project/project\"\n"),
			want:   0,
		},
		{
			name:   "escaped Desktop Project is a sibling",
			source: declarativeShell("cd /Users/buildworld/Desktop\\ Project/project\n"),
			want:   0,
		},
		{
			name:   "shell comment is ignored",
			source: declarativeShell("# cd /Users/buildworld/Desktop/project\necho ok\n"),
			want:   0,
		},
		{
			name: "HTTPS URL is ignored",
			source: `pipeline {
  agent any
  environment { REFERENCE_URL = 'https://example.invalid/Users/buildworld/Desktop/project' }
  stages { stage('Build') { steps { sh 'echo ok' } } }
}`,
			want: 0,
		},
		{
			name: "environment sibling with spaces is ignored",
			source: `pipeline {
  agent any
  environment { TARGET_DIR = '/Users/buildworld/Desktop Project/project' }
  stages { stage('Build') { steps { sh 'echo ok' } } }
}`,
			want: 0,
		},
		{
			name: "protected path in PATH style value",
			source: `pipeline {
  agent any
  environment { TOOL_PATH = '${env.PATH}:/Users/buildworld/Documents/tools' }
  stages { stage('Build') { steps { sh 'echo ok' } } }
}`,
			want: 1,
		},
		{
			name:   "Developer directory is unprotected",
			source: declarativeShell("cd /Users/buildworld/Developer/project\n"),
			want:   0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := NewJenkinsfileStrategy().Convert(Request{Source: test.source})
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, warning := range result.Warnings {
				if warning.Code == "macos_protected_directory" {
					count++
				}
			}
			if count != test.want {
				t.Fatalf("macOS protected-directory warnings = %d, want %d: %#v", count, test.want, result.Warnings)
			}
		})
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
    PORT = "10101"
  }
  stages {
    stage('Monitor') {
      steps {
        sh """
          cd ${TARGET_DIR}
          SERVER_PID=\$(cat ${PID_FILE})
          tail -f ${LOG_FILE} 2>/dev/null &
          TAIL_PID=\$!
          HEARTBEAT_INTERVAL=30
          while true; do
            if ! kill -0 \${SERVER_PID} 2>/dev/null; then
              tail -50 ${LOG_FILE}
              exit 1
            fi
            echo "[heartbeat] service is running (PID: \${SERVER_PID}, port: ${PORT})"
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
	if step.Config["port"] != "${build.PORT}" || step.Config["heartbeat_seconds"] != "300" || step.Config["poll_seconds"] != "5" || step.Config["initial_lines"] != "10" {
		t.Fatalf("watch cadence config = %#v", step.Config)
	}
}

func TestJenkinsfileStrategyPreservesShellBlocksBeforeNativeWatch(t *testing.T) {
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: `
pipeline {
  agent any
  environment {
    TARGET_DIR = "/srv/game"
    BINARY_NAME = "game-server"
    PID_FILE = "server.pid"
    LOG_FILE = "server.log"
    PORT = "10101"
  }
  stages {
    stage('构建与启动') {
      steps {
        script {
          echo "================= 编译 Go 项目 ================="
          sh """
            cd ${TARGET_DIR}
            go test ./...
            go build -o ${BINARY_NAME} ./cmd/server
          """

          echo "================= 启动服务器 ================="
          sh """
            cd ${TARGET_DIR}
            nohup ./${BINARY_NAME} > ${LOG_FILE} 2>&1 &
            SERVER_PID=\$!
            echo \${SERVER_PID} > ${PID_FILE}
          """

          echo "================= 启动日志监控 ================="
          sh """
            cd ${TARGET_DIR}
            SERVER_PID=\$(cat ${PID_FILE})
            tail -f ${LOG_FILE} 2>/dev/null &
            TAIL_PID=\$!
            HEARTBEAT_INTERVAL=30
            LAST_HEARTBEAT=\$(date +%s)
            while true; do
              if ! kill -0 \${SERVER_PID} 2>/dev/null; then
                tail -50 ${LOG_FILE}
                exit 1
              fi
              CURRENT_TIME=\$(date +%s)
              if [ \$((CURRENT_TIME - LAST_HEARTBEAT)) -ge \${HEARTBEAT_INTERVAL} ]; then
                echo "[心跳] 服务器运行正常 (PID: \${SERVER_PID}, 端口: ${PORT})"
                LAST_HEARTBEAT=\${CURRENT_TIME}
              fi
              sleep 5
            done
          """
        }
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
	if !config.AllowLongRunning {
		t.Fatal("pipeline with native service watch must allow long-running builds")
	}
	steps := config.Stages[0].Steps
	if len(steps) != 3 {
		t.Fatalf("steps = %#v", steps)
	}
	if steps[0].Name != "编译 Go 项目" || steps[0].Type != "shell" {
		t.Fatalf("compile step = %#v", steps[0])
	}
	if !strings.Contains(steps[0].Command, "go test ./...") || !strings.Contains(steps[0].Command, "go build -o ${BINARY_NAME}") {
		t.Fatalf("compile commands were lost:\n%s", steps[0].Command)
	}
	if steps[1].Name != "启动服务器" || steps[1].Type != "shell" {
		t.Fatalf("launch step = %#v", steps[1])
	}
	if !strings.Contains(steps[1].Command, "nohup ./${BINARY_NAME}") || !strings.Contains(steps[1].Command, "echo ${SERVER_PID} > ${PID_FILE}") {
		t.Fatalf("launch commands were lost:\n%s", steps[1].Command)
	}
	if steps[2].Name != "启动日志监控" || steps[2].Type != "service_watch" {
		t.Fatalf("watch step = %#v", steps[2])
	}
	if steps[2].Config["target_dir"] != "${build.TARGET_DIR}" || steps[2].Config["pid_file"] != "${build.PID_FILE}" || steps[2].Config["log_file"] != "${build.LOG_FILE}" {
		t.Fatalf("watch config = %#v", steps[2].Config)
	}
	if steps[2].Config["port"] != "${build.PORT}" || steps[2].Config["heartbeat_seconds"] != "300" || steps[2].Config["poll_seconds"] != "5" || steps[2].Config["initial_lines"] != "10" || steps[2].Config["stop_service_on_cancel"] != "true" || steps[2].Config["shutdown_timeout_seconds"] != "65" {
		t.Fatalf("watch cadence config = %#v", steps[2].Config)
	}
	for _, step := range steps[:2] {
		if strings.Contains(step.Command, "tail -f") || strings.Contains(step.Command, "while true") {
			t.Fatalf("monitor leaked into shell step %q:\n%s", step.Name, step.Command)
		}
	}
}

func TestJenkinsfileStrategyRecognizesTailFollowWithOptionsAndRedirectPID(t *testing.T) {
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: `
pipeline {
  agent any
  environment {
    TARGET_DIR = "/srv/game"
    PID_FILE = "game-server.pid.txt"
    LOG_FILE = "logs_game_server/server.log"
  }
  stages {
    stage('Live Log Monitor') {
      steps {
        sh '''
          cd "$TARGET_DIR"
          SERVER_PID="$(LC_ALL=C tr -cd '0-9' < "$PID_FILE")"
          tail -n 50 -F "$LOG_FILE" &
          TAIL_PID=$!
          while is_server_pid_alive "$SERVER_PID"; do
            kill -0 "$SERVER_PID" 2>/dev/null || exit 1
            sleep 1
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
	if !config.AllowLongRunning || len(config.Stages) != 1 || len(config.Stages[0].Steps) != 1 {
		t.Fatalf("expected one native long-running watch, got %#v", config)
	}
	watch := config.Stages[0].Steps[0]
	if watch.Type != "service_watch" || watch.Config["target_dir"] != "${build.TARGET_DIR}" || watch.Config["pid_file"] != "${build.PID_FILE}" || watch.Config["log_file"] != "${build.LOG_FILE}" || watch.Config["poll_seconds"] != "1" {
		t.Fatalf("watch = %#v", watch)
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
	if config.Stages[0].TimeoutSec != jenkinsGitStageTimeout {
		t.Fatalf("Git sync stage timeout = %d, want %d", config.Stages[0].TimeoutSec, jenkinsGitStageTimeout)
	}
	for _, expected := range []string{
		`echo "BuildWorld: starting non-interactive git fetch (60s HTTP idle timeout)"`,
		jenkinsGitSafetyEnv + ` git fetch --all || echo "WARNING: BuildWorld: git fetch unavailable; using stale existing checkout"`,
		`git reset --hard origin/main || git reset --hard HEAD`,
		`echo "BuildWorld: starting non-interactive git pull (60s HTTP idle timeout)"`,
		jenkinsGitSafetyEnv + ` git pull || echo "WARNING: BuildWorld: git pull unavailable; using stale existing checkout"`,
	} {
		if !strings.Contains(command, expected) {
			t.Fatalf("missing %q:\n%s", expected, command)
		}
	}
}

func TestJenkinsfileStrategyMakesLegacyWorkspaceCloneIdempotent(t *testing.T) {
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: `
pipeline {
  agent any
  stages {
    stage('Checkout Latest Main') {
      steps {
        deleteDir()
        checkout scm
        sh '''
          set -eu
          cd "$WORKSPACE"
          git clone --depth 1 --branch main 'https://example.invalid/game.git' .
          SOURCE_COMMIT="$(git rev-parse HEAD)"
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
	if len(config.Stages) != 1 || len(config.Stages[0].Steps) != 1 {
		t.Fatalf("stages = %#v", config.Stages)
	}
	command := config.Stages[0].Steps[0].Command
	for _, expected := range []string{
		`if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then`,
		`BuildWorld: workspace already contains the SCM checkout; skipping legacy git clone into .`,
		`git clone --depth 1 --branch main 'https://example.invalid/game.git' .`,
		`SOURCE_COMMIT="$(git rev-parse HEAD)"`,
	} {
		if !strings.Contains(command, expected) {
			t.Fatalf("missing %q:\n%s", expected, command)
		}
	}
	if strings.Contains(command, "deleteDir()") || strings.Contains(command, "checkout scm") {
		t.Fatalf("Jenkins SCM directives must not run after BuildWorld prepares the workspace:\n%s", command)
	}
	if strings.Index(command, "if git rev-parse --is-inside-work-tree") > strings.Index(command, "git clone --depth 1") {
		t.Fatalf("legacy clone appears before the workspace guard:\n%s", command)
	}
}

func TestNormalizeJenkinsWorkspaceCloneLeavesNonWorkspaceDestinationsUnchanged(t *testing.T) {
	command := `git clone --depth 1 https://example.invalid/resources.git "$RESOURCE_DIR"`
	if got := normalizeJenkinsWorkspaceClone(command); got != command {
		t.Fatalf("resource clone = %q, want unchanged %q", got, command)
	}
}

func TestJenkinsfileStrategyLegacyWorkspaceCloneRunsAfterSCMCheckout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("POSIX shell is not available")
	}

	root := t.TempDir()
	sourceRepository := filepath.Join(root, "source")
	runJenkinsMigrationGit(t, "init", sourceRepository)
	runJenkinsMigrationGit(t, "-C", sourceRepository, "config", "user.email", "buildworld-test@example.invalid")
	runJenkinsMigrationGit(t, "-C", sourceRepository, "config", "user.name", "BuildWorld Test")
	if err := os.WriteFile(filepath.Join(sourceRepository, "marker.txt"), []byte("source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runJenkinsMigrationGit(t, "-C", sourceRepository, "add", "marker.txt")
	runJenkinsMigrationGit(t, "-C", sourceRepository, "commit", "-m", "initial")
	runJenkinsMigrationGit(t, "-C", sourceRepository, "branch", "-M", "main")

	workspace := filepath.Join(root, "workspace")
	runJenkinsMigrationGit(t, "clone", "--branch", "main", sourceRepository, workspace)
	cloneURL := strings.ReplaceAll(filepath.ToSlash(sourceRepository), "'", "'\\''")
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: fmt.Sprintf(`
pipeline {
  agent any
  stages {
    stage('Checkout Latest Main') {
      steps { sh '''
        set -eu
        cd "$WORKSPACE"
        git clone --depth 1 --branch main '%s' .
        test -f marker.txt
      ''' }
    }
  }
}`, cloneURL)})
	if err != nil {
		t.Fatal(err)
	}
	config, err := engine.ParsePipelineConfig(result.Config)
	if err != nil {
		t.Fatal(err)
	}
	command := config.Stages[0].Steps[0].Command
	shell := exec.Command("sh", "-c", command)
	shell.Dir = workspace
	shell.Env = append(os.Environ(), "WORKSPACE="+filepath.ToSlash(workspace))
	output, err := shell.CombinedOutput()
	if err != nil {
		t.Fatalf("migrated legacy SCM checkout failed: %v\n%s\n%s", err, command, output)
	}
	if !strings.Contains(string(output), "workspace already contains the SCM checkout") {
		t.Fatalf("legacy clone did not report reuse:\n%s", output)
	}
}

func runJenkinsMigrationGit(t *testing.T, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
}

func TestJenkinsfileStrategyDoesNotCapMixedGitAndBuildStage(t *testing.T) {
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: `pipeline {
  agent any
  stages {
    stage('Build') { steps { sh '''
      git pull
      make all
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
	if config.Stages[0].TimeoutSec != 0 {
		t.Fatalf("mixed build stage timeout = %d, want pipeline/default timeout", config.Stages[0].TimeoutSec)
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

func TestJenkinsfileStrategyUsesPrivateMacOSFeishuHelperPath(t *testing.T) {
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: `pipeline {
  agent any
  stages {
    stage('飞书通知') {
      steps { sh '''
cd /Users/buildworld/Desktop/Code/Automation-Projects/jenkins-for-project-sf/feishu-robot
./feishu-robot game-config-refresh --channel game || echo "继续执行"
''' }
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
	command := config.Stages[0].Steps[0].Command
	want := `if [ ! -x "$HOME/Library/Application Support/buildworld/helpers/feishu-robot" ]; then
  printf '%s\n' 'BuildWorld: install the trusted feishu-robot helper at $HOME/Library/Application Support/buildworld/helpers/feishu-robot with mode 0700 before enabling this pipeline.' >&2
  exit 1
fi
cd "${TMPDIR:-/tmp}"
"$HOME/Library/Application Support/buildworld/helpers/feishu-robot" game-config-refresh --channel game || echo "继续执行"`
	if !strings.Contains(command, want) {
		t.Fatalf("Feishu helper was not launched from the stable private path:\n%s", command)
	}
	if strings.Contains(command, "/Users/buildworld/Desktop/") {
		t.Fatalf("protected Desktop executable leaked into the migrated command:\n%s", command)
	}
	foundWarning := false
	for _, warning := range result.Warnings {
		if warning.Code == "macos_feishu_helper_install_required" &&
			strings.Contains(warning.Message, "install or copy") &&
			strings.Contains(warning.Message, `"$HOME/Library/Application Support/buildworld/helpers/feishu-robot"`) &&
			strings.Contains(warning.Message, "0700") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Fatal("migrated helper must require private-path installation with mode 0700")
	}
	if strings.Contains(command, "eval") {
		t.Fatalf("migrated helper command must not use eval:\n%s", command)
	}
	for _, folder := range []string{"Documents", "Downloads"} {
		t.Run(folder, func(t *testing.T) {
			source := strings.ReplaceAll(
				`pipeline { agent any stages { stage('飞书通知') { steps { sh '''
cd /Users/buildworld/Desktop/tools/feishu-robot
./feishu-robot game-config-refresh --channel game
''' } } } }`,
				"/Desktop/", "/"+folder+"/",
			)
			migrated, err := NewJenkinsfileStrategy().Convert(Request{Source: source})
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := engine.ParsePipelineConfig(migrated.Config)
			if err != nil {
				t.Fatal(err)
			}
			migratedCommand := parsed.Stages[0].Steps[0].Command
			if strings.Contains(migratedCommand, "/Users/buildworld/"+folder+"/") ||
				!strings.Contains(migratedCommand, `"$HOME/Library/Application Support/buildworld/helpers/feishu-robot" game-config-refresh --channel game`) {
				t.Fatalf("%s helper was not migrated to the private path:\n%s", folder, migratedCommand)
			}
			if !hasWarning(migrated.Warnings, "macos_feishu_helper_install_required") {
				t.Fatalf("%s helper did not require private installation: %#v", folder, migrated.Warnings)
			}
		})
	}

	safeSource := strings.ReplaceAll(
		`pipeline { agent any stages { stage('飞书通知') { steps { sh '''
cd /Users/buildworld/Desktop/tools/feishu-robot
./feishu-robot game-config-refresh
''' } } } }`,
		"/Desktop/", "/Developer/",
	)
	safeResult, err := NewJenkinsfileStrategy().Convert(Request{Source: safeSource})
	if err != nil {
		t.Fatal(err)
	}
	safeConfig, err := engine.ParsePipelineConfig(safeResult.Config)
	if err != nil {
		t.Fatal(err)
	}
	safeCommand := safeConfig.Stages[0].Steps[0].Command
	if strings.Contains(safeCommand, `${TMPDIR:-/tmp}`) || !strings.Contains(safeCommand, "cd /Users/buildworld/Developer/tools/feishu-robot") {
		t.Fatalf("unprotected helper working directory was unexpectedly rewritten:\n%s", safeCommand)
	}
	if hasWarning(safeResult.Warnings, "macos_feishu_helper_install_required") {
		t.Fatalf("unprotected helper unexpectedly required private installation: %#v", safeResult.Warnings)
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

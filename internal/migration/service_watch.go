package migration

import (
	"strings"

	"github.com/neko233-com/buildworld/internal/engine"
)

// extractJenkinsServiceWatch recognizes the conventional Jenkins deployment
// monitor (tail -f + PID heartbeat loop) and maps it to BuildWorld's native,
// cancellable observer instead of retaining an opaque infinite shell loop.
func extractJenkinsServiceWatch(source, name string) (engine.Step, bool) {
	normalized := expandGroovyShellVariables(source)
	if !strings.Contains(normalized, "tail -f") || !strings.Contains(normalized, "while true") || !strings.Contains(normalized, "kill -0") {
		return engine.Step{}, false
	}
	tail := jenkinsMonitorTail.FindStringSubmatch(normalized)
	pid := jenkinsMonitorPID.FindStringSubmatch(normalized)
	if len(tail) != 2 || len(pid) != 2 {
		return engine.Step{}, false
	}
	target := ""
	if match := jenkinsMonitorTarget.FindStringSubmatch(normalized); len(match) == 2 {
		target = normalizeJenkinsWatchValue(match[1])
	}
	config := map[string]string{
		"pid_file":      normalizeJenkinsWatchValue(pid[1]),
		"log_file":      normalizeJenkinsWatchValue(tail[1]),
		"initial_lines": "10",
	}
	if target != "" {
		config["target_dir"] = target
	}
	return engine.Step{Name: name, Type: "service_watch", Config: config}, true
}

func normalizeJenkinsWatchValue(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "'\"")
	value = groovyVariableRef.ReplaceAllString(value, `${build.$1}`)
	return value
}

func hasServiceWatch(stages []engine.Stage) bool {
	for _, stage := range stages {
		for _, step := range stage.Steps {
			if step.Type == "service_watch" {
				return true
			}
		}
	}
	return false
}

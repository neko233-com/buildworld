package migration

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/neko233-com/buildworld/internal/engine"
)

var (
	jenkinsMonitorHeartbeat = regexp.MustCompile(`(?m)^\s*HEARTBEAT_INTERVAL\s*=\s*([0-9]+)\s*(?:#.*)?$`)
	jenkinsMonitorPoll      = regexp.MustCompile(`(?m)^\s*sleep\s+([0-9]+)\s*(?:#.*)?\r?\n\s*done\b`)
	jenkinsMonitorPort      = regexp.MustCompile(`(?im)(?:\bport|端口)\s*[:：=]\s*(\\?\$\{[A-Za-z_][A-Za-z0-9_]*\})`)
)

type jenkinsShellInvocation struct {
	start int
	end   int
	value string
	label string
	safe  bool
}

type jenkinsStepTitle struct {
	name  string
	start int
	end   int
	found bool
}

// extractJenkinsServiceWatch recognizes the conventional Jenkins deployment
// monitor (tail -f + PID heartbeat loop) and maps it to BuildWorld's native,
// cancellable observer instead of retaining an opaque infinite shell loop.
func extractJenkinsServiceWatch(source, name string) (engine.Step, bool) {
	normalized := expandGroovyShellVariables(source)
	if !jenkinsMonitorTail.MatchString(normalized) || !strings.Contains(normalized, "while ") || !strings.Contains(normalized, "kill -0") {
		return engine.Step{}, false
	}
	tail := jenkinsMonitorTail.FindStringSubmatch(normalized)
	pid := jenkinsMonitorPID.FindStringSubmatch(normalized)
	if len(pid) != 2 {
		pid = jenkinsMonitorPIDRead.FindStringSubmatch(normalized)
	}
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
		// BuildWorld owns this monitor after cutover. Unlike Jenkins' monitor
		// handover, explicitly cancelling the BuildWorld build must not leave
		// the deployed Go process behind.
		"stop_service_on_cancel":   "true",
		"shutdown_timeout_seconds": "65",
	}
	if target != "" {
		config["target_dir"] = target
	}
	if match := jenkinsMonitorPort.FindStringSubmatch(normalized); len(match) == 2 {
		config["port"] = normalizeJenkinsWatchValue(match[1])
	}
	if match := jenkinsMonitorHeartbeat.FindStringSubmatch(normalized); len(match) == 2 && jenkinsWatchSecondsInRange(match[1], 5, 86_400) {
		config["heartbeat_seconds"] = match[1]
	}
	if loop := strings.Index(normalized, "while "); loop >= 0 {
		matches := jenkinsMonitorPoll.FindAllStringSubmatch(normalized[loop:], -1)
		if len(matches) > 0 {
			// The final sleep controls PID/process polling only. Native log
			// following uses an independent realtime cadence, like tail -f.
			// A tail -50 exit diagnostic is deliberately unrelated.
			value := matches[len(matches)-1][1]
			if jenkinsWatchSecondsInRange(value, 1, 3_600) {
				config["poll_seconds"] = value
			}
		}
	}
	// Preserve the conventional Jenkins monitor handover protocol so a newer
	// deployment retires its predecessor without marking it as a crash.
	if strings.Contains(normalized, ".jenkins-monitor-owner") && strings.Contains(normalized, ".jenkins-monitor-handover") {
		config["owner_file"] = ".jenkins-monitor-owner"
		config["handover_file"] = ".jenkins-monitor-handover"
		config["owner_id"] = "${build.BUILD_NUMBER}"
	}
	return engine.Step{Name: name, Type: "service_watch", Config: config}, true
}

func jenkinsWatchSecondsInRange(value string, minimum, maximum int) bool {
	seconds, err := strconv.Atoi(value)
	return err == nil && seconds >= minimum && seconds <= maximum
}

// translateJenkinsStageWatchSteps keeps every shell block in a stage that also
// contains a Jenkins PID/tail monitor. A stage-level watch match must not
// replace compile or launch blocks that happen to precede the monitor.
func translateJenkinsStageWatchSteps(source, stageName string) ([]engine.Step, []Warning, bool) {
	if _, ok := extractJenkinsServiceWatch(source, stageName); !ok {
		return nil, nil, false
	}

	invocations, allParsed := findJenkinsShellInvocations(source)
	if len(invocations) == 0 {
		return nil, nil, false
	}

	watches := make([]bool, len(invocations))
	hasWatch := false
	canSplit := allParsed
	for index, invocation := range invocations {
		_, watches[index] = extractJenkinsServiceWatch(invocation.value, stageName)
		hasWatch = hasWatch || watches[index]
		canSplit = canSplit && invocation.safe
	}
	if !hasWatch {
		return nil, nil, false
	}
	if !canSplit {
		return translateJenkinsStageWatchFallback(source, stageName, invocations, watches)
	}

	steps := make([]engine.Step, 0, len(invocations))
	warnings := make([]Warning, 0)
	previousEnd := 0
	for index, invocation := range invocations {
		prefix := source[previousEnd:invocation.start]
		title := findJenkinsStepTitle(prefix)
		name := migratedJenkinsStepName(stageName, invocation.label, title.name, index, len(invocations))

		if watches[index] {
			// Jenkins title echoes become the native step name. Retain any
			// other executable statements that occur before the monitor.
			remainingPrefix := prefix
			if title.found {
				remainingPrefix = prefix[:title.start] + prefix[title.end:]
			}
			prefixCommand, prefixWarnings := translateJenkinsShell(remainingPrefix)
			warnings = append(warnings, prefixWarnings...)
			if strings.TrimSpace(prefixCommand) != "" {
				steps = append(steps, engine.Step{
					Name:    name + " preparation",
					Type:    "shell",
					Command: "set -e\n\n" + strings.TrimSpace(prefixCommand),
				})
			}
			watch, _ := extractJenkinsServiceWatch(invocation.value, name)
			steps = append(steps, watch)
			previousEnd = invocation.end
			continue
		}

		command, commandWarnings := translateJenkinsShell(prefix + source[invocation.start:invocation.end])
		warnings = append(warnings, commandWarnings...)
		if strings.TrimSpace(command) != "" {
			steps = append(steps, engine.Step{
				Name:    name,
				Type:    "shell",
				Command: "set -e\n\n" + strings.TrimSpace(command),
			})
		}
		previousEnd = invocation.end
	}

	trailingCommand, trailingWarnings := translateJenkinsShell(source[previousEnd:])
	warnings = append(warnings, trailingWarnings...)
	if strings.TrimSpace(trailingCommand) != "" {
		steps = append(steps, engine.Step{
			Name:    stageName + " completion",
			Type:    "shell",
			Command: "set -e\n\n" + strings.TrimSpace(trailingCommand),
		})
	}
	return steps, warnings, len(steps) > 0
}

// The safe splitter only enters plain script { ... } wrappers. For an unusual
// control-flow wrapper, remove just the monitor invocation, translate the
// complete remaining Groovy structure once, then append the native observer.
// This fallback favors retaining commands over the old lossy stage replacement.
func translateJenkinsStageWatchFallback(source, stageName string, invocations []jenkinsShellInvocation, watches []bool) ([]engine.Step, []Warning, bool) {
	cleaned := []byte(source)
	nativeWatches := make([]engine.Step, 0)
	for index, invocation := range invocations {
		if !watches[index] {
			continue
		}
		for cursor := invocation.start; cursor < invocation.end; cursor++ {
			if cleaned[cursor] != '\n' && cleaned[cursor] != '\r' {
				cleaned[cursor] = ' '
			}
		}
		name := migratedJenkinsStepName(stageName, invocation.label, "", index, len(invocations))
		watch, _ := extractJenkinsServiceWatch(invocation.value, name)
		nativeWatches = append(nativeWatches, watch)
	}

	command, warnings := translateJenkinsShell(string(cleaned))
	steps := make([]engine.Step, 0, 1+len(nativeWatches))
	if strings.TrimSpace(command) != "" {
		steps = append(steps, engine.Step{
			Name:    stageName,
			Type:    "shell",
			Command: "set -e\n\n" + strings.TrimSpace(command),
		})
	}
	steps = append(steps, nativeWatches...)
	warnings = appendWarning(warnings, "service_watch_structure_review_required", "A Jenkins service monitor used nested control flow. BuildWorld retained the other commands, but the resulting step grouping should be reviewed.")
	return steps, warnings, len(steps) > 0
}

func findJenkinsShellInvocations(source string) ([]jenkinsShellInvocation, bool) {
	mask := groovyCodeMask(source)
	safeAt := jenkinsSafeScriptPositions(mask)
	invocations := make([]jenkinsShellInvocation, 0)
	allParsed := true
	for cursor := 0; cursor < len(mask); {
		index, word := nextIdentifier(mask, cursor)
		if index < 0 {
			break
		}
		after := index + len(word)
		if word != "sh" {
			cursor = after
			continue
		}
		value, end, ok := parseStepArgument(source, mask, after)
		if !ok {
			allParsed = false
			cursor = after
			continue
		}
		invocations = append(invocations, jenkinsShellInvocation{
			start: index,
			end:   end,
			value: value,
			label: parseJenkinsStepLabel(source, mask, index, end),
			safe:  safeAt[index],
		})
		cursor = end
	}
	return invocations, allParsed
}

func jenkinsSafeScriptPositions(mask string) []bool {
	safeAt := make([]bool, len(mask))
	stack := make([]bool, 0)
	unsafeDepth := 0
	for index := 0; index < len(mask); index++ {
		safeAt[index] = unsafeDepth == 0
		switch mask[index] {
		case '{':
			scriptBlock := previousJenkinsIdentifier(mask, index) == "script"
			stack = append(stack, scriptBlock)
			if !scriptBlock {
				unsafeDepth++
			}
		case '}':
			if len(stack) == 0 {
				continue
			}
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !last {
				unsafeDepth--
			}
		}
	}
	return safeAt
}

func previousJenkinsIdentifier(mask string, position int) string {
	cursor := position - 1
	for cursor >= 0 && (mask[cursor] == ' ' || mask[cursor] == '\t' || mask[cursor] == '\r' || mask[cursor] == '\n') {
		cursor--
	}
	end := cursor + 1
	for cursor >= 0 && isIdentifierByte(mask[cursor]) {
		cursor--
	}
	if end == cursor+1 {
		return ""
	}
	return mask[cursor+1 : end]
}

func parseJenkinsStepLabel(source, mask string, start, end int) string {
	labelIndex := findToken(mask, "label", start, end)
	if labelIndex < 0 {
		return ""
	}
	colon := skipSpace(mask, labelIndex+len("label"), end)
	if colon >= end || mask[colon] != ':' {
		return ""
	}
	value, _, ok := parseGroovyString(source, skipSourceSpace(source, colon+1, end))
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func findJenkinsStepTitle(source string) jenkinsStepTitle {
	mask := groovyCodeMask(source)
	title := jenkinsStepTitle{}
	for cursor := 0; cursor < len(mask); {
		index := findToken(mask, "echo", cursor, len(mask))
		if index < 0 {
			break
		}
		value, end, ok := parseStepArgument(source, mask, index+len("echo"))
		if !ok {
			cursor = index + len("echo")
			continue
		}
		name := strings.TrimSpace(strings.Trim(strings.TrimSpace(value), "=*-#"))
		if name != "" {
			title = jenkinsStepTitle{name: name, start: index, end: end, found: true}
		}
		cursor = end
	}
	return title
}

func migratedJenkinsStepName(stageName, label, title string, index, total int) string {
	if strings.TrimSpace(label) != "" {
		return strings.TrimSpace(label)
	}
	if strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	if total <= 1 {
		return stageName
	}
	return fmt.Sprintf("%s %d", stageName, index+1)
}

func normalizeJenkinsWatchValue(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "'\"")
	value = strings.TrimPrefix(value, `\`)
	value = groovyVariableRef.ReplaceAllString(value, `${build.$1}`)
	value = shellVariableRef.ReplaceAllString(value, `${build.$1}`)
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

package migration

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/neko233-com/buildworld/internal/engine"
)

const (
	maxJenkinsfileBytes      = 2 << 20
	macOSFeishuHelperPath    = `$HOME/Library/Application Support/buildworld/helpers/feishu-robot`
	macOSFeishuHelperWarning = `This Jenkins step launched feishu-robot from a macOS protected directory. Before enabling the migrated pipeline, install or copy the trusted binary to "$HOME/Library/Application Support/buildworld/helpers/feishu-robot" and set its mode to 0700. BuildWorld does not copy it automatically.`
	jenkinsGitSafetyEnv      = `GIT_TERMINAL_PROMPT=0 GCM_INTERACTIVE=Never GIT_HTTP_LOW_SPEED_LIMIT=1 GIT_HTTP_LOW_SPEED_TIME=60`
	jenkinsGitStageTimeout   = 120
)

var (
	environmentAssignment = regexp.MustCompile(`(?m)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:"((?:\\.|[^"\\])*)"|'((?:\\.|[^'\\])*)'|([0-9]+)|\b(true|false)\b)\s*(?://.*)?$`)
	scriptedAssignment    = regexp.MustCompile(`(?m)^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:"((?:\\.|[^"\\])*)"|'((?:\\.|[^'\\])*)')\s*$`)
	envReference          = regexp.MustCompile(`\$\{env\.([A-Za-z_][A-Za-z0-9_]*)\}`)
	paramsReference       = regexp.MustCompile(`\$\{params\.([A-Za-z_][A-Za-z0-9_]*)\}`)
	groovyAssignment      = regexp.MustCompile(`(?m)\bdef\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*`)
	groovyVariableRef     = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	shellVariableRef      = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
	bareShellVariable     = regexp.MustCompile(`(?m)(\bsh\s+)([A-Za-z_][A-Za-z0-9_]*)\s*$`)
	artifactDeclaration   = regexp.MustCompile(`(?s)\barchiveArtifacts\s*(?:\(\s*)?artifacts\s*:\s*`)
	persistentTailMonitor = regexp.MustCompile(`(?s)\n\s*tail\s+-f[^\n]*&\s*\n\s*TAIL_PID=\$!\s*\n\s*while\s+true;\s+do.*\n\s*done\s*$`)
	// Jenkins accepts both `tail -f log` and `tail -n 50 -F log`.  The
	// latter is common for production monitors because it emits recent context
	// before following the file.
	jenkinsMonitorTail    = regexp.MustCompile(`(?im)\btail\s+(?:(?:-[^\s]+)(?:\s+\d+)?\s+)*-[^\s]*f[^\s]*\s+([^\s;&|]+)`)
	jenkinsMonitorPID     = regexp.MustCompile(`(?m)\bSERVER_PID\s*=\s*\\?\$?\(\s*cat\s+([^\s)]+)`)
	jenkinsMonitorPIDRead = regexp.MustCompile(`(?m)\bSERVER_PID\s*=.*<\s*([^\s)]+)`)
	jenkinsMonitorTarget  = regexp.MustCompile(`(?m)^\s*cd\s+([^\s;&|]+)`)
	foregroundService     = regexp.MustCompile(`(?m)^(\s*)(go\s+run\s+\./cmd/server/main\.go|\./\$?\{?(?:BINARY_NAME|BINARY_NAME)\}?)[ \t]+2>&1[ \t]*\|[ \t]*tee[ \t]+([^\r\n]+)$`)
	teamResourcesFallback = regexp.MustCompile(`(?s)if\s+\[\s+-f\s+\./update-team-resources\.sh\s+\];\s+then(.*?)\n\s*else(.*?)\n\s*fi`)
	teamResourcesDirect   = regexp.MustCompile(`(?m)^(\s*)chmod\s+\+x\s+\./update-team-resources\.sh\s*\n\s*\./update-team-resources\.sh\s*$`)
	macOSFeishuHelper     = regexp.MustCompile(`(?m)^([ \t]*)cd[ \t]+(/Users/[A-Za-z0-9._@%+=:,~-]+/(Desktop|Documents|Downloads)/[A-Za-z0-9._/@%+=:,~-]*feishu-robot)[ \t]*\n[ \t]*\./feishu-robot([^\r\n]*)$`)
	jenkinsGitFetch       = regexp.MustCompile(`(?m)^([ \t]*)(git\s+fetch[^\r\n|]*)[ \t]*$`)
	jenkinsGitPull        = regexp.MustCompile(`(?m)^([ \t]*)(git\s+pull[^\r\n|]*)[ \t]*$`)
	jenkinsGitResetRemote = regexp.MustCompile(`(?m)^(\s*git\s+reset\s+--hard\s+origin/[^\s|]+)\s*$`)
	retentionDeclaration  = regexp.MustCompile(`(?s)numToKeepStr\s*:\s*['\"](\d+)['\"]`)
	timeoutDeclaration    = regexp.MustCompile(`(?is)\btimeout\s*\(.*?time\s*:\s*(\d+).*?unit\s*:\s*['\"]([A-Z]+)['\"].*?\)`)
	fileExistsCondition   = regexp.MustCompile(`^(!?)\s*fileExists\s*\((.*)\)\s*$`)
	fileExistsAssignment  = regexp.MustCompile(`(?m)\bdef\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*fileExists\s*\(([^\r\n]+)\)`)
	statusAssignment      = regexp.MustCompile(`^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*$`)
	statusCondition       = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*(==|!=)\s*(\d+)\s*$`)
	originBranch          = regexp.MustCompile(`\borigin/([A-Za-z0-9._/-]+)`)
)

type JenkinsfileStrategy struct{}

func NewJenkinsfileStrategy() *JenkinsfileStrategy { return &JenkinsfileStrategy{} }

// JenkinsfileToBuildConfig converts a Jenkinsfile directly into the canonical
// engine.BuildConfig, bypassing the migration Result/TS-encoding wrapper. It is
// used by the live pipeline engine (via engine.JenkinsfileConverter) so
// Jenkinsfile becomes a first-class source format alongside TypeScript and YAML.
func JenkinsfileToBuildConfig(source, name string) (*engine.BuildConfig, []Warning, error) {
	result, err := NewJenkinsfileStrategy().Convert(Request{Source: source, Name: name})
	if err != nil {
		return nil, nil, err
	}
	config, err := engine.ParsePipelineConfig(result.Config)
	if err != nil {
		return nil, nil, fmt.Errorf("validate converted Jenkinsfile: %w", err)
	}
	return config, result.Warnings, nil
}

func init() {
	engine.JenkinsfileConverter = func(source, name string) (*engine.BuildConfig, []string, error) {
		config, warnings, err := JenkinsfileToBuildConfig(source, name)
		if err != nil {
			return nil, nil, err
		}
		messages := make([]string, 0, len(warnings))
		for _, warning := range warnings {
			messages = append(messages, warning.Message)
		}
		return config, messages, nil
	}
}

func (*JenkinsfileStrategy) SourceFormat() string { return "jenkinsfile" }

func (*JenkinsfileStrategy) Convert(request Request) (*Result, error) {
	source := strings.TrimPrefix(strings.ReplaceAll(request.Source, "\r\n", "\n"), "\ufeff")
	if strings.TrimSpace(source) == "" {
		return nil, fmt.Errorf("Jenkinsfile cannot be empty")
	}
	if len(source) > maxJenkinsfileBytes {
		return nil, fmt.Errorf("Jenkinsfile exceeds the 2 MiB migration limit")
	}
	if strings.HasPrefix(strings.TrimSpace(source), "<") || strings.HasPrefix(strings.TrimSpace(source), "<?xml") {
		var description string
		var err error
		source, description, err = extractJenkinsPipelineXML(source)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(request.Name) == "" {
			request.Name = description
		}
	}
	mask := groovyCodeMask(source)
	pipeline, ok := namedBlock(source, mask, "pipeline", 0, len(source))
	if !ok {
		return convertScriptedJenkinsfile(request, source, mask)
	}

	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = "Migrated Jenkins pipeline"
	}
	config := &engine.BuildConfig{
		Name:        name,
		Description: "Migrated from Jenkinsfile",
		Environment: map[string]string{},
		Triggers:    []engine.Trigger{{Type: "manual", Config: map[string]string{}}},
	}
	warnings := make([]Warning, 0)

	pipelineSource := source[pipeline.start:pipeline.end]
	pipelineMask := mask[pipeline.start:pipeline.end]
	if environment, found := namedBlock(pipelineSource, pipelineMask, "environment", 0, len(pipelineSource)); found {
		parseJenkinsEnvironment(pipelineSource[environment.start:environment.end], config.Environment, &warnings)
	}
	if parameters, found := namedBlock(pipelineSource, pipelineMask, "parameters", 0, len(pipelineSource)); found {
		parseJenkinsParameters(pipelineSource[parameters.start:parameters.end], config, &warnings)
	}
	if triggers, found := namedBlock(pipelineSource, pipelineMask, "triggers", 0, len(pipelineSource)); found {
		parseJenkinsTriggers(pipelineSource[triggers.start:triggers.end], config, &warnings)
	}
	parseJenkinsArtifacts(pipelineSource, config)
	parseJenkinsAgent(pipelineSource, pipelineMask, config, &warnings)
	if options, found := namedBlock(pipelineSource, pipelineMask, "options", 0, len(pipelineSource)); found {
		parseJenkinsOptions(pipelineSource[options.start:options.end], config, &warnings)
	}

	stagesBlock, found := namedBlock(pipelineSource, pipelineMask, "stages", 0, len(pipelineSource))
	if !found {
		return nil, fmt.Errorf("declarative Jenkinsfile must contain stages { ... }")
	}
	stageSource := pipelineSource[stagesBlock.start:stagesBlock.end]
	stages, err := parseJenkinsStages(stageSource, &warnings)
	if err != nil {
		return nil, err
	}
	config.Stages = stages
	if hasServiceWatch(stages) {
		config.AllowLongRunning = true
	}

	if post, found := namedBlock(pipelineSource, pipelineMask, "post", 0, len(pipelineSource)); found {
		parseJenkinsPost(pipelineSource[post.start:post.end], config, &warnings)
	}

	encoded, err := engine.FormatTypeScriptPipeline(config)
	if err != nil {
		return nil, err
	}
	if _, err := engine.ParsePipelineConfig(encoded); err != nil {
		return nil, fmt.Errorf("validate migrated pipeline: %w", err)
	}
	warnings = appendMacOSProtectedDirectoryWarning(warnings, config)
	warnings = appendMacOSFeishuHelperInstallWarning(warnings, source)

	hints := Hints{RepositoryURL: config.Environment["GIT_REPO_URL"], DefaultBranch: "main"}
	if match := originBranch.FindStringSubmatch(source); len(match) == 2 {
		hints.DefaultBranch = match[1]
	}
	return &Result{
		Version:      ResultVersion,
		SourceFormat: "jenkinsfile",
		TargetFormat: "buildworld-typescript",
		Config:       encoded,
		Warnings:     warnings,
		Summary:      Summary{StageCount: len(config.Stages), EnvironmentCount: len(config.Environment)},
		Hints:        hints,
	}, nil
}

func convertScriptedJenkinsfile(request Request, source, mask string) (*Result, error) {
	node, ok := namedBlock(source, mask, "node", 0, len(source))
	if !ok {
		return nil, fmt.Errorf("Jenkinsfile must contain declarative pipeline { ... } or scripted node { ... }")
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = "Migrated Jenkins pipeline"
	}
	config := &engine.BuildConfig{
		Name:        name,
		Description: "Migrated from Jenkins scripted pipeline",
		Environment: map[string]string{},
		Triggers:    []engine.Trigger{{Type: "manual", Config: map[string]string{}}},
	}
	warnings := make([]Warning, 0)
	nodeSource := source[node.start:node.end]
	parseScriptedEnvironment(nodeSource, config.Environment, &warnings)
	stages, err := parseScriptedJenkinsStages(nodeSource, &warnings)
	if err != nil {
		return nil, err
	}
	config.Stages = stages
	if hasServiceWatch(stages) {
		config.AllowLongRunning = true
	}

	encoded, err := engine.FormatTypeScriptPipeline(config)
	if err != nil {
		return nil, err
	}
	if _, err := engine.ParsePipelineConfig(encoded); err != nil {
		return nil, fmt.Errorf("validate migrated pipeline: %w", err)
	}
	warnings = appendMacOSProtectedDirectoryWarning(warnings, config)
	warnings = appendMacOSFeishuHelperInstallWarning(warnings, source)
	hints := Hints{RepositoryURL: config.Environment["GIT_REPO_URL"], DefaultBranch: "main"}
	if match := originBranch.FindStringSubmatch(source); len(match) == 2 {
		hints.DefaultBranch = match[1]
	}
	return &Result{
		Version:      ResultVersion,
		SourceFormat: "jenkinsfile",
		TargetFormat: "buildworld-typescript",
		Config:       encoded,
		Warnings:     warnings,
		Summary:      Summary{StageCount: len(config.Stages), EnvironmentCount: len(config.Environment)},
		Hints:        hints,
	}, nil
}

type sourceRange struct {
	start int
	end   int
}

// extractJenkinsPipelineXML accepts a Pipeline job's config.xml directly. This
// lets an operator move the exact job definition out of JENKINS_HOME without
// copying its Groovy script by hand. Jenkins writes XML 1.1; encoding/xml only
// accepts 1.0 declarations, while the document structure used here is shared.
func extractJenkinsPipelineXML(source string) (script, description string, err error) {
	source = strings.Replace(source, `version="1.1"`, `version="1.0"`, 1)
	source = strings.Replace(source, `version='1.1'`, `version='1.0'`, 1)
	var job struct {
		Description string `xml:"description"`
		Definition  struct {
			Class  string `xml:"class,attr"`
			Script string `xml:"script"`
		} `xml:"definition"`
	}
	if err := xml.Unmarshal([]byte(source), &job); err != nil {
		return "", "", fmt.Errorf("parse Jenkins job config.xml: %w", err)
	}
	if !strings.Contains(job.Definition.Class, "CpsFlowDefinition") || strings.TrimSpace(job.Definition.Script) == "" {
		return "", "", fmt.Errorf("Jenkins config.xml must contain an inline Pipeline script; SCM-backed and folder jobs must be exported with their Jenkinsfile")
	}
	return job.Definition.Script, strings.TrimSpace(job.Description), nil
}

func parseJenkinsParameters(source string, config *engine.BuildConfig, warnings *[]Warning) {
	mask := groovyCodeMask(source)
	for position := 0; position < len(mask); {
		index, kind := nextIdentifier(mask, position)
		if index < 0 {
			return
		}
		if kind != "string" && kind != "booleanParam" && kind != "choice" && kind != "password" {
			position = index + len(kind)
			continue
		}
		open := skipSpace(mask, index+len(kind), len(mask))
		if open >= len(mask) || mask[open] != '(' {
			position = index + len(kind)
			continue
		}
		close, ok := matchingDelimiter(mask, open, '(', ')', len(mask))
		if !ok {
			*warnings = appendWarning(*warnings, "parameter_review_required", "A Jenkins parameter has an unclosed declaration and needs review.")
			return
		}
		arguments := source[open+1 : close]
		name, found := groovyNamedString(arguments, "name")
		if !found || name == "" {
			*warnings = appendWarning(*warnings, "parameter_review_required", "A Jenkins parameter without a literal name needs review.")
			position = close + 1
			continue
		}
		description, _ := groovyNamedString(arguments, "description")
		parameter := engine.BuildParameter{Name: name, Description: description}
		switch kind {
		case "booleanParam":
			parameter.Type = "boolean"
			if value, ok := groovyNamedBool(arguments, "defaultValue"); ok {
				parameter.Default = value
			}
		case "choice":
			parameter.Type = "choice"
			parameter.Choices = groovyChoices(arguments)
			if len(parameter.Choices) == 0 {
				*warnings = appendWarning(*warnings, "parameter_review_required", fmt.Sprintf("Parameter %q has no literal choices.", name))
			} else {
				parameter.Default = parameter.Choices[0]
			}
		case "password":
			parameter.Type = "string"
			parameter.IsSecret = true
			parameter.Required = true
		default:
			parameter.Type = "string"
			parameter.Default, _ = groovyNamedString(arguments, "defaultValue")
		}
		config.Parameters = append(config.Parameters, parameter)
		position = close + 1
	}
}

func parseJenkinsTriggers(source string, config *engine.BuildConfig, warnings *[]Warning) {
	mask := groovyCodeMask(source)
	for position := 0; position < len(mask); {
		index := findToken(mask, "cron", position, len(mask))
		if index < 0 {
			return
		}
		value, end, ok := parseStepArgument(source, mask, index+len("cron"))
		if !ok || strings.TrimSpace(value) == "" {
			*warnings = appendWarning(*warnings, "trigger_review_required", "A Jenkins cron trigger is not a literal string and needs review.")
			position = index + len("cron")
			continue
		}
		config.Triggers = append(config.Triggers, engine.Trigger{Type: "schedule", Config: map[string]string{"cron": strings.TrimSpace(value)}})
		position = end
	}
}

func parseJenkinsArtifacts(source string, config *engine.BuildConfig) {
	for _, match := range artifactDeclaration.FindAllStringIndex(source, -1) {
		value, _, ok := parseGroovyString(source, skipSourceSpace(source, match[1], len(source)))
		if !ok {
			continue
		}
		for _, pattern := range strings.Split(value, ",") {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" || containsString(config.Artifacts, pattern) {
				continue
			}
			config.Artifacts = append(config.Artifacts, pattern)
		}
	}
}

func parseJenkinsOptions(source string, config *engine.BuildConfig, warnings *[]Warning) {
	if match := retentionDeclaration.FindStringSubmatch(source); len(match) == 2 {
		if retain, err := strconv.Atoi(match[1]); err == nil {
			config.RetentionCompleted = retain
		}
	}
	if strings.Contains(source, "disableConcurrentBuilds") {
		config.DisableConcurrent = true
		config.AbortPrevious = strings.Contains(source, "abortPrevious: true")
	}
	if match := timeoutDeclaration.FindStringSubmatch(source); len(match) == 3 {
		if value, err := strconv.Atoi(match[1]); err == nil {
			multiplier := map[string]int{"SECONDS": 1, "MINUTES": 60, "HOURS": 3600}[strings.ToUpper(match[2])]
			if multiplier > 0 {
				config.TimeoutSec = value * multiplier
			}
		}
	}
	if hasUnsupportedJenkinsOption(source) {
		*warnings = appendWarning(*warnings, "options_review_required", "Some Jenkins options could not be represented in BuildWorld.")
	}
}

func hasUnsupportedJenkinsOption(source string) bool {
	allowed := map[string]bool{
		"skipDefaultCheckout": true, "timestamps": true, "buildDiscarder": true,
		"logRotator": true, "numToKeepStr": true, "disableConcurrentBuilds": true,
		"abortPrevious": true, "timeout": true, "time": true, "unit": true,
		"true": true, "false": true,
	}
	mask := groovyCodeMask(source)
	for position := 0; position < len(mask); {
		index, word := nextIdentifier(mask, position)
		if index < 0 {
			return false
		}
		if !allowed[word] {
			return true
		}
		position = index + len(word)
	}
	return false
}

func parseJenkinsPost(source string, config *engine.BuildConfig, warnings *[]Warning) {
	mask := groovyCodeMask(source)
	for position := 0; position < len(mask); {
		index, condition := nextIdentifier(mask, position)
		if index < 0 {
			break
		}
		open := skipSpace(mask, index+len(condition), len(mask))
		if open >= len(mask) || mask[open] != '{' {
			position = index + len(condition)
			continue
		}
		close, ok := matchingDelimiter(mask, open, '{', '}', len(mask))
		if !ok {
			*warnings = appendWarning(*warnings, "post_review_required", "A Jenkins post condition has an unclosed body.")
			return
		}
		if condition != "always" && condition != "success" && condition != "failure" && condition != "cleanup" {
			*warnings = appendWarning(*warnings, "post_review_required", fmt.Sprintf("Jenkins post condition %q is not supported.", condition))
			position = close + 1
			continue
		}
		command, postWarnings := translateJenkinsShell(source[open+1 : close])
		for _, warning := range postWarnings {
			*warnings = appendWarning(*warnings, warning.Code, "Post "+condition+": "+warning.Message)
		}
		if strings.TrimSpace(command) == "" {
			*warnings = appendWarning(*warnings, "post_review_required", fmt.Sprintf("Jenkins post condition %q has no supported commands.", condition))
		} else {
			if config.Post == nil {
				config.Post = map[string][]engine.Step{}
			}
			config.Post[condition] = append(config.Post[condition], engine.Step{Name: "post " + condition, Type: "shell", Shell: "bash", Command: "set -e\n\n" + strings.TrimSpace(command)})
		}
		position = close + 1
	}
}

func groovyNamedString(arguments, name string) (string, bool) {
	pattern := regexp.MustCompile(`(?s)\b` + regexp.QuoteMeta(name) + `\s*:\s*`)
	match := pattern.FindStringIndex(arguments)
	if match == nil {
		return "", false
	}
	value, _, ok := parseGroovyString(arguments, skipSourceSpace(arguments, match[1], len(arguments)))
	return value, ok
}

func groovyNamedBool(arguments, name string) (bool, bool) {
	pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(name) + `\s*:\s*(true|false)\b`)
	match := pattern.FindStringSubmatch(arguments)
	return len(match) == 2 && strings.EqualFold(match[1], "true"), len(match) == 2
}

func groovyChoices(arguments string) []string {
	index := strings.Index(arguments, "choices")
	if index < 0 {
		return nil
	}
	open := strings.Index(arguments[index:], "[")
	close := strings.Index(arguments[index:], "]")
	if open < 0 || close < open {
		return nil
	}
	list := arguments[index+open+1 : index+close]
	values := make([]string, 0)
	for position := 0; position < len(list); {
		position = skipSourceSpace(list, position, len(list))
		value, end, ok := parseGroovyString(list, position)
		if !ok {
			position++
			continue
		}
		if value != "" {
			values = append(values, value)
		}
		position = end
	}
	return values
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func namedBlock(source, mask, name string, from, limit int) (sourceRange, bool) {
	_ = source
	for position := from; position < limit; {
		index := findToken(mask, name, position, limit)
		if index < 0 {
			return sourceRange{}, false
		}
		cursor := skipSpace(mask, index+len(name), limit)
		if cursor < limit && mask[cursor] == '{' {
			end, ok := matchingDelimiter(mask, cursor, '{', '}', limit)
			if ok {
				return sourceRange{start: cursor + 1, end: end}, true
			}
			return sourceRange{}, false
		}
		position = index + len(name)
	}
	return sourceRange{}, false
}

func parseJenkinsEnvironment(source string, environment map[string]string, warnings *[]Warning) {
	matches := environmentAssignment.FindAllStringSubmatch(source, -1)
	for _, match := range matches {
		value := match[2]
		if value == "" && match[3] != "" {
			value = match[3]
		}
		if value == "" && match[4] != "" {
			value = match[4]
		}
		if value == "" && match[5] != "" {
			value = match[5]
		}
		value = strings.ReplaceAll(value, `\$`, `$`)
		value = envReference.ReplaceAllString(value, `${$1}`)
		environment[match[1]] = value
	}
	mask := groovyCodeMask(source)
	assignmentCount := strings.Count(mask, "=")
	if assignmentCount > len(matches) {
		*warnings = appendWarning(*warnings, "environment_review_required", "Some Jenkins environment expressions are not plain strings and require manual review.")
	}
}

func parseScriptedEnvironment(source string, environment map[string]string, warnings *[]Warning) {
	matches := scriptedAssignment.FindAllStringSubmatch(source, -1)
	for _, match := range matches {
		value := match[2]
		if value == "" && match[3] != "" {
			value = match[3]
		}
		value = strings.ReplaceAll(value, `\$`, `$`)
		value = envReference.ReplaceAllString(value, `${$1}`)
		environment[match[1]] = value
	}
	if strings.Contains(groovyCodeMask(source), "def ") && len(matches) == 0 {
		*warnings = appendWarning(*warnings, "scripted_environment_review_required", "Some scripted-pipeline variables are not plain strings and require manual review.")
	}
}

func parseJenkinsAgent(source, mask string, config *engine.BuildConfig, warnings *[]Warning) {
	index := findToken(mask, "agent", 0, len(mask))
	if index < 0 {
		return
	}
	cursor := skipSpace(mask, index+len("agent"), len(mask))
	if strings.HasPrefix(mask[cursor:], "any") || strings.HasPrefix(mask[cursor:], "none") {
		return
	}
	if cursor >= len(mask) || mask[cursor] != '{' {
		return
	}
	end, ok := matchingDelimiter(mask, cursor, '{', '}', len(mask))
	if !ok {
		return
	}
	body := source[cursor+1 : end]
	bodyMask := mask[cursor+1 : end]
	label := findToken(bodyMask, "label", 0, len(bodyMask))
	if label >= 0 {
		if value, _, ok := parseGroovyString(body, skipSourceSpace(body, label+len("label"), len(body))); ok && strings.TrimSpace(value) != "" {
			config.AgentRequirements = append(config.AgentRequirements, strings.TrimSpace(value))
			return
		}
	}
	*warnings = appendWarning(*warnings, "agent_review_required", "The Jenkins agent declaration is more specific than agent any; map its image or workspace requirements to a distributed Worker.")
}

func parseJenkinsStages(source string, warnings *[]Warning) ([]engine.Stage, error) {
	mask := groovyCodeMask(source)
	stages := make([]engine.Stage, 0)
	for position := 0; position < len(source); {
		index := findToken(mask, "stage", position, len(mask))
		if index < 0 {
			break
		}
		cursor := skipSpace(mask, index+len("stage"), len(mask))
		if cursor >= len(mask) || mask[cursor] != '(' {
			position = index + len("stage")
			continue
		}
		closeParen, ok := matchingDelimiter(mask, cursor, '(', ')', len(mask))
		if !ok {
			return nil, fmt.Errorf("Jenkins stage near byte %d has an unclosed name", index)
		}
		stageName, _, ok := parseGroovyString(source, skipSourceSpace(source, cursor+1, closeParen))
		if !ok || strings.TrimSpace(stageName) == "" {
			return nil, fmt.Errorf("Jenkins stage near byte %d must use a quoted name", index)
		}
		cursor = skipSpace(mask, closeParen+1, len(mask))
		if cursor >= len(mask) || mask[cursor] != '{' {
			return nil, fmt.Errorf("Jenkins stage %q has no body", stageName)
		}
		closeBody, ok := matchingDelimiter(mask, cursor, '{', '}', len(mask))
		if !ok {
			return nil, fmt.Errorf("Jenkins stage %q has an unclosed body", stageName)
		}
		body := source[cursor+1 : closeBody]
		bodyMask := mask[cursor+1 : closeBody]
		branches, whenWarnings := parseJenkinsWhen(body, bodyMask)
		for _, warning := range whenWarnings {
			*warnings = appendWarning(*warnings, warning.Code, fmt.Sprintf("Stage %q: %s", stageName, warning.Message))
		}
		stepsBlock, found := namedBlock(body, bodyMask, "steps", 0, len(body))
		if !found {
			*warnings = appendWarning(*warnings, "stage_without_steps", fmt.Sprintf("Stage %q has no declarative steps block and was skipped.", stageName))
			position = closeBody + 1
			continue
		}
		stepSource := body[stepsBlock.start:stepsBlock.end]
		if steps, stepWarnings, ok := translateJenkinsStageWatchSteps(stepSource, strings.TrimSpace(stageName)); ok {
			for _, warning := range stepWarnings {
				*warnings = appendWarning(*warnings, warning.Code, fmt.Sprintf("Stage %q: %s", stageName, warning.Message))
			}
			stages = append(stages, engine.Stage{Name: strings.TrimSpace(stageName), Branches: branches, Steps: steps})
			position = closeBody + 1
			continue
		}
		if watch, ok := extractJenkinsServiceWatch(stepSource, strings.TrimSpace(stageName)); ok {
			stages = append(stages, engine.Stage{Name: strings.TrimSpace(stageName), Branches: branches, Steps: []engine.Step{watch}})
			position = closeBody + 1
			continue
		}
		if command, ok := translateJenkinsPIDStopStage(stepSource); ok {
			stages = append(stages, engine.Stage{Name: strings.TrimSpace(stageName), Branches: branches, Steps: []engine.Step{{
				Name: strings.TrimSpace(stageName), Type: "shell", Shell: "bash", Command: "set -e\n\n" + command,
			}}})
			position = closeBody + 1
			continue
		}
		command, commandWarnings := translateJenkinsShell(stepSource)
		for _, warning := range commandWarnings {
			*warnings = appendWarning(*warnings, warning.Code, fmt.Sprintf("Stage %q: %s", stageName, warning.Message))
		}
		if strings.TrimSpace(command) == "" {
			*warnings = appendWarning(*warnings, "stage_without_supported_commands", fmt.Sprintf("Stage %q contains no supported sh/echo/script/dir commands and was skipped.", stageName))
			position = closeBody + 1
			continue
		}
		stage := engine.Stage{
			Name:     strings.TrimSpace(stageName),
			Branches: branches,
			Steps: []engine.Step{{
				Name:    strings.TrimSpace(stageName),
				Type:    "shell",
				Shell:   "bash",
				Command: "set -e\n\n" + strings.TrimSpace(command),
			}},
		}
		applyJenkinsGitStageTimeout(&stage)
		stages = append(stages, stage)
		position = closeBody + 1
	}
	if len(stages) == 0 {
		return nil, fmt.Errorf("Jenkinsfile has no stages with supported executable commands")
	}
	return stages, nil
}

// translateJenkinsPIDStopStage preserves the common declarative Jenkins
// pattern where a Groovy script captures `sh(returnStdout: true)` into oldPid
// before terminating the previous daemon.  BuildWorld does not execute Groovy,
// so flattening the inner sh blocks loses the captured value and can leave the
// old process bound to the service port.  Translate that idiom directly to its
// shell equivalent.
func translateJenkinsPIDStopStage(source string) (string, bool) {
	normalized := expandGroovyShellVariables(source)
	if !strings.Contains(normalized, "oldPid") || !strings.Contains(normalized, "PID_FILE") || !strings.Contains(normalized, "kill -TERM") {
		return "", false
	}
	return `cd "${TARGET_DIR:?missing TARGET_DIR}" || exit 0
OLD_PID=""
if [ -f "${PID_FILE:?missing PID_FILE}" ]; then
    OLD_PID="$(LC_ALL=C tr -cd '0-9' < "$PID_FILE" | head -c 20 || true)"
fi
if ! printf '%s' "$OLD_PID" | grep -Eq '^[1-9][0-9]*$'; then
    echo 'No valid live PID was found. The next server overwrites the stale PID file.'
    exit 0
fi
if ! kill -0 "$OLD_PID" 2>/dev/null; then
    echo "Previous server PID=$OLD_PID no longer exists; removing stale PID file."
    rm -f "$PID_FILE"
    exit 0
fi
OLD_PROCESS_COMMAND="$(ps -p "$OLD_PID" -o command= 2>/dev/null || true)"
case "$OLD_PROCESS_COMMAND" in
    *"${BINARY_NAME:?missing BINARY_NAME}"*) ;;
    *)
        echo "PID=$OLD_PID belongs to a non-game process; removing stale PID file without signaling it. command=$OLD_PROCESS_COMMAND"
        rm -f "$PID_FILE"
        exit 0
        ;;
esac
# Development game servers must either leave cleanly in this bounded window or
# be replaced. Do not inherit a stale deployment environment timeout here.
MAX_WAIT_SECONDS=10
POLL_INTERVAL_MS=200
MAX_WAIT_ATTEMPTS="$((MAX_WAIT_SECONDS * 1000 / POLL_INTERVAL_MS))"
POLL_INTERVAL_SECONDS=0.2
echo "Sending SIGTERM to previous server PID=$OLD_PID; maxWaitSeconds=$MAX_WAIT_SECONDS"
kill -TERM "$OLD_PID"
LAST_REMAINING_SECONDS=-1
for i in $(seq 1 "$MAX_WAIT_ATTEMPTS"); do
    kill -0 "$OLD_PID" 2>/dev/null || exit 0
    ELAPSED_MS="$(((i - 1) * POLL_INTERVAL_MS))"
    REMAINING_SECONDS="$(((MAX_WAIT_SECONDS * 1000 - ELAPSED_MS + 999) / 1000))"
    if [ "$REMAINING_SECONDS" -ne "$LAST_REMAINING_SECONDS" ]; then
        echo "Graceful shutdown countdown: ${REMAINING_SECONDS}s remaining."
        LAST_REMAINING_SECONDS="$REMAINING_SECONDS"
    fi
    sleep "$POLL_INTERVAL_SECONDS"
done
echo "ERROR: Previous server did not exit within $MAX_WAIT_SECONDS seconds; final database flush is considered failed." >&2
if [ -n "${LOG_FILE:-}" ] && [ -f "$LOG_FILE" ]; then
    echo 'Last 240 lines of previous server log:' >&2
    tail -n 240 "$LOG_FILE" >&2 || true
fi
echo "Sending SIGKILL to previous server PID=$OLD_PID after graceful shutdown timeout." >&2
kill -KILL "$OLD_PID" 2>/dev/null || true
for i in $(seq 1 25); do
    kill -0 "$OLD_PID" 2>/dev/null || exit 0
    sleep "$POLL_INTERVAL_SECONDS"
done
echo "ERROR: Previous server PID=$OLD_PID remained alive after SIGKILL; refusing to start a conflicting replacement." >&2
exit 1`, true
}

func parseScriptedJenkinsStages(source string, warnings *[]Warning) ([]engine.Stage, error) {
	mask := groovyCodeMask(source)
	stages := make([]engine.Stage, 0)
	for position := 0; position < len(source); {
		index := findToken(mask, "stage", position, len(mask))
		if index < 0 {
			break
		}
		cursor := skipSpace(mask, index+len("stage"), len(mask))
		if cursor >= len(mask) || mask[cursor] != '(' {
			position = index + len("stage")
			continue
		}
		closeParen, ok := matchingDelimiter(mask, cursor, '(', ')', len(mask))
		if !ok {
			return nil, fmt.Errorf("Jenkins stage near byte %d has an unclosed name", index)
		}
		stageName, _, ok := parseGroovyString(source, skipSourceSpace(source, cursor+1, closeParen))
		if !ok || strings.TrimSpace(stageName) == "" {
			return nil, fmt.Errorf("Jenkins stage near byte %d must use a quoted name", index)
		}
		cursor = skipSpace(mask, closeParen+1, len(mask))
		if cursor >= len(mask) || mask[cursor] != '{' {
			return nil, fmt.Errorf("Jenkins stage %q has no body", stageName)
		}
		closeBody, ok := matchingDelimiter(mask, cursor, '{', '}', len(mask))
		if !ok {
			return nil, fmt.Errorf("Jenkins stage %q has an unclosed body", stageName)
		}
		stageSource := source[cursor+1 : closeBody]
		if watch, ok := extractJenkinsServiceWatch(stageSource, strings.TrimSpace(stageName)); ok {
			stages = append(stages, engine.Stage{Name: strings.TrimSpace(stageName), Steps: []engine.Step{watch}})
			position = closeBody + 1
			continue
		}
		command, commandWarnings := translateJenkinsShell(stageSource)
		for _, warning := range commandWarnings {
			*warnings = appendWarning(*warnings, warning.Code, fmt.Sprintf("Stage %q: %s", stageName, warning.Message))
		}
		if strings.TrimSpace(command) == "" {
			*warnings = appendWarning(*warnings, "stage_without_supported_commands", fmt.Sprintf("Stage %q contains no supported sh/echo/script/dir commands and was skipped.", stageName))
			position = closeBody + 1
			continue
		}
		stage := engine.Stage{Name: strings.TrimSpace(stageName), Steps: []engine.Step{{Name: strings.TrimSpace(stageName), Type: "shell", Shell: "bash", Command: "set -e\n\n" + strings.TrimSpace(command)}}}
		applyJenkinsGitStageTimeout(&stage)
		stages = append(stages, stage)
		position = closeBody + 1
	}
	if len(stages) == 0 {
		return nil, fmt.Errorf("scripted Jenkinsfile has no stages with supported executable commands")
	}
	return stages, nil
}

// A local credential helper or pre-fetch hook can block before Git opens its
// HTTP connection, where low-speed safeguards cannot help. Bound migrated Git
// sync stages independently instead of consuming the whole build timeout.
func applyJenkinsGitStageTimeout(stage *engine.Stage) {
	for _, step := range stage.Steps {
		if isPureJenkinsGitSyncCommand(step.Command) {
			stage.TimeoutSec = jenkinsGitStageTimeout
			return
		}
	}
}

func isPureJenkinsGitSyncCommand(command string) bool {
	foundSync := false
	for _, line := range strings.Split(command, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "", strings.HasPrefix(line, "#"), strings.HasPrefix(line, "set "),
			strings.HasPrefix(line, "echo "), strings.HasPrefix(line, "printf "), strings.HasPrefix(line, "cd "),
			strings.HasPrefix(line, "git reset "):
			continue
		case strings.HasPrefix(line, jenkinsGitSafetyEnv+" git fetch"), strings.HasPrefix(line, jenkinsGitSafetyEnv+" git pull"):
			foundSync = true
		default:
			return false
		}
	}
	return foundSync
}

func parseJenkinsWhen(source, mask string) ([]string, []Warning) {
	block, found := namedBlock(source, mask, "when", 0, len(source))
	if !found {
		return nil, nil
	}
	body := source[block.start:block.end]
	bodyMask := mask[block.start:block.end]
	branches := make([]string, 0)
	for position := 0; position < len(bodyMask); {
		index := findToken(bodyMask, "branch", position, len(bodyMask))
		if index < 0 {
			break
		}
		value, end, ok := parseGroovyString(body, skipSourceSpace(body, index+len("branch"), len(body)))
		if ok && strings.TrimSpace(value) != "" {
			branches = append(branches, strings.TrimSpace(value))
			position = end
			continue
		}
		position = index + len("branch")
	}
	if len(branches) > 0 {
		return branches, nil
	}
	return nil, []Warning{{Code: "when_review_required", Message: "Jenkins when condition could not be converted. Configure an equivalent BuildWorld branch or approval policy before enabling this stage."}}
}

func translateJenkinsShell(source string) (string, []Warning) {
	source = expandGroovyShellVariables(source)
	source = expandFileExistsVariables(source)
	mask := groovyCodeMask(source)
	lines := make([]string, 0)
	warnings := make([]Warning, 0)
	for cursor := 0; cursor < len(source); {
		index, word := nextIdentifier(mask, cursor)
		if index < 0 {
			break
		}
		after := index + len(word)
		switch word {
		case "script":
			open := skipSpace(mask, after, len(mask))
			if open < len(mask) && mask[open] == '{' {
				close, ok := matchingDelimiter(mask, open, '{', '}', len(mask))
				if ok {
					command, nestedWarnings := translateJenkinsShell(source[open+1 : close])
					appendCommand(&lines, command)
					warnings = append(warnings, nestedWarnings...)
					cursor = close + 1
					continue
				}
			}
		case "sh":
			value, end, ok := parseStepArgument(source, mask, after)
			if ok {
				command := normalizeJenkinsShell(value)
				if name, found := returnStatusAssignment(source, index, end); found {
					command = "set +e\n" + command + "\n" + name + "=$?\nset -e"
				}
				appendCommand(&lines, command)
				cursor = end
				continue
			}
		case "error":
			value, end, ok := parseStepArgument(source, mask, after)
			if ok {
				appendCommand(&lines, `printf '%s\n' "`+shellDoubleQuote(normalizeJenkinsShell(value))+`" >&2`+"\nexit 1")
				cursor = end
				continue
			}
		case "echo":
			value, end, ok := parseStepArgument(source, mask, after)
			if ok {
				appendCommand(&lines, `printf '%s\n' "`+shellDoubleQuote(normalizeJenkinsShell(value))+`"`)
				cursor = end
				continue
			}
		case "dir":
			openParen := skipSpace(mask, after, len(mask))
			if openParen < len(mask) && mask[openParen] == '(' {
				closeParen, ok := matchingDelimiter(mask, openParen, '(', ')', len(mask))
				if ok {
					path, pathOK := shellPathExpression(strings.TrimSpace(source[openParen+1 : closeParen]))
					openBody := skipSpace(mask, closeParen+1, len(mask))
					if pathOK && openBody < len(mask) && mask[openBody] == '{' {
						closeBody, bodyOK := matchingDelimiter(mask, openBody, '{', '}', len(mask))
						if bodyOK {
							command, nestedWarnings := translateJenkinsShell(source[openBody+1 : closeBody])
							warnings = append(warnings, nestedWarnings...)
							if strings.TrimSpace(command) != "" {
								appendCommand(&lines, "(\n  cd "+path+"\n"+indentShell(command, "  ")+"\n)")
							}
							cursor = closeBody + 1
							continue
						}
					}
				}
			}
			warnings = appendWarning(warnings, "unsupported_dir", "a dir(...) block could not be converted automatically.")
		case "if":
			openParen := skipSpace(mask, after, len(mask))
			if openParen < len(mask) && mask[openParen] == '(' {
				closeParen, conditionOK := matchingDelimiter(mask, openParen, '(', ')', len(mask))
				openBody := skipSpace(mask, closeParen+1, len(mask))
				if conditionOK && openBody < len(mask) && mask[openBody] == '{' {
					closeBody, bodyOK := matchingDelimiter(mask, openBody, '{', '}', len(mask))
					condition, shellOK := shellFileCondition(source[openParen+1 : closeParen])
					if bodyOK && shellOK {
						thenCommand, thenWarnings := translateJenkinsShell(source[openBody+1 : closeBody])
						warnings = append(warnings, thenWarnings...)
						next := skipSpace(mask, closeBody+1, len(mask))
						elseCommand := ""
						end := closeBody + 1
						if elseIndex := findToken(mask, "else", next, len(mask)); elseIndex == next {
							elseBody := skipSpace(mask, elseIndex+len("else"), len(mask))
							if elseBody < len(mask) && mask[elseBody] == '{' {
								elseClose, elseOK := matchingDelimiter(mask, elseBody, '{', '}', len(mask))
								if elseOK {
									elseCommand, thenWarnings = translateJenkinsShell(source[elseBody+1 : elseClose])
									warnings = append(warnings, thenWarnings...)
									end = elseClose + 1
								}
							}
						}
						block := "if " + condition + "; then\n" + indentShell(thenCommand, "  ")
						if strings.TrimSpace(elseCommand) != "" {
							block += "\nelse\n" + indentShell(elseCommand, "  ")
						}
						block += "\nfi"
						appendCommand(&lines, block)
						cursor = end
						continue
					}
				}
			}
			warnings = appendWarning(warnings, "unsupported_condition", "a Groovy condition could not be converted; move it into an sh block and review the result.")
		}
		cursor = after
	}
	return strings.Join(lines, "\n\n"), warnings
}

func expandFileExistsVariables(source string) string {
	for _, match := range fileExistsAssignment.FindAllStringSubmatch(source, -1) {
		if len(match) != 3 {
			continue
		}
		condition := regexp.MustCompile(`\(\s*(!?)\s*` + regexp.QuoteMeta(match[1]) + `\s*\)`)
		source = condition.ReplaceAllString(source, "(${1}fileExists("+strings.TrimSpace(match[2])+"))")
	}
	return source
}

func returnStatusAssignment(source string, index, end int) (string, bool) {
	if !strings.Contains(source[index:end], "returnStatus: true") {
		return "", false
	}
	lineStart := strings.LastIndex(source[:index], "\n") + 1
	match := statusAssignment.FindStringSubmatch(source[lineStart:index])
	return func() string {
		if len(match) == 2 {
			return match[1]
		}
		return ""
	}(), len(match) == 2
}

func parseStepArgument(source, mask string, position int) (string, int, bool) {
	cursor := skipSourceSpace(source, position, len(source))
	if cursor < len(mask) && mask[cursor] == '(' {
		close, ok := matchingDelimiter(mask, cursor, '(', ')', len(mask))
		if !ok {
			return "", position, false
		}
		argumentStart := skipSourceSpace(source, cursor+1, close)
		if scriptIndex := findToken(mask, "script", argumentStart, close); scriptIndex == argumentStart {
			colon := skipSpace(mask, scriptIndex+len("script"), close)
			if colon < close && mask[colon] == ':' {
				argumentStart = skipSourceSpace(source, colon+1, close)
			}
		}
		value, _, ok := parseGroovyString(source, argumentStart)
		return value, close + 1, ok
	}
	value, end, ok := parseGroovyString(source, cursor)
	return value, end, ok
}

func parseGroovyString(source string, position int) (string, int, bool) {
	if position < 0 || position >= len(source) || (source[position] != '\'' && source[position] != '"') {
		return "", position, false
	}
	quote := source[position]
	triple := position+2 < len(source) && source[position+1] == quote && source[position+2] == quote
	if triple {
		delimiter := strings.Repeat(string(quote), 3)
		endOffset := strings.Index(source[position+3:], delimiter)
		if endOffset < 0 {
			return "", position, false
		}
		value := source[position+3 : position+3+endOffset]
		if quote == '"' {
			value = strings.ReplaceAll(value, `\$`, "$")
		}
		return value, position + 3 + endOffset + 3, true
	}
	for cursor := position + 1; cursor < len(source); cursor++ {
		if source[cursor] == '\\' {
			cursor++
			continue
		}
		if source[cursor] == quote {
			value := source[position+1 : cursor]
			value = strings.ReplaceAll(value, `\$`, "$")
			value = strings.ReplaceAll(value, `\`+string(quote), string(quote))
			return value, cursor + 1, true
		}
	}
	return "", position, false
}

func normalizeJenkinsShell(command string) string {
	command = strings.ReplaceAll(command, `\$`, "$")
	command = envReference.ReplaceAllString(command, `${$1}`)
	command = paramsReference.ReplaceAllString(command, `${$1}`)
	lines := strings.Split(strings.Trim(command, "\n"), "\n")
	minimum := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if minimum < 0 || indent < minimum {
			minimum = indent
		}
	}
	if minimum > 0 {
		for index := range lines {
			if len(lines[index]) >= minimum {
				lines[index] = lines[index][minimum:]
			}
		}
	}
	command = normalizeMacOSFeishuHelperPath(strings.TrimSpace(strings.Join(lines, "\n")))
	command = normalizeLongRunningJenkinsProcess(command)
	return normalizeJenkinsGitSync(command)
}

// A protected Desktop/Documents/Downloads executable can stall the macOS
// dynamic loader before main starts, even with a safe cwd. Never execute that
// path from the migrated pipeline. Use the operator-installed private helper
// path, fail clearly when it is absent, and preserve the original arguments.
func normalizeMacOSFeishuHelperPath(command string) string {
	return macOSFeishuHelper.ReplaceAllStringFunc(command, func(block string) string {
		match := macOSFeishuHelper.FindStringSubmatch(block)
		if len(match) != 5 {
			return block
		}
		indent := match[1]
		helper := `"` + macOSFeishuHelperPath + `"`
		return indent + `if [ ! -x ` + helper + ` ]; then` + "\n" +
			indent + `  printf '%s\n' 'BuildWorld: install the trusted feishu-robot helper at $HOME/Library/Application Support/buildworld/helpers/feishu-robot with mode 0700 before enabling this pipeline.' >&2` + "\n" +
			indent + `  exit 1` + "\n" +
			indent + `fi` + "\n" +
			indent + `cd "${TMPDIR:-/tmp}"` + "\n" +
			indent + helper + match[4]
	})
}

// Jenkins credentials are encrypted with that controller's master key and
// cannot be copied into a BuildWorld pipeline. On the same host, retain the
// checked-out revision when an authenticated fetch/pull is unavailable; a
// fresh clone still fails loudly rather than claiming a deployment succeeded.
func normalizeJenkinsGitSync(command string) string {
	command = jenkinsGitFetch.ReplaceAllString(command, `${1}echo "BuildWorld: starting non-interactive git fetch (60s HTTP idle timeout)"
${1}`+jenkinsGitSafetyEnv+` ${2} || echo "WARNING: BuildWorld: git fetch unavailable; using stale existing checkout"`)
	command = jenkinsGitPull.ReplaceAllString(command, `${1}echo "BuildWorld: starting non-interactive git pull (60s HTTP idle timeout)"
${1}`+jenkinsGitSafetyEnv+` ${2} || echo "WARNING: BuildWorld: git pull unavailable; using stale existing checkout"`)
	command = jenkinsGitResetRemote.ReplaceAllString(command, `${1} || git reset --hard HEAD`)
	return normalizeJenkinsWorkspaceClone(command)
}

// Pipeline script from SCM already creates a fresh Git checkout before the
// migrated Jenkinsfile executes. Older jobs sometimes repeat that work with
// `git clone ... .`, which fails because the workspace is no longer empty.
// Preserve inline-pipeline behavior while making that legacy SCM form
// idempotent.
func normalizeJenkinsWorkspaceClone(command string) string {
	lines := strings.Split(command, "\n")
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !isJenkinsWorkspaceCloneCommand(trimmed) {
			normalized = append(normalized, line)
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		normalized = append(normalized,
			indent+`if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then`,
			indent+`  echo "BuildWorld: workspace already contains the SCM checkout; skipping legacy git clone into ."`,
			indent+`else`,
			indent+`  `+trimmed,
			indent+`fi`,
		)
	}
	return strings.Join(normalized, "\n")
}

func isJenkinsWorkspaceCloneCommand(command string) bool {
	if !strings.HasPrefix(command, "git clone ") {
		return false
	}
	return strings.HasSuffix(command, " .") || strings.HasSuffix(command, " \".\"") || strings.HasSuffix(command, " '.'")
}

// Jenkins deployment jobs commonly reserve their executor forever by tailing a
// service log, or by running the service in the foreground through tee. In
// BuildWorld the build must finish so its result, artifacts, and lifecycle
// hooks are observable. Keep the service alive in the target workspace and
// leave log observation to BuildWorld's build log rather than an infinite loop.
func normalizeLongRunningJenkinsProcess(command string) string {
	command = persistentTailMonitor.ReplaceAllString(command, "\n# BuildWorld manages service log observation; the Jenkins infinite tail monitor was removed.")
	if monitor := strings.Index(command, "tail -f"); monitor >= 0 {
		if loop := strings.Index(command[monitor:], "while true; do"); loop >= 0 {
			start := strings.LastIndex(command[:monitor], "\n")
			if start < 0 {
				start = 0
			}
			command = command[:start] + "\n# BuildWorld manages service log observation; the Jenkins infinite tail monitor was removed."
		}
	}
	command = foregroundService.ReplaceAllString(command, `${1}nohup ${2} > ${3} 2>&1 &
${1}echo $! > buildworld-service.pid
${1}echo "BuildWorld started background service PID $(cat buildworld-service.pid)"`)
	command = teamResourcesFallback.ReplaceAllString(command, `if [ -f ./update-team-resources.sh ]; then$1
elif [ -f ./_scripts/deploy/update-team-resources.sh ]; then
  chmod +x ./_scripts/deploy/update-team-resources.sh
  ./_scripts/deploy/update-team-resources.sh
else$2
fi`)
	command = teamResourcesDirect.ReplaceAllString(command, `${1}if [ -f ./update-team-resources.sh ]; then
${1}  chmod +x ./update-team-resources.sh
${1}  ./update-team-resources.sh
${1}elif [ -f ./_scripts/deploy/update-team-resources.sh ]; then
${1}  chmod +x ./_scripts/deploy/update-team-resources.sh
${1}  ./_scripts/deploy/update-team-resources.sh
${1}else
${1}  echo "⚠️ 未找到 Team-Resources 更新脚本，跳过"
${1}fi`)
	return command
}

// Jenkins scripted blocks frequently construct a command in a simple GString
// and then call `sh command`. Resolve that safe, local form before extracting
// shell steps; expressions remain untouched and therefore visible for review.
func expandGroovyShellVariables(source string) string {
	values := make(map[string]string)
	for _, match := range groovyAssignment.FindAllStringSubmatchIndex(source, -1) {
		if len(match) < 4 {
			continue
		}
		value, _, ok := parseGroovyString(source, skipSourceSpace(source, match[1], len(source)))
		if ok {
			values[source[match[2]:match[3]]] = normalizeJenkinsShell(value)
		}
	}
	for iteration := 0; iteration < len(values); iteration++ {
		for name, value := range values {
			values[name] = groovyVariableRef.ReplaceAllStringFunc(value, func(reference string) string {
				key := strings.TrimSuffix(strings.TrimPrefix(reference, "${"), "}")
				if replacement, found := values[key]; found {
					return replacement
				}
				return reference
			})
		}
	}
	return bareShellVariable.ReplaceAllStringFunc(source, func(line string) string {
		parts := bareShellVariable.FindStringSubmatch(line)
		if len(parts) != 3 {
			return line
		}
		if value, found := values[parts[2]]; found {
			return parts[1] + strconv.Quote(value)
		}
		return line
	})
}

func shellFileCondition(source string) (string, bool) {
	source = strings.TrimSpace(source)
	if match := fileExistsCondition.FindStringSubmatch(source); len(match) == 3 {
		path, ok := shellPathExpression(strings.TrimSpace(match[2]))
		if !ok {
			return "", false
		}
		operator := "-e"
		if match[1] == "!" {
			operator = "! -e"
		}
		return `[ ` + operator + ` ` + path + ` ]`, true
	}
	if source == "isUnix()" {
		return `[ -n "$(uname 2>/dev/null || true)" ]`, true
	}
	if match := statusCondition.FindStringSubmatch(source); len(match) == 4 {
		return `[ "${` + match[1] + `}" ` + match[2] + ` "` + match[3] + `" ]`, true
	}
	return "", false
}

func shellPathExpression(source string) (string, bool) {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "env.") && isIdentifier(source[4:]) {
		return `"${` + source[4:] + `}"`, true
	}
	if isIdentifier(source) {
		return `"${` + source + `}"`, true
	}
	if value, _, ok := parseGroovyString(source, 0); ok {
		return strconv.Quote(normalizeJenkinsShell(value)), true
	}
	return "", false
}

func groovyCodeMask(source string) string {
	mask := []byte(source)
	for index := range mask {
		if mask[index] != '\n' && mask[index] != '\r' {
			mask[index] = ' '
		}
	}
	for cursor := 0; cursor < len(source); {
		if strings.HasPrefix(source[cursor:], "//") {
			end := strings.IndexByte(source[cursor:], '\n')
			if end < 0 {
				break
			}
			cursor += end
			continue
		}
		if strings.HasPrefix(source[cursor:], "/*") {
			end := strings.Index(source[cursor+2:], "*/")
			if end < 0 {
				break
			}
			cursor += end + 4
			continue
		}
		if source[cursor] == '\'' || source[cursor] == '"' {
			_, end, ok := parseGroovyString(source, cursor)
			if !ok {
				break
			}
			cursor = end
			continue
		}
		mask[cursor] = source[cursor]
		cursor++
	}
	return string(mask)
}

func matchingDelimiter(mask string, open int, left, right byte, limit int) (int, bool) {
	depth := 0
	for cursor := open; cursor < limit; cursor++ {
		switch mask[cursor] {
		case left:
			depth++
		case right:
			depth--
			if depth == 0 {
				return cursor, true
			}
		}
	}
	return 0, false
}

func findToken(mask, token string, from, limit int) int {
	for position := from; position < limit; {
		offset := strings.Index(mask[position:limit], token)
		if offset < 0 {
			return -1
		}
		index := position + offset
		beforeOK := index == 0 || !isIdentifierByte(mask[index-1])
		after := index + len(token)
		afterOK := after >= len(mask) || !isIdentifierByte(mask[after])
		if beforeOK && afterOK {
			return index
		}
		position = index + len(token)
	}
	return -1
}

func nextIdentifier(mask string, from int) (int, string) {
	for cursor := from; cursor < len(mask); cursor++ {
		if !isIdentifierByte(mask[cursor]) || (mask[cursor] >= '0' && mask[cursor] <= '9') {
			continue
		}
		end := cursor + 1
		for end < len(mask) && isIdentifierByte(mask[end]) {
			end++
		}
		return cursor, mask[cursor:end]
	}
	return -1, ""
}

func skipSpace(source string, position, limit int) int {
	for position < limit && unicode.IsSpace(rune(source[position])) {
		position++
	}
	return position
}

func skipSourceSpace(source string, position, limit int) int {
	for position < limit {
		switch source[position] {
		case ' ', '\t', '\r', '\n':
			position++
		default:
			return position
		}
	}
	return position
}

func isIdentifierByte(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func isIdentifier(value string) bool {
	if value == "" || value[0] >= '0' && value[0] <= '9' {
		return false
	}
	for index := range value {
		if !isIdentifierByte(value[index]) {
			return false
		}
	}
	return true
}

func appendCommand(lines *[]string, command string) {
	if command = strings.TrimSpace(command); command != "" {
		*lines = append(*lines, command)
	}
}

func indentShell(command, prefix string) string {
	if strings.TrimSpace(command) == "" {
		return prefix + ":"
	}
	return prefix + strings.ReplaceAll(strings.TrimSpace(command), "\n", "\n"+prefix)
}

func shellDoubleQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "`", "\\`")
	return value
}

func appendWarning(warnings []Warning, code, message string) []Warning {
	for _, warning := range warnings {
		if warning.Code == code && warning.Message == message {
			return warnings
		}
	}
	return append(warnings, Warning{Code: code, Message: message})
}

func appendMacOSProtectedDirectoryWarning(warnings []Warning, config *engine.BuildConfig) []Warning {
	if !buildConfigReferencesMacOSProtectedDirectory(config) {
		return warnings
	}
	return appendWarning(warnings, "macos_protected_directory", "This pipeline references a macOS protected directory. Jenkins privacy access is not inherited by BuildWorld; move the referenced workspace, file, script, or executable outside Desktop, Documents, and Downloads, or explicitly authorize the trusted installed server before cutover.")
}

func appendMacOSFeishuHelperInstallWarning(warnings []Warning, source string) []Warning {
	if !macOSFeishuHelper.MatchString(source) {
		return warnings
	}
	return appendWarning(warnings, "macos_feishu_helper_install_required", macOSFeishuHelperWarning)
}

func buildConfigReferencesMacOSProtectedDirectory(config *engine.BuildConfig) bool {
	if config == nil {
		return false
	}
	if directStringMapReferencesMacOSProtectedDirectory(config.Environment) || directStringsReferenceMacOSProtectedDirectory(config.Artifacts) {
		return true
	}
	for _, parameter := range config.Parameters {
		if value, ok := parameter.Default.(string); ok && directValueReferencesMacOSProtectedDirectory(value) {
			return true
		}
		if directStringsReferenceMacOSProtectedDirectory(parameter.Choices) {
			return true
		}
	}
	for _, stage := range config.Stages {
		if directValueReferencesMacOSProtectedDirectory(stage.WorkingDirectory) || directStringMapReferencesMacOSProtectedDirectory(stage.Environment) || stepsReferenceMacOSProtectedDirectory(stage.Steps) {
			return true
		}
	}
	for _, steps := range config.Post {
		if stepsReferenceMacOSProtectedDirectory(steps) {
			return true
		}
	}
	for _, tools := range config.Toolchains {
		if directStringsReferenceMacOSProtectedDirectory(tools) {
			return true
		}
	}
	return false
}

func stepsReferenceMacOSProtectedDirectory(steps []engine.Step) bool {
	for _, step := range steps {
		if commandReferencesMacOSProtectedDirectory(step.Command) ||
			directValueReferencesMacOSProtectedDirectory(step.Shell) ||
			directStringMapReferencesMacOSProtectedDirectory(step.Config) ||
			commandStringMapReferencesMacOSProtectedDirectory(step.PlatformAdditions) {
			return true
		}
	}
	return false
}

func directStringMapReferencesMacOSProtectedDirectory(values map[string]string) bool {
	for _, value := range values {
		if directValueReferencesMacOSProtectedDirectory(value) {
			return true
		}
	}
	return false
}

func commandStringMapReferencesMacOSProtectedDirectory(values map[string]string) bool {
	for _, value := range values {
		if commandReferencesMacOSProtectedDirectory(value) {
			return true
		}
	}
	return false
}

func directStringsReferenceMacOSProtectedDirectory(values []string) bool {
	for _, value := range values {
		if directValueReferencesMacOSProtectedDirectory(value) {
			return true
		}
	}
	return false
}

func commandReferencesMacOSProtectedDirectory(value string) bool {
	for _, token := range shellReferenceTokens(value) {
		if shellTokenReferencesMacOSProtectedDirectory(token) {
			return true
		}
	}
	return false
}

func directValueReferencesMacOSProtectedDirectory(value string) bool {
	for _, candidate := range strings.Split(value, ":") {
		candidate = strings.TrimSpace(candidate)
		if len(candidate) >= 2 && ((candidate[0] == '"' && candidate[len(candidate)-1] == '"') || (candidate[0] == '\'' && candidate[len(candidate)-1] == '\'')) {
			candidate = candidate[1 : len(candidate)-1]
		}
		if shellTokenReferencesMacOSProtectedDirectory(candidate) {
			return true
		}
	}
	return false
}

func shellTokenReferencesMacOSProtectedDirectory(token string) bool {
	candidate := token
	if index := strings.LastIndexByte(candidate, '='); index >= 0 {
		candidate = candidate[index+1:]
	}
	if candidate == "" || strings.Contains(candidate, "://") {
		return false
	}

	var remainder string
	switch {
	case strings.HasPrefix(candidate, "/Users/"):
		remainder = strings.TrimPrefix(candidate, "/Users/")
		separator := strings.IndexByte(remainder, '/')
		if separator <= 0 {
			return false
		}
		remainder = remainder[separator+1:]
	case strings.HasPrefix(candidate, "~/"):
		remainder = strings.TrimPrefix(candidate, "~/")
	case strings.HasPrefix(candidate, "$HOME/"):
		remainder = strings.TrimPrefix(candidate, "$HOME/")
	case strings.HasPrefix(candidate, "${HOME}/"):
		remainder = strings.TrimPrefix(candidate, "${HOME}/")
	case strings.HasPrefix(candidate, "${env.HOME}/"):
		remainder = strings.TrimPrefix(candidate, "${env.HOME}/")
	default:
		return false
	}

	folder := remainder
	if separator := strings.IndexByte(folder, '/'); separator >= 0 {
		folder = folder[:separator]
	}
	return strings.EqualFold(folder, "Desktop") || strings.EqualFold(folder, "Documents") || strings.EqualFold(folder, "Downloads")
}

func shellReferenceTokens(value string) []string {
	tokens := make([]string, 0)
	var token strings.Builder
	var quote byte
	escaped := false
	comment := false
	flush := func() {
		if token.Len() > 0 {
			tokens = append(tokens, token.String())
			token.Reset()
		}
	}

	for index := 0; index < len(value); index++ {
		character := value[index]
		if comment {
			if character == '\n' {
				comment = false
			}
			continue
		}
		if escaped {
			token.WriteByte(character)
			escaped = false
			continue
		}
		if quote != 0 {
			if character == quote {
				quote = 0
			} else if character == '\\' && quote == '"' {
				escaped = true
			} else {
				token.WriteByte(character)
			}
			continue
		}

		switch {
		case character == '\\':
			escaped = true
		case character == '\'' || character == '"':
			quote = character
		case character == '#' && token.Len() == 0:
			comment = true
		case unicode.IsSpace(rune(character)) || strings.ContainsRune(";|&()<>\n", rune(character)):
			flush()
		default:
			token.WriteByte(character)
		}
	}
	if escaped {
		token.WriteByte('\\')
	}
	flush()
	return tokens
}

package migration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/neko233-com/buildworld/internal/engine"
)

const maxJenkinsfileBytes = 2 << 20

var (
	environmentAssignment = regexp.MustCompile(`(?m)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:"((?:\\.|[^"\\])*)"|'((?:\\.|[^'\\])*)')\s*$`)
	envReference          = regexp.MustCompile(`\$\{env\.([A-Za-z_][A-Za-z0-9_]*)\}`)
	fileExistsCondition   = regexp.MustCompile(`^(!?)\s*fileExists\s*\(\s*(?:env\.)?([A-Za-z_][A-Za-z0-9_]*)\s*\)$`)
	originBranch          = regexp.MustCompile(`\borigin/([A-Za-z0-9._/-]+)`)
)

type JenkinsfileStrategy struct{}

func NewJenkinsfileStrategy() *JenkinsfileStrategy { return &JenkinsfileStrategy{} }

func (*JenkinsfileStrategy) SourceFormat() string { return "jenkinsfile" }

func (*JenkinsfileStrategy) Convert(request Request) (*Result, error) {
	source := strings.TrimPrefix(strings.ReplaceAll(request.Source, "\r\n", "\n"), "\ufeff")
	if strings.TrimSpace(source) == "" {
		return nil, fmt.Errorf("Jenkinsfile cannot be empty")
	}
	if len(source) > maxJenkinsfileBytes {
		return nil, fmt.Errorf("Jenkinsfile exceeds the 2 MiB migration limit")
	}
	mask := groovyCodeMask(source)
	pipeline, ok := namedBlock(source, mask, "pipeline", 0, len(source))
	if !ok {
		return nil, fmt.Errorf("declarative Jenkinsfile must contain pipeline { ... }")
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
	parseJenkinsAgent(pipelineSource, pipelineMask, config, &warnings)

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

	if _, found := namedBlock(pipelineSource, pipelineMask, "post", 0, len(pipelineSource)); found {
		warnings = appendWarning(warnings, "post_review_required", "Jenkins post conditions were detected. Configure equivalent Buildworld notifications or cleanup steps after import.")
	}
	if _, found := namedBlock(pipelineSource, pipelineMask, "options", 0, len(pipelineSource)); found {
		warnings = appendWarning(warnings, "options_review_required", "Jenkins options were detected. Review concurrency and timeout policies in Buildworld settings.")
	}

	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(config); err != nil {
		return nil, fmt.Errorf("encode migrated pipeline: %w", err)
	}
	if _, err := engine.ParsePipelineConfig(encoded.String()); err != nil {
		return nil, fmt.Errorf("validate migrated pipeline: %w", err)
	}

	hints := Hints{RepositoryURL: config.Environment["GIT_REPO_URL"], DefaultBranch: "main"}
	if match := originBranch.FindStringSubmatch(source); len(match) == 2 {
		hints.DefaultBranch = match[1]
	}
	return &Result{
		Version:      ResultVersion,
		SourceFormat: "jenkinsfile",
		TargetFormat: "buildworld-json",
		Config:       encoded.String(),
		Warnings:     warnings,
		Summary:      Summary{StageCount: len(config.Stages), EnvironmentCount: len(config.Environment)},
		Hints:        hints,
	}, nil
}

type sourceRange struct {
	start int
	end   int
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
		stepsBlock, found := namedBlock(body, bodyMask, "steps", 0, len(body))
		if !found {
			*warnings = appendWarning(*warnings, "stage_without_steps", fmt.Sprintf("Stage %q has no declarative steps block and was skipped.", stageName))
			position = closeBody + 1
			continue
		}
		stepSource := body[stepsBlock.start:stepsBlock.end]
		command, commandWarnings := translateJenkinsShell(stepSource)
		for _, warning := range commandWarnings {
			*warnings = appendWarning(*warnings, warning.Code, fmt.Sprintf("Stage %q: %s", stageName, warning.Message))
		}
		if strings.TrimSpace(command) == "" {
			*warnings = appendWarning(*warnings, "stage_without_supported_commands", fmt.Sprintf("Stage %q contains no supported sh/echo/script/dir commands and was skipped.", stageName))
			position = closeBody + 1
			continue
		}
		stages = append(stages, engine.Stage{
			Name: strings.TrimSpace(stageName),
			Steps: []engine.Step{{
				Name:    strings.TrimSpace(stageName),
				Type:    "shell",
				Command: "set -e\n\n" + strings.TrimSpace(command),
			}},
		})
		position = closeBody + 1
	}
	if len(stages) == 0 {
		return nil, fmt.Errorf("Jenkinsfile has no stages with supported executable commands")
	}
	return stages, nil
}

func translateJenkinsShell(source string) (string, []Warning) {
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
				appendCommand(&lines, normalizeJenkinsShell(value))
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
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func shellFileCondition(source string) (string, bool) {
	match := fileExistsCondition.FindStringSubmatch(strings.TrimSpace(source))
	if len(match) != 3 {
		return "", false
	}
	operator := "-e"
	if match[1] == "!" {
		operator = "! -e"
	}
	return `[ ` + operator + ` "${` + match[2] + `}" ]`, true
}

func shellPathExpression(source string) (string, bool) {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "env.") && isIdentifier(source[4:]) {
		return `"${` + source[4:] + `}"`, true
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

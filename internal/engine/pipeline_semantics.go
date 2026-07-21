package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const (
	StageStatusSuccess   = "success"
	StageStatusFailed    = "failed"
	StageStatusSkipped   = "skipped"
	StageStatusCancelled = "cancelled"
)

// ValidatePipelineSemantics applies format-independent validation and derives
// execution behavior after YAML or TypeScript parsing. A native service
// watcher disables the default timeout only when it is an unconditional final
// step in the final stage. Explicit pipeline, stage, or queued-build timeouts
// still win in the runner.
func ValidatePipelineSemantics(config *BuildConfig) error {
	if config == nil {
		return fmt.Errorf("pipeline config is required")
	}
	orderedStages, err := orderPipelineStages(config.Stages)
	if err != nil {
		return err
	}
	config.Stages = orderedStages

	automaticLongRunning := false
	for stageIndex := range config.Stages {
		stage := &config.Stages[stageIndex]
		if err := ValidatePipelineCondition(stage.If); err != nil {
			return fmt.Errorf("stage %q if: %w", stage.Name, err)
		}
		for stepIndex := range stage.Steps {
			step := &stage.Steps[stepIndex]
			if err := ValidatePipelineCondition(step.If); err != nil {
				return fmt.Errorf("stage %q step %q if: %w", stage.Name, step.Name, err)
			}
			if step.Type != "service_watch" {
				continue
			}
			if err := ValidateServiceWatchConfig(step.Config); err != nil {
				return fmt.Errorf("stage %q step %q: %w", stage.Name, step.Name, err)
			}
			if stageIndex != len(config.Stages)-1 {
				return fmt.Errorf("stage %q step %q: service_watch must be in the terminal stage", stage.Name, step.Name)
			}
			if stepIndex != len(stage.Steps)-1 {
				return fmt.Errorf("stage %q step %q: service_watch must be the terminal step", stage.Name, step.Name)
			}
			// Branch and if filters can skip a watcher. Keep the finite default
			// timeout in that case so some earlier command cannot run forever
			// merely because an observer happened to be present in the source.
			if strings.TrimSpace(stage.If) == "" &&
				len(stage.Branches) == 0 &&
				strings.TrimSpace(step.If) == "" {
				automaticLongRunning = true
			}
		}
	}
	for condition, steps := range config.Post {
		for _, step := range steps {
			if err := ValidatePipelineCondition(step.If); err != nil {
				return fmt.Errorf("post %q step %q if: %w", condition, step.Name, err)
			}
			if step.Type == "service_watch" {
				return fmt.Errorf("post %q step %q: service_watch is not allowed in post steps", condition, step.Name)
			}
		}
	}
	// Treat this as validated derived state. A source-level flag alone must not
	// suppress the safety timeout when its watcher is conditional or misplaced.
	config.AllowLongRunning = automaticLongRunning
	return nil
}

// orderPipelineStages validates the complete dependency graph and returns a
// stable topological order. All source formats pass through this function
// before persistence, so forward references are safe while unknown, duplicate,
// and cyclic dependencies fail before a build can be queued.
func orderPipelineStages(stages []Stage) ([]Stage, error) {
	if len(stages) == 0 {
		return stages, nil
	}

	indices := make(map[string]int, len(stages))
	names := make(map[string]int, len(stages))
	for index := range stages {
		name := strings.TrimSpace(stages[index].Name)
		if name == "" {
			return nil, fmt.Errorf("stage %d has an empty name", index+1)
		}
		if previous, exists := names[name]; exists {
			return nil, fmt.Errorf("stage name %q is declared more than once (stages %d and %d)", name, previous+1, index+1)
		}
		names[name] = index

		key := StageKey(stages[index])
		if key == "" {
			return nil, fmt.Errorf("stage %q has an empty dependency identifier", name)
		}
		if previous, exists := indices[key]; exists {
			return nil, fmt.Errorf("stage identifier %q is declared more than once (stages %d and %d)", key, previous+1, index+1)
		}
		indices[key] = index
	}

	indegree := make([]int, len(stages))
	dependents := make([][]int, len(stages))
	for index := range stages {
		seen := make(map[string]struct{}, len(stages[index].DependsOn))
		for dependencyIndex, rawDependency := range stages[index].DependsOn {
			dependency := strings.TrimSpace(rawDependency)
			if dependency == "" {
				return nil, fmt.Errorf("stage %q has an empty dependency", StageKey(stages[index]))
			}
			if _, duplicate := seen[dependency]; duplicate {
				return nil, fmt.Errorf("stage %q repeats dependency %q", StageKey(stages[index]), dependency)
			}
			seen[dependency] = struct{}{}
			resolvedIndex, exists := indices[dependency]
			if !exists {
				return nil, fmt.Errorf("stage %q depends on unknown stage %q", StageKey(stages[index]), dependency)
			}
			stages[index].DependsOn[dependencyIndex] = dependency
			indegree[index]++
			dependents[resolvedIndex] = append(dependents[resolvedIndex], index)
		}
	}

	processed := make([]bool, len(stages))
	ordered := make([]Stage, 0, len(stages))
	for len(ordered) < len(stages) {
		next := -1
		for index := range stages {
			if !processed[index] && indegree[index] == 0 {
				next = index
				break
			}
		}
		if next < 0 {
			cycle := make([]string, 0, len(stages)-len(ordered))
			for index := range stages {
				if !processed[index] {
					cycle = append(cycle, StageKey(stages[index]))
				}
			}
			return nil, fmt.Errorf("stages contain a dependency cycle: %s", strings.Join(cycle, " -> "))
		}
		processed[next] = true
		ordered = append(ordered, stages[next])
		for _, dependent := range dependents[next] {
			indegree[dependent]--
		}
	}
	return ordered, nil
}

// ConditionContext is the deliberately small expression context supported by
// declarative pipelines. It exposes status predicates without exposing a
// general-purpose expression runtime.
type ConditionContext struct {
	Success bool
	Failure bool
}

// ValidatePipelineCondition rejects unknown identifiers and syntax while the
// pipeline is parsed, before any build is queued.
func ValidatePipelineCondition(expression string) error {
	_, err := EvaluatePipelineCondition(expression, ConditionContext{Success: true})
	return err
}

// EvaluatePipelineCondition evaluates the safe GitHub Actions-style status
// subset: booleans, success(), failure(), always(), !, &&, || and parentheses.
func EvaluatePipelineCondition(expression string, state ConditionContext) (bool, error) {
	normalized, err := normalizePipelineCondition(expression)
	if err != nil {
		return false, err
	}
	parser := conditionParser{input: normalized, state: state}
	parser.next()
	value, err := parser.parseOr()
	if err != nil {
		return false, err
	}
	if parser.token.kind != conditionTokenEOF {
		return false, fmt.Errorf("condition %q: unexpected token %q", expression, parser.token.text)
	}
	return value, nil
}

func normalizePipelineCondition(expression string) (string, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return "success()", nil
	}
	hasPrefix := strings.HasPrefix(expression, "${{")
	hasSuffix := strings.HasSuffix(expression, "}}")
	if hasPrefix != hasSuffix {
		return "", fmt.Errorf("condition %q: unbalanced ${{ }} wrapper", expression)
	}
	if hasPrefix {
		expression = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(expression, "${{"), "}}"))
	}
	if expression == "" {
		return "", fmt.Errorf("condition cannot be empty")
	}
	return expression, nil
}

type conditionTokenKind int

const (
	conditionTokenInvalid conditionTokenKind = iota
	conditionTokenEOF
	conditionTokenIdentifier
	conditionTokenNot
	conditionTokenAnd
	conditionTokenOr
	conditionTokenLeftParen
	conditionTokenRightParen
)

type conditionToken struct {
	kind conditionTokenKind
	text string
}

type conditionParser struct {
	input string
	pos   int
	token conditionToken
	err   error
	state ConditionContext
}

func (p *conditionParser) next() {
	for p.pos < len(p.input) && unicode.IsSpace(rune(p.input[p.pos])) {
		p.pos++
	}
	if p.pos >= len(p.input) {
		p.token = conditionToken{kind: conditionTokenEOF}
		return
	}
	start := p.pos
	switch p.input[p.pos] {
	case '!':
		p.pos++
		p.token = conditionToken{kind: conditionTokenNot, text: "!"}
	case '(':
		p.pos++
		p.token = conditionToken{kind: conditionTokenLeftParen, text: "("}
	case ')':
		p.pos++
		p.token = conditionToken{kind: conditionTokenRightParen, text: ")"}
	case '&':
		if p.pos+1 < len(p.input) && p.input[p.pos+1] == '&' {
			p.pos += 2
			p.token = conditionToken{kind: conditionTokenAnd, text: "&&"}
			return
		}
		p.pos++
		p.token = conditionToken{kind: conditionTokenInvalid, text: p.input[start:p.pos]}
	case '|':
		if p.pos+1 < len(p.input) && p.input[p.pos+1] == '|' {
			p.pos += 2
			p.token = conditionToken{kind: conditionTokenOr, text: "||"}
			return
		}
		p.pos++
		p.token = conditionToken{kind: conditionTokenInvalid, text: p.input[start:p.pos]}
	default:
		for p.pos < len(p.input) {
			current := rune(p.input[p.pos])
			if !unicode.IsLetter(current) && !unicode.IsDigit(current) && current != '_' {
				break
			}
			p.pos++
		}
		if p.pos == start {
			p.pos++
			p.token = conditionToken{kind: conditionTokenInvalid, text: p.input[start:p.pos]}
			return
		}
		p.token = conditionToken{kind: conditionTokenIdentifier, text: p.input[start:p.pos]}
	}
}

func (p *conditionParser) parseOr() (bool, error) {
	left, err := p.parseAnd()
	if err != nil {
		return false, err
	}
	for p.token.kind == conditionTokenOr {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return false, err
		}
		left = left || right
	}
	return left, nil
}

func (p *conditionParser) parseAnd() (bool, error) {
	left, err := p.parseUnary()
	if err != nil {
		return false, err
	}
	for p.token.kind == conditionTokenAnd {
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return false, err
		}
		left = left && right
	}
	return left, nil
}

func (p *conditionParser) parseUnary() (bool, error) {
	if p.token.kind == conditionTokenNot {
		p.next()
		value, err := p.parseUnary()
		return !value, err
	}
	return p.parsePrimary()
}

func (p *conditionParser) parsePrimary() (bool, error) {
	switch p.token.kind {
	case conditionTokenLeftParen:
		p.next()
		value, err := p.parseOr()
		if err != nil {
			return false, err
		}
		if p.token.kind != conditionTokenRightParen {
			return false, fmt.Errorf("condition: expected closing parenthesis")
		}
		p.next()
		return value, nil
	case conditionTokenIdentifier:
		identifier := strings.ToLower(p.token.text)
		p.next()
		switch identifier {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "success", "failure", "always":
			if p.token.kind != conditionTokenLeftParen {
				return false, fmt.Errorf("condition: %s must be called as %s()", identifier, identifier)
			}
			p.next()
			if p.token.kind != conditionTokenRightParen {
				return false, fmt.Errorf("condition: %s() does not accept arguments", identifier)
			}
			p.next()
			switch identifier {
			case "success":
				return p.state.Success, nil
			case "failure":
				return p.state.Failure, nil
			default:
				return true, nil
			}
		default:
			return false, fmt.Errorf("condition: unsupported identifier %q", identifier)
		}
	default:
		return false, fmt.Errorf("condition: unexpected token %q", p.token.text)
	}
}

// StageKey returns the stable dependency identifier. YAML jobs use ID while
// TypeScript pipelines use the stage display name.
func StageKey(stage Stage) string {
	if strings.TrimSpace(stage.ID) != "" {
		return strings.TrimSpace(stage.ID)
	}
	return strings.TrimSpace(stage.Name)
}

// ShouldRunStage applies dependency status and the stage's safe condition.
func ShouldRunStage(stage Stage, statuses map[string]string) (bool, error) {
	state := ConditionContext{Success: true}
	for _, dependency := range stage.DependsOn {
		status, exists := statuses[dependency]
		if !exists {
			return false, fmt.Errorf("stage %q depends on unknown stage %q", StageKey(stage), dependency)
		}
		if status != StageStatusSuccess {
			state.Success = false
		}
		if status == StageStatusFailed || status == StageStatusCancelled {
			state.Failure = true
		}
	}
	return EvaluatePipelineCondition(stage.If, state)
}

// AppendPipelineEnvironment overlays configured variables deterministically.
func AppendPipelineEnvironment(base []string, configured map[string]string) []string {
	if len(configured) == 0 {
		return append([]string(nil), base...)
	}
	values := make(map[string]string, len(base)+len(configured))
	order := make([]string, 0, len(base)+len(configured))
	for _, entry := range base {
		key, value, found := strings.Cut(entry, "=")
		if !found || key == "" {
			continue
		}
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = value
	}
	keys := make([]string, 0, len(configured))
	for key := range configured {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		value := os.Expand(configured[key], func(reference string) string {
			if resolved, exists := values[reference]; exists {
				return resolved
			}
			return "${" + reference + "}"
		})
		values[key] = value
	}
	result := make([]string, 0, len(order))
	for _, key := range order {
		result = append(result, key+"="+values[key])
	}
	return result
}

// AppendStepEnvironment applies env.* entries from a step config.
func AppendStepEnvironment(base []string, config map[string]string) []string {
	configured := make(map[string]string)
	for key, value := range config {
		if strings.HasPrefix(key, "env.") && len(key) > len("env.") {
			configured[strings.TrimPrefix(key, "env.")] = value
		}
	}
	return AppendPipelineEnvironment(base, configured)
}

// ResolveWorkspaceDirectory resolves a job/step directory and rejects lexical
// and symlink escapes from the isolated build workspace.
func ResolveWorkspaceDirectory(workspace, configured string) (string, error) {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	candidate := root
	if strings.TrimSpace(configured) != "" {
		if filepath.IsAbs(configured) {
			candidate = filepath.Clean(configured)
		} else {
			candidate = filepath.Join(root, configured)
		}
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve working-directory: %w", err)
	}
	if !pathWithin(root, candidate) {
		return "", fmt.Errorf("working-directory %q escapes workspace", configured)
	}

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace links: %w", err)
	}
	existing := candidate
	for {
		if _, statErr := os.Lstat(existing); statErr == nil {
			break
		} else if !os.IsNotExist(statErr) {
			return "", fmt.Errorf("inspect working-directory: %w", statErr)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", fmt.Errorf("working-directory %q has no accessible parent", configured)
		}
		existing = parent
	}
	resolvedExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", fmt.Errorf("resolve working-directory links: %w", err)
	}
	if !pathWithin(resolvedRoot, resolvedExisting) {
		return "", fmt.Errorf("working-directory %q escapes workspace through a symlink", configured)
	}
	return candidate, nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative))
}

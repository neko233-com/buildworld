package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/dop251/goja"
)

// TypeScript pipeline sources deliberately use a small, declarative API. The
// server strips the type-only module boundary and evaluates only object
// construction in an otherwise empty Goja runtime; it never grants Node, file,
// network, process, or plugin access to a project configuration.
var (
	typeScriptPipelineImport = regexp.MustCompile(`(?m)^\s*import\s+(?:type\s+)?(?:\{[^}]*\}|\*\s+as\s+\w+)\s+from\s+['"]@buildworld/pipeline['"]\s*;?\s*$`)
	typeScriptImport         = regexp.MustCompile(`(?m)^\s*import\b`)
	typeScriptExportDefault  = regexp.MustCompile(`\bexport\s+default\s+`)
	typeScriptAssertion      = regexp.MustCompile(`\s+as\s+const\b|\s+satisfies\s+[A-Za-z_$][\w.$<>\[\]|& ,]*`)
	typeScriptUnsafeSyntax   = regexp.MustCompile(`=>|\b(?:while|for|function|class|new|eval|Function|require|process|globalThis|setTimeout|setInterval|Promise)\b`)
)

const typeScriptPipelinePrelude = `
function __bwCopy(value) {
  var result = {};
  Object.keys(value || {}).forEach(function(key) { result[key] = value[key]; });
  return result;
}
function definePipeline(value) {
  var pipeline = __bwCopy(value);
  if (pipeline.agentRequirements && !pipeline.agent_requirements) pipeline.agent_requirements = pipeline.agentRequirements;
  if (pipeline.retentionCompleted && !pipeline.retention_completed) pipeline.retention_completed = pipeline.retentionCompleted;
  if (pipeline.timeoutSec && !pipeline.timeout_sec) pipeline.timeout_sec = pipeline.timeoutSec;
	if (pipeline.allowLongRunning !== undefined && pipeline.allow_long_running === undefined) pipeline.allow_long_running = pipeline.allowLongRunning;
  if (pipeline.disableConcurrent !== undefined && pipeline.disable_concurrent === undefined) pipeline.disable_concurrent = pipeline.disableConcurrent;
  if (pipeline.abortPrevious !== undefined && pipeline.abort_previous === undefined) pipeline.abort_previous = pipeline.abortPrevious;
  if (pipeline.on && !pipeline.triggers) pipeline.triggers = pipeline.on;
  return pipeline;
}
function step(name, type, command, options) {
  var value = __bwCopy(options);
  value.name = name; value.type = type; value.command = command;
  if (value.platformAdditions && !value.platform_additions) value.platform_additions = value.platformAdditions;
  return value;
}
function shell(name, command, options) { return step(name, 'shell', command, options); }
function tail(name, command, options) { return step(name, 'tail', command, options); }
function script(name, command, options) { return step(name, 'script', command, options); }
function git(name, options) { return step(name, 'git', '', options); }
function notify(name, options) { return step(name, 'notify', '', options); }
function watchService(name, options) {
  var config = {};
  Object.keys(options || {}).forEach(function(key) {
    var value = options[key];
    if (value !== undefined && value !== null) config[key] = String(value);
  });
  return { name: name, type: 'service_watch', config: config };
}
function stage(name, steps, options) {
  var value = __bwCopy(options);
  value.name = name; value.steps = Array.isArray(steps) ? steps : [steps];
  if (value.dependsOn && !value.depends_on) value.depends_on = value.dependsOn;
  return value;
}
function trigger(type, config) { return { type: type, config: config || {} }; }
function parameter(name, type, options) {
  var value = __bwCopy(options);
  value.name = name; value.type = type;
  if (value.isSecret !== undefined && value.is_secret === undefined) value.is_secret = value.isSecret;
  return value;
}
var __buildworld_pipeline = null;
`

// IsTypeScriptPipeline identifies the supported @buildworld/pipeline source
// without confusing normal YAML that happens to contain a JavaScript word.
func IsTypeScriptPipeline(source string) bool {
	trimmed := strings.TrimSpace(source)
	return strings.Contains(trimmed, "@buildworld/pipeline") || strings.HasPrefix(trimmed, "// buildworld-pipeline: ts") || strings.HasPrefix(trimmed, "import ") || strings.Contains(trimmed, "export default definePipeline")
}

// ParseTypeScriptPipeline resolves a typed declarative source into the same
// BuildConfig used by JSON, YAML, and Markdown pipelines.
func ParseTypeScriptPipeline(source string) (*BuildConfig, error) {
	trimmed := strings.TrimSpace(source)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("TypeScript pipeline cannot be empty")
	}
	if len(trimmed) > 256*1024 {
		return nil, fmt.Errorf("TypeScript pipeline exceeds 256 KiB")
	}
	if typeScriptUnsafeSyntax.MatchString(typeScriptProgramWithoutStrings(trimmed)) {
		return nil, fmt.Errorf("TypeScript pipelines are declarative; loops, functions, and runtime APIs are not allowed")
	}
	program := typeScriptPipelineImport.ReplaceAllString(trimmed, "")
	if typeScriptImport.MatchString(program) {
		return nil, fmt.Errorf("TypeScript pipelines may only import @buildworld/pipeline")
	}
	program = typeScriptAssertion.ReplaceAllString(program, "")
	if !typeScriptExportDefault.MatchString(program) {
		return nil, fmt.Errorf("TypeScript pipeline must use export default definePipeline(...)")
	}
	program = typeScriptExportDefault.ReplaceAllString(program, "__buildworld_pipeline = ")

	runtime := goja.New()
	if _, err := runtime.RunString(typeScriptPipelinePrelude + "\n" + program); err != nil {
		return nil, fmt.Errorf("TypeScript pipeline: %w", err)
	}
	value := runtime.Get("__buildworld_pipeline")
	if value == nil || goja.IsNull(value) || goja.IsUndefined(value) {
		return nil, fmt.Errorf("TypeScript pipeline did not export a pipeline")
	}
	data, err := json.Marshal(value.Export())
	if err != nil {
		return nil, fmt.Errorf("TypeScript pipeline export: %w", err)
	}
	config, err := ParseBuildConfig(string(data))
	if err != nil {
		return nil, fmt.Errorf("TypeScript pipeline config: %w", err)
	}
	return config, nil
}

// FormatTypeScriptPipeline emits a deliberately plain TypeScript source for a
// BuildConfig. JSON is valid TypeScript object syntax, so this preserves every
// migrated field without manufacturing executable code or losing future
// BuildConfig fields. Authors can progressively replace literal steps with
// the typed helper calls from @buildworld/pipeline.
func FormatTypeScriptPipeline(config *BuildConfig) (string, error) {
	if config == nil {
		return "", fmt.Errorf("TypeScript pipeline config is required")
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(config); err != nil {
		return "", fmt.Errorf("encode TypeScript pipeline: %w", err)
	}
	return "import { definePipeline } from '@buildworld/pipeline'\n\nexport default definePipeline(" + strings.TrimSpace(encoded.String()) + ")\n", nil
}

// typeScriptProgramWithoutStrings keeps security checks scoped to the pipeline
// program rather than shell commands or explanatory comments inside literals.
func typeScriptProgramWithoutStrings(source string) string {
	var output strings.Builder
	output.Grow(len(source))
	for index := 0; index < len(source); {
		if source[index] == '/' && index+1 < len(source) && source[index+1] == '/' {
			for index < len(source) && source[index] != '\n' {
				output.WriteByte(' ')
				index++
			}
			continue
		}
		if source[index] == '/' && index+1 < len(source) && source[index+1] == '*' {
			output.WriteString("  ")
			index += 2
			for index < len(source) && !(source[index] == '*' && index+1 < len(source) && source[index+1] == '/') {
				if source[index] == '\n' {
					output.WriteByte('\n')
				} else {
					output.WriteByte(' ')
				}
				index++
			}
			if index+1 < len(source) {
				output.WriteString("  ")
				index += 2
			}
			continue
		}
		quote := source[index]
		if quote != '\'' && quote != '"' && quote != '`' {
			output.WriteByte(quote)
			index++
			continue
		}
		output.WriteByte(' ')
		index++
		for index < len(source) {
			current := source[index]
			if current == '\\' {
				output.WriteByte(' ')
				index++
				if index < len(source) {
					output.WriteByte(' ')
					index++
				}
				continue
			}
			if current == quote {
				output.WriteByte(' ')
				index++
				break
			}
			if current == '\n' {
				output.WriteByte('\n')
			} else {
				output.WriteByte(' ')
			}
			index++
		}
	}
	return output.String()
}

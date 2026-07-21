package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
)

const (
	typeScriptPipelineMaxSourceBytes = 256 * 1024
	typeScriptPipelineMaxOutputBytes = 512 * 1024
	typeScriptPipelineMaxNodes       = 10_000
	typeScriptPipelineMaxDepth       = 64
	typeScriptPipelineMaxSyntaxDepth = 128
)

var typeScriptPipelineCalls = map[string]struct{}{
	"definePipeline": {},
	"step":           {},
	"shell":          {},
	"tail":           {},
	"script":         {},
	"git":            {},
	"notify":         {},
	"watchService":   {},
	"stage":          {},
	"trigger":        {},
	"parameter":      {},
}

var typeScriptPipelineImports = map[string]struct{}{
	"definePipeline":      {},
	"step":                {},
	"shell":               {},
	"tail":                {},
	"script":              {},
	"git":                 {},
	"notify":              {},
	"watchService":        {},
	"stage":               {},
	"trigger":             {},
	"parameter":           {},
	"Pipeline":            {},
	"Step":                {},
	"StepType":            {},
	"ServiceWatchOptions": {},
	"Stage":               {},
	"Trigger":             {},
	"Parameter":           {},
	"ApprovalPolicy":      {},
	"PostCondition":       {},
	"ApprovalRole":        {},
}

var typeScriptPipelineForbiddenWords = map[string]struct{}{
	"async":       {},
	"await":       {},
	"catch":       {},
	"class":       {},
	"constructor": {},
	"debugger":    {},
	"delete":      {},
	"do":          {},
	"else":        {},
	"eval":        {},
	"finally":     {},
	"for":         {},
	"function":    {},
	"Function":    {},
	"globalThis":  {},
	"new":         {},
	"process":     {},
	"Promise":     {},
	"require":     {},
	"return":      {},
	"setInterval": {},
	"setTimeout":  {},
	"switch":      {},
	"throw":       {},
	"this":        {},
	"try":         {},
	"typeof":      {},
	"void":        {},
	"while":       {},
	"with":        {},
	"yield":       {},
}

// IsTypeScriptPipeline identifies the supported @buildworld/pipeline source
// without confusing normal YAML that happens to contain a JavaScript word.
func IsTypeScriptPipeline(source string) bool {
	trimmed := strings.TrimSpace(source)
	return strings.Contains(trimmed, "@buildworld/pipeline") ||
		strings.HasPrefix(trimmed, "// buildworld-pipeline: ts") ||
		strings.HasPrefix(trimmed, "import ") ||
		strings.Contains(trimmed, "export default definePipeline")
}

// ParseTypeScriptPipeline parses a deliberately small TypeScript-shaped DSL.
// User source is never compiled or executed. Module syntax is removed with a
// string-aware scanner, then a Goja AST is interpreted through a strict
// literal/call whitelist.
func ParseTypeScriptPipeline(source string) (*BuildConfig, error) {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return nil, fmt.Errorf("TypeScript pipeline cannot be empty")
	}
	if len(trimmed) > typeScriptPipelineMaxSourceBytes {
		return nil, fmt.Errorf("TypeScript pipeline exceeds 256 KiB")
	}

	expressionSource, valueImports, err := typeScriptPipelineExpression(trimmed)
	if err != nil {
		return nil, err
	}
	expressionSource, err = stripTypeScriptPipelineAssertions(expressionSource)
	if err != nil {
		return nil, fmt.Errorf("TypeScript pipeline syntax: %w", err)
	}
	if err := validateTypeScriptPipelineLexicalLimits(expressionSource); err != nil {
		return nil, err
	}

	program, err := parser.ParseFile(nil, "pipeline.ts", expressionSource, 0, parser.WithDisableSourceMaps)
	if err != nil {
		return nil, fmt.Errorf("TypeScript pipeline syntax: %w", err)
	}
	root, err := typeScriptPipelineRoot(program)
	if err != nil {
		return nil, err
	}

	evaluator := typeScriptPipelineEvaluator{valueImports: valueImports}
	value, err := evaluator.evaluate(root, 1)
	if err != nil {
		return nil, fmt.Errorf("TypeScript pipeline: %w", err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("TypeScript pipeline export: %w", err)
	}
	if len(data) > typeScriptPipelineMaxOutputBytes {
		return nil, fmt.Errorf("TypeScript pipeline output exceeds 512 KiB")
	}
	config, err := decodeTypeScriptPipelineData(string(data))
	if err != nil {
		return nil, fmt.Errorf("TypeScript pipeline config: %w", err)
	}
	return config, nil
}

func typeScriptPipelineRoot(program *ast.Program) (ast.Expression, error) {
	var root ast.Expression
	for _, statement := range program.Body {
		switch statement := statement.(type) {
		case *ast.EmptyStatement:
			continue
		case *ast.ExpressionStatement:
			if root != nil {
				return nil, fmt.Errorf("TypeScript pipelines are declarative; export default must contain exactly one expression")
			}
			root = statement.Expression
		default:
			return nil, fmt.Errorf("TypeScript pipelines are declarative; statements and declarations are not allowed")
		}
	}
	if root == nil {
		return nil, fmt.Errorf("TypeScript pipeline did not export a pipeline")
	}
	call, ok := root.(*ast.CallExpression)
	if !ok || typeScriptIdentifier(call.Callee) != "definePipeline" {
		return nil, fmt.Errorf("TypeScript pipeline must use export default definePipeline(...)")
	}
	return root, nil
}

// FormatTypeScriptPipeline emits a deliberately plain TypeScript source for a
// BuildConfig. JSON is valid TypeScript object syntax, so this preserves every
// migrated field without manufacturing executable code or losing future
// BuildConfig fields. Authors can progressively replace literal steps with
// typed helper calls from @buildworld/pipeline.
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

// typeScriptPipelineExpression validates and removes the TypeScript module
// shell. It only accepts named imports from @buildworld/pipeline and exactly
// one top-level export default.
func typeScriptPipelineExpression(source string) (string, map[string]struct{}, error) {
	exportStart, expressionStart, err := findTypeScriptExportDefault(source)
	if err != nil {
		return "", nil, err
	}
	valueImports, err := validateTypeScriptPipelineImports(source[:exportStart])
	if err != nil {
		return "", nil, err
	}
	return source[expressionStart:], valueImports, nil
}

func findTypeScriptExportDefault(source string) (int, int, error) {
	round, square, curly := 0, 0, 0
	for index := 0; index < len(source); {
		if next, ok, err := skipTypeScriptTrivia(source, index); err != nil {
			return 0, 0, err
		} else if ok {
			index = next
			continue
		}
		switch source[index] {
		case '\'', '"', '`':
			next, err := skipTypeScriptQuoted(source, index)
			if err != nil {
				return 0, 0, err
			}
			index = next
			continue
		case '(':
			round++
			index++
			continue
		case ')':
			round--
			index++
			continue
		case '[':
			square++
			index++
			continue
		case ']':
			square--
			index++
			continue
		case '{':
			curly++
			index++
			continue
		case '}':
			curly--
			index++
			continue
		}
		if round < 0 || square < 0 || curly < 0 {
			return 0, 0, fmt.Errorf("TypeScript pipeline syntax has unmatched delimiters")
		}
		if round == 0 && square == 0 && curly == 0 && hasTypeScriptKeyword(source, index, "export") {
			afterExport := index + len("export")
			afterExport, _, err := skipTypeScriptTrivia(source, afterExport)
			if err != nil {
				return 0, 0, err
			}
			if !hasTypeScriptKeyword(source, afterExport, "default") {
				return 0, 0, fmt.Errorf("TypeScript pipeline only supports export default")
			}
			return index, afterExport + len("default"), nil
		}
		if isTypeScriptIdentifierStart(source[index]) {
			_, index = readTypeScriptIdentifier(source, index)
			continue
		}
		index++
	}
	return 0, 0, fmt.Errorf("TypeScript pipeline must use export default definePipeline(...)")
}

func validateTypeScriptPipelineImports(source string) (map[string]struct{}, error) {
	valueImports := make(map[string]struct{})
	index := 0
	for {
		next, _, err := skipTypeScriptTrivia(source, index)
		if err != nil {
			return nil, err
		}
		index = next
		if index == len(source) {
			return valueImports, nil
		}
		if !hasTypeScriptKeyword(source, index, "import") {
			return nil, fmt.Errorf("TypeScript pipelines are declarative; only imports may appear before export default")
		}
		index += len("import")
		index, _, err = skipTypeScriptTrivia(source, index)
		if err != nil {
			return nil, err
		}
		declarationTypeOnly := false
		if hasTypeScriptKeyword(source, index, "type") {
			declarationTypeOnly = true
			index += len("type")
			index, _, err = skipTypeScriptTrivia(source, index)
			if err != nil {
				return nil, err
			}
		}
		if index >= len(source) || source[index] != '{' {
			return nil, fmt.Errorf("TypeScript pipelines may only use named imports from @buildworld/pipeline")
		}
		index++
		importCount := 0
		for {
			index, _, err = skipTypeScriptTrivia(source, index)
			if err != nil {
				return nil, err
			}
			if index < len(source) && source[index] == '}' {
				if importCount == 0 {
					return nil, fmt.Errorf("TypeScript pipeline import list cannot be empty")
				}
				index++
				break
			}
			specifierTypeOnly := false
			if hasTypeScriptKeyword(source, index, "type") {
				specifierTypeOnly = true
				index += len("type")
				index, _, err = skipTypeScriptTrivia(source, index)
				if err != nil {
					return nil, err
				}
			}
			name, afterName := readTypeScriptIdentifier(source, index)
			if name == "" {
				return nil, fmt.Errorf("TypeScript pipeline import contains an invalid name")
			}
			if _, allowed := typeScriptPipelineImports[name]; !allowed {
				return nil, fmt.Errorf("TypeScript pipelines may only import the declared @buildworld/pipeline API; %q is not exported", name)
			}
			if !declarationTypeOnly && !specifierTypeOnly {
				valueImports[name] = struct{}{}
			}
			importCount++
			index = afterName
			index, _, err = skipTypeScriptTrivia(source, index)
			if err != nil {
				return nil, err
			}
			if index < len(source) && source[index] == ',' {
				index++
				continue
			}
			if index < len(source) && source[index] == '}' {
				index++
				break
			}
			return nil, fmt.Errorf("TypeScript pipeline imports cannot be aliased or computed")
		}
		index, _, err = skipTypeScriptTrivia(source, index)
		if err != nil {
			return nil, err
		}
		if !hasTypeScriptKeyword(source, index, "from") {
			return nil, fmt.Errorf("TypeScript pipelines may only import @buildworld/pipeline")
		}
		index += len("from")
		index, _, err = skipTypeScriptTrivia(source, index)
		if err != nil {
			return nil, err
		}
		module, afterModule, err := readTypeScriptModuleString(source, index)
		if err != nil || module != "@buildworld/pipeline" {
			return nil, fmt.Errorf("TypeScript pipelines may only import @buildworld/pipeline")
		}
		index = afterModule
		index, _, err = skipTypeScriptTrivia(source, index)
		if err != nil {
			return nil, err
		}
		if index < len(source) && source[index] == ';' {
			index++
		}
	}
}

func readTypeScriptModuleString(source string, index int) (string, int, error) {
	if index >= len(source) || (source[index] != '\'' && source[index] != '"') {
		return "", index, fmt.Errorf("module path must be a string")
	}
	quote := source[index]
	start := index + 1
	for index = start; index < len(source); index++ {
		if source[index] == '\\' {
			return "", index, fmt.Errorf("escaped module paths are not allowed")
		}
		if source[index] == quote {
			return source[start:index], index + 1, nil
		}
		if source[index] == '\r' || source[index] == '\n' {
			break
		}
	}
	return "", index, fmt.Errorf("unterminated module path")
}

// stripTypeScriptPipelineAssertions accepts only the two editor-oriented
// assertions supported by this DSL. Ranges are blanked only outside literals
// and only when the assertion is in a trailing expression position.
func stripTypeScriptPipelineAssertions(source string) (string, error) {
	output := []byte(source)
	for index := 0; index < len(source); {
		if next, ok, err := skipTypeScriptTrivia(source, index); err != nil {
			return "", err
		} else if ok {
			index = next
			continue
		}
		if source[index] == '\'' || source[index] == '"' || source[index] == '`' {
			next, err := skipTypeScriptQuoted(source, index)
			if err != nil {
				return "", err
			}
			index = next
			continue
		}
		word, afterWord := readTypeScriptIdentifier(source, index)
		if word == "" {
			index++
			continue
		}
		var assertionEnd int
		switch word {
		case "as":
			next, _, err := skipTypeScriptTrivia(source, afterWord)
			if err != nil {
				return "", err
			}
			if hasTypeScriptKeyword(source, next, "const") {
				assertionEnd = next + len("const")
			}
		case "satisfies":
			next, _, err := skipTypeScriptTrivia(source, afterWord)
			if err != nil {
				return "", err
			}
			if hasTypeScriptKeyword(source, next, "Pipeline") {
				assertionEnd = next + len("Pipeline")
			}
		}
		if assertionEnd > 0 {
			trailing, _, err := skipTypeScriptTrivia(source, assertionEnd)
			if err != nil {
				return "", err
			}
			if trailing == len(source) || strings.ContainsRune(")]},;", rune(source[trailing])) {
				blankTypeScriptRange(output, index, assertionEnd)
				index = assertionEnd
				continue
			}
		}
		index = afterWord
	}
	return string(output), nil
}

func blankTypeScriptRange(source []byte, start, end int) {
	for index := start; index < end; index++ {
		if source[index] != '\r' && source[index] != '\n' {
			source[index] = ' '
		}
	}
}

// validateTypeScriptPipelineLexicalLimits bounds recursive syntax before it
// reaches the third-party parser. The AST limits below remain authoritative
// for semantic nodes.
func validateTypeScriptPipelineLexicalLimits(source string) error {
	depth := 0
	signRun := 0
	for index := 0; index < len(source); {
		if next, ok, err := skipTypeScriptTrivia(source, index); err != nil {
			return fmt.Errorf("TypeScript pipeline syntax: %w", err)
		} else if ok {
			index = next
			continue
		}
		if source[index] == '\'' || source[index] == '"' || source[index] == '`' {
			next, err := skipTypeScriptQuoted(source, index)
			if err != nil {
				return fmt.Errorf("TypeScript pipeline syntax: %w", err)
			}
			index, signRun = next, 0
			continue
		}
		if isTypeScriptIdentifierStart(source[index]) {
			word, next := readTypeScriptIdentifier(source, index)
			if _, forbidden := typeScriptPipelineForbiddenWords[word]; forbidden {
				return fmt.Errorf("TypeScript pipelines are declarative; %q is not allowed", word)
			}
			index, signRun = next, 0
			continue
		}
		switch source[index] {
		case '(', '[', '{':
			depth++
			if depth > typeScriptPipelineMaxSyntaxDepth {
				return fmt.Errorf("TypeScript pipeline exceeds maximum syntax depth of %d", typeScriptPipelineMaxSyntaxDepth)
			}
			signRun = 0
		case ')', ']', '}':
			depth--
			if depth < 0 {
				return fmt.Errorf("TypeScript pipeline syntax has unmatched delimiters")
			}
			signRun = 0
		case '+', '-':
			signRun++
			if signRun > typeScriptPipelineMaxSyntaxDepth {
				return fmt.Errorf("TypeScript pipeline exceeds maximum unary expression depth of %d", typeScriptPipelineMaxSyntaxDepth)
			}
		case '!', '~', '?', '=', '*', '/', '%', '&', '|', '^', '<', '>':
			return fmt.Errorf("TypeScript pipelines are declarative; operator %q is not allowed", source[index])
		default:
			signRun = 0
		}
		index++
	}
	if depth != 0 {
		return fmt.Errorf("TypeScript pipeline syntax has unmatched delimiters")
	}
	return nil
}

func skipTypeScriptTrivia(source string, index int) (int, bool, error) {
	start := index
	for index < len(source) {
		switch source[index] {
		case ' ', '\t', '\r', '\n', '\f', '\v':
			index++
			continue
		}
		if index+1 < len(source) && source[index:index+2] == "//" {
			index += 2
			for index < len(source) && source[index] != '\n' {
				index++
			}
			continue
		}
		if index+1 < len(source) && source[index:index+2] == "/*" {
			close := strings.Index(source[index+2:], "*/")
			if close < 0 {
				return index, index != start, fmt.Errorf("unterminated block comment")
			}
			index += close + 4
			continue
		}
		break
	}
	return index, index != start, nil
}

func skipTypeScriptQuoted(source string, index int) (int, error) {
	quote := source[index]
	for index++; index < len(source); index++ {
		if source[index] == '\\' {
			index++
			continue
		}
		if quote == '`' && source[index] == '$' && index+1 < len(source) && source[index+1] == '{' {
			return index, fmt.Errorf("template interpolation is not allowed")
		}
		if source[index] == quote {
			return index + 1, nil
		}
	}
	return index, fmt.Errorf("unterminated string or template literal")
}

func hasTypeScriptKeyword(source string, index int, keyword string) bool {
	if index < 0 || index+len(keyword) > len(source) || source[index:index+len(keyword)] != keyword {
		return false
	}
	if index > 0 && isTypeScriptIdentifierPart(source[index-1]) {
		return false
	}
	return index+len(keyword) == len(source) || !isTypeScriptIdentifierPart(source[index+len(keyword)])
}

func readTypeScriptIdentifier(source string, index int) (string, int) {
	if index >= len(source) || !isTypeScriptIdentifierStart(source[index]) {
		return "", index
	}
	end := index + 1
	for end < len(source) && isTypeScriptIdentifierPart(source[end]) {
		end++
	}
	return source[index:end], end
}

func isTypeScriptIdentifierStart(value byte) bool {
	return value == '_' || value == '$' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func isTypeScriptIdentifierPart(value byte) bool {
	return isTypeScriptIdentifierStart(value) || value >= '0' && value <= '9'
}

type typeScriptPipelineEvaluator struct {
	nodes        int
	outputBytes  int
	valueImports map[string]struct{}
}

func (e *typeScriptPipelineEvaluator) evaluate(expression ast.Expression, depth int) (interface{}, error) {
	if err := e.consumeNode(depth); err != nil {
		return nil, err
	}
	switch expression := expression.(type) {
	case *ast.StringLiteral:
		value := expression.Value.String()
		if err := e.consumeOutput(len(value)); err != nil {
			return nil, err
		}
		return value, nil
	case *ast.TemplateLiteral:
		if expression.Tag != nil {
			return nil, fmt.Errorf("TypeScript pipelines are declarative; tagged templates are not allowed")
		}
		if len(expression.Expressions) != 0 {
			return nil, fmt.Errorf("TypeScript pipelines are declarative; template interpolation is not allowed")
		}
		var value strings.Builder
		for _, element := range expression.Elements {
			value.WriteString(element.Parsed.String())
		}
		if err := e.consumeOutput(value.Len()); err != nil {
			return nil, err
		}
		return value.String(), nil
	case *ast.NumberLiteral:
		switch value := expression.Value.(type) {
		case int64:
			if err := e.consumeOutput(24); err != nil {
				return nil, err
			}
			return value, nil
		case float64:
			if math.IsInf(value, 0) || math.IsNaN(value) {
				return nil, fmt.Errorf("TypeScript pipeline number must be finite")
			}
			if err := e.consumeOutput(24); err != nil {
				return nil, err
			}
			return value, nil
		default:
			return nil, fmt.Errorf("TypeScript pipelines are declarative; bigint and non-JSON numbers are not allowed")
		}
	case *ast.BooleanLiteral:
		if err := e.consumeOutput(5); err != nil {
			return nil, err
		}
		return expression.Value, nil
	case *ast.NullLiteral:
		if err := e.consumeOutput(4); err != nil {
			return nil, err
		}
		return nil, nil
	case *ast.UnaryExpression:
		return e.evaluateNumberSign(expression, depth)
	case *ast.ArrayLiteral:
		values := make([]interface{}, 0, len(expression.Value))
		for _, item := range expression.Value {
			if item == nil {
				return nil, fmt.Errorf("TypeScript pipelines are declarative; sparse arrays are not allowed")
			}
			value, err := e.evaluate(item, depth+1)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	case *ast.ObjectLiteral:
		return e.evaluateObject(expression, depth)
	case *ast.CallExpression:
		return e.evaluateCall(expression, depth)
	default:
		return nil, fmt.Errorf("TypeScript pipelines are declarative; expression %T is not allowed", expression)
	}
}

func (e *typeScriptPipelineEvaluator) evaluateNumberSign(expression *ast.UnaryExpression, depth int) (interface{}, error) {
	operator := expression.Operator.String()
	if expression.Postfix || operator != "+" && operator != "-" {
		return nil, fmt.Errorf("TypeScript pipelines are declarative; unary operator %q is not allowed", operator)
	}
	value, err := e.evaluate(expression.Operand, depth+1)
	if err != nil {
		return nil, err
	}
	switch value := value.(type) {
	case int64:
		if operator == "+" {
			return value, nil
		}
		if value == math.MinInt64 {
			return nil, fmt.Errorf("TypeScript pipeline number is out of range")
		}
		return -value, nil
	case float64:
		if operator == "-" {
			value = -value
		}
		if math.IsInf(value, 0) || math.IsNaN(value) {
			return nil, fmt.Errorf("TypeScript pipeline number must be finite")
		}
		return value, nil
	default:
		return nil, fmt.Errorf("TypeScript pipelines are declarative; unary signs require a numeric literal")
	}
}

func (e *typeScriptPipelineEvaluator) evaluateObject(object *ast.ObjectLiteral, depth int) (interface{}, error) {
	value := make(map[string]interface{}, len(object.Value))
	for _, property := range object.Value {
		if err := e.consumeNode(depth + 1); err != nil {
			return nil, err
		}
		keyed, ok := property.(*ast.PropertyKeyed)
		if !ok || keyed.Computed || keyed.Kind != ast.PropertyKindValue {
			return nil, fmt.Errorf("TypeScript pipelines are declarative; spreads, shorthand, methods, getters, setters, and computed properties are not allowed")
		}
		key, err := e.propertyName(keyed.Key, depth+2)
		if err != nil {
			return nil, err
		}
		if key == "__proto__" || key == "prototype" || key == "constructor" {
			return nil, fmt.Errorf("TypeScript pipelines are declarative; property %q is not allowed", key)
		}
		if _, duplicate := value[key]; duplicate {
			return nil, fmt.Errorf("TypeScript pipeline object contains duplicate property %q", key)
		}
		propertyValue, err := e.evaluate(keyed.Value, depth+2)
		if err != nil {
			return nil, err
		}
		if err := e.consumeOutput(len(key)); err != nil {
			return nil, err
		}
		value[key] = propertyValue
	}
	return value, nil
}

func (e *typeScriptPipelineEvaluator) propertyName(expression ast.Expression, depth int) (string, error) {
	if err := e.consumeNode(depth); err != nil {
		return "", err
	}
	switch expression := expression.(type) {
	case *ast.Identifier:
		return expression.Name.String(), nil
	case *ast.StringLiteral:
		return expression.Value.String(), nil
	case *ast.NumberLiteral:
		switch value := expression.Value.(type) {
		case int64:
			return fmt.Sprintf("%d", value), nil
		case float64:
			if math.IsInf(value, 0) || math.IsNaN(value) {
				return "", fmt.Errorf("TypeScript pipeline property number must be finite")
			}
			return fmt.Sprintf("%g", value), nil
		}
	}
	return "", fmt.Errorf("TypeScript pipelines are declarative; object property keys must be identifiers, strings, or numbers")
}

func (e *typeScriptPipelineEvaluator) evaluateCall(call *ast.CallExpression, depth int) (interface{}, error) {
	name := typeScriptIdentifier(call.Callee)
	if _, allowed := typeScriptPipelineCalls[name]; !allowed {
		return nil, fmt.Errorf("TypeScript pipelines are declarative; only direct @buildworld/pipeline helper calls are allowed")
	}
	if _, imported := e.valueImports[name]; !imported {
		return nil, fmt.Errorf("helper %q must be imported as a value from @buildworld/pipeline", name)
	}
	if err := e.consumeNode(depth + 1); err != nil {
		return nil, err
	}
	arguments := make([]interface{}, 0, len(call.ArgumentList))
	for _, argument := range call.ArgumentList {
		value, err := e.evaluate(argument, depth+1)
		if err != nil {
			return nil, err
		}
		arguments = append(arguments, value)
	}
	return e.callHelper(name, arguments)
}

func typeScriptIdentifier(expression ast.Expression) string {
	identifier, ok := expression.(*ast.Identifier)
	if !ok {
		return ""
	}
	return identifier.Name.String()
}

func (e *typeScriptPipelineEvaluator) consumeNode(depth int) error {
	if depth > typeScriptPipelineMaxDepth {
		return fmt.Errorf("TypeScript pipeline exceeds maximum AST depth of %d", typeScriptPipelineMaxDepth)
	}
	e.nodes++
	if e.nodes > typeScriptPipelineMaxNodes {
		return fmt.Errorf("TypeScript pipeline exceeds maximum AST node count of %d", typeScriptPipelineMaxNodes)
	}
	return nil
}

func (e *typeScriptPipelineEvaluator) consumeOutput(size int) error {
	e.outputBytes += size
	if e.outputBytes > typeScriptPipelineMaxOutputBytes {
		return fmt.Errorf("TypeScript pipeline output exceeds 512 KiB")
	}
	return nil
}

func (e *typeScriptPipelineEvaluator) callHelper(name string, arguments []interface{}) (interface{}, error) {
	switch name {
	case "definePipeline":
		if err := typeScriptArgumentCount(name, arguments, 1, 1); err != nil {
			return nil, err
		}
		pipeline, err := typeScriptObjectArgument(name, arguments[0], false)
		if err != nil {
			return nil, err
		}
		pipeline = cloneTypeScriptObject(pipeline)
		typeScriptAlias(pipeline, "agentRequirements", "agent_requirements")
		typeScriptAlias(pipeline, "retentionCompleted", "retention_completed")
		typeScriptAlias(pipeline, "timeoutSec", "timeout_sec")
		typeScriptAlias(pipeline, "allowLongRunning", "allow_long_running")
		typeScriptAlias(pipeline, "disableConcurrent", "disable_concurrent")
		typeScriptAlias(pipeline, "abortPrevious", "abort_previous")
		typeScriptAlias(pipeline, "on", "triggers")
		if approval, ok := pipeline["approval"].(map[string]interface{}); ok {
			approval = cloneTypeScriptObject(approval)
			typeScriptAlias(approval, "requiredRoles", "required_roles")
			typeScriptAlias(approval, "allowRequester", "allow_requester")
			pipeline["approval"] = approval
		}
		return pipeline, nil
	case "step":
		if err := typeScriptArgumentCount(name, arguments, 3, 4); err != nil {
			return nil, err
		}
		stepName, err := typeScriptStringArgument(name, arguments[0])
		if err != nil {
			return nil, err
		}
		stepType, err := typeScriptStringArgument(name, arguments[1])
		if err != nil {
			return nil, err
		}
		command, err := typeScriptStringArgument(name, arguments[2])
		if err != nil {
			return nil, err
		}
		options, err := typeScriptOptionalOptions(name, arguments, 3)
		if err != nil {
			return nil, err
		}
		typeScriptAlias(options, "platformAdditions", "platform_additions")
		options["name"], options["type"], options["command"] = stepName, stepType, command
		return options, nil
	case "shell", "tail", "script":
		if err := typeScriptArgumentCount(name, arguments, 2, 3); err != nil {
			return nil, err
		}
		stepName, err := typeScriptStringArgument(name, arguments[0])
		if err != nil {
			return nil, err
		}
		command, err := typeScriptStringArgument(name, arguments[1])
		if err != nil {
			return nil, err
		}
		options, err := typeScriptOptionalOptions(name, arguments, 2)
		if err != nil {
			return nil, err
		}
		typeScriptAlias(options, "platformAdditions", "platform_additions")
		options["name"], options["type"], options["command"] = stepName, name, command
		return options, nil
	case "git", "notify":
		if err := typeScriptArgumentCount(name, arguments, 1, 2); err != nil {
			return nil, err
		}
		stepName, err := typeScriptStringArgument(name, arguments[0])
		if err != nil {
			return nil, err
		}
		options, err := typeScriptOptionalOptions(name, arguments, 1)
		if err != nil {
			return nil, err
		}
		typeScriptAlias(options, "platformAdditions", "platform_additions")
		options["name"], options["type"], options["command"] = stepName, name, ""
		return options, nil
	case "watchService":
		if err := typeScriptArgumentCount(name, arguments, 2, 2); err != nil {
			return nil, err
		}
		stepName, err := typeScriptStringArgument(name, arguments[0])
		if err != nil {
			return nil, err
		}
		options, err := typeScriptObjectArgument(name, arguments[1], false)
		if err != nil {
			return nil, err
		}
		options = cloneTypeScriptObject(options)
		typeScriptAlias(options, "targetDir", "target_dir")
		typeScriptAlias(options, "pidFile", "pid_file")
		typeScriptAlias(options, "logFile", "log_file")
		typeScriptAlias(options, "heartbeatSeconds", "heartbeat_seconds")
		typeScriptAlias(options, "pollSeconds", "poll_seconds")
		typeScriptAlias(options, "initialLines", "initial_lines")
		config := make(map[string]interface{}, len(options))
		for key, value := range options {
			if value == nil {
				continue
			}
			text, err := typeScriptScalarString(value)
			if err != nil {
				return nil, fmt.Errorf("%s option %q: %w", name, key, err)
			}
			config[key] = text
		}
		return map[string]interface{}{"name": stepName, "type": "service_watch", "config": config}, nil
	case "stage":
		if err := typeScriptArgumentCount(name, arguments, 2, 3); err != nil {
			return nil, err
		}
		stageName, err := typeScriptStringArgument(name, arguments[0])
		if err != nil {
			return nil, err
		}
		options, err := typeScriptOptionalOptions(name, arguments, 2)
		if err != nil {
			return nil, err
		}
		typeScriptAlias(options, "dependsOn", "depends_on")
		typeScriptAlias(options, "workingDirectory", "working_directory")
		typeScriptAlias(options, "timeoutSec", "timeout_sec")
		steps, ok := arguments[1].([]interface{})
		if !ok {
			steps = []interface{}{arguments[1]}
		}
		options["name"], options["steps"] = stageName, steps
		return options, nil
	case "trigger":
		if err := typeScriptArgumentCount(name, arguments, 1, 2); err != nil {
			return nil, err
		}
		triggerType, err := typeScriptStringArgument(name, arguments[0])
		if err != nil {
			return nil, err
		}
		config := map[string]interface{}{}
		if len(arguments) == 2 {
			config, err = typeScriptObjectArgument(name, arguments[1], true)
			if err != nil {
				return nil, err
			}
		}
		return map[string]interface{}{"type": triggerType, "config": cloneTypeScriptObject(config)}, nil
	case "parameter":
		if err := typeScriptArgumentCount(name, arguments, 2, 3); err != nil {
			return nil, err
		}
		parameterName, err := typeScriptStringArgument(name, arguments[0])
		if err != nil {
			return nil, err
		}
		parameterType, err := typeScriptStringArgument(name, arguments[1])
		if err != nil {
			return nil, err
		}
		options, err := typeScriptOptionalOptions(name, arguments, 2)
		if err != nil {
			return nil, err
		}
		typeScriptAlias(options, "isSecret", "is_secret")
		options["name"], options["type"] = parameterName, parameterType
		return options, nil
	}
	return nil, fmt.Errorf("TypeScript pipeline helper %q is not supported", name)
}

func typeScriptArgumentCount(name string, arguments []interface{}, minimum, maximum int) error {
	if len(arguments) < minimum || len(arguments) > maximum {
		if minimum == maximum {
			return fmt.Errorf("%s requires exactly %d argument(s)", name, minimum)
		}
		return fmt.Errorf("%s requires %d to %d arguments", name, minimum, maximum)
	}
	return nil
}

func typeScriptStringArgument(name string, value interface{}) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s requires string arguments", name)
	}
	return text, nil
}

func typeScriptObjectArgument(name string, value interface{}, allowNull bool) (map[string]interface{}, error) {
	if value == nil && allowNull {
		return map[string]interface{}{}, nil
	}
	object, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s requires an object argument", name)
	}
	return object, nil
}

func typeScriptOptionalOptions(name string, arguments []interface{}, index int) (map[string]interface{}, error) {
	if len(arguments) <= index || arguments[index] == nil {
		return map[string]interface{}{}, nil
	}
	options, err := typeScriptObjectArgument(name, arguments[index], false)
	if err != nil {
		return nil, err
	}
	return cloneTypeScriptObject(options), nil
}

func cloneTypeScriptObject(source map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func typeScriptAlias(value map[string]interface{}, camel, snake string) {
	if _, exists := value[snake]; exists {
		return
	}
	if alias, exists := value[camel]; exists {
		value[snake] = alias
	}
}

func typeScriptScalarString(value interface{}) (string, error) {
	switch value := value.(type) {
	case string:
		return value, nil
	case bool:
		if value {
			return "true", nil
		}
		return "false", nil
	case int64:
		return fmt.Sprintf("%d", value), nil
	case float64:
		if math.IsInf(value, 0) || math.IsNaN(value) {
			return "", fmt.Errorf("number must be finite")
		}
		return fmt.Sprintf("%g", value), nil
	default:
		return "", fmt.Errorf("must be a string, number, or boolean literal")
	}
}

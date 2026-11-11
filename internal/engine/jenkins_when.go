package engine

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/big"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
)

// A NUL-containing key cannot originate from an OS environment variable.
const jenkinsBuildNumberContextKey = "\x00buildworld.currentBuild.number"
const jenkinsSCMChangesContextKey = "\x00buildworld.currentBuild.scmChanges"
const jenkinsSCMHistoryUnknownContextKey = "\x00buildworld.currentBuild.scmHistoryUnknown"
const jenkinsTriggerTypeContextKey = "\x00buildworld.currentBuild.trigger"
const jenkinsTriggerActorContextKey = "\x00buildworld.currentBuild.triggerActor"
const jenkinsRetryCausesContextKey = "\x00buildworld.currentBuild.retryCauses"

var jenkinsDecimalSyntax = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

// jenkinsDecimal preserves Groovy BigDecimal scale for both comparison and toString.
type jenkinsDecimal string

func parseJenkinsDecimal(text string) (jenkinsDecimal, error) {
	unsigned := strings.TrimPrefix(text, "-")
	if !jenkinsDecimalSyntax.MatchString(unsigned) {
		return "", fmt.Errorf("unsupported Jenkins decimal literal")
	}
	if _, _, err := jenkinsNumeric(json.Number(text)); err != nil {
		return "", err
	}
	return jenkinsDecimal(text), nil
}

func (decimal jenkinsDecimal) String() string {
	text := string(decimal)
	negative := strings.HasPrefix(text, "-")
	text = strings.TrimPrefix(text, "-")
	exponent := 0
	if index := strings.IndexAny(text, "eE"); index >= 0 {
		exponent, _ = strconv.Atoi(text[index+1:]) // bounded by parseJenkinsDecimal
		text = text[:index]
	}
	whole, fraction, _ := strings.Cut(text, ".")
	scale := len(fraction) - exponent
	digits := strings.TrimLeft(whole+fraction, "0")
	if digits == "" {
		digits = "0"
		negative = false
	}
	adjusted := len(digits) - 1 - scale
	var result string
	if scale >= 0 && adjusted >= -6 {
		switch {
		case scale == 0:
			result = digits
		case scale < len(digits):
			point := len(digits) - scale
			result = digits[:point] + "." + digits[point:]
		default:
			result = "0." + strings.Repeat("0", scale-len(digits)) + digits
		}
	} else {
		result = digits[:1]
		if len(digits) > 1 {
			result += "." + digits[1:]
		}
		result += fmt.Sprintf("E%+d", adjusted)
	}
	if negative {
		result = "-" + result
	}
	return result
}

func pipelineUsesJenkinsTrigger(config *BuildConfig) bool {
	for _, stage := range config.Stages {
		expr, err := parser.ParseExpr(stage.JenkinsWhen)
		if err != nil {
			continue
		}
		found := false
		ast.Inspect(expr, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "jenkinsTriggeredBy" {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

// jenkinsCauseLiteral limits patterns to the Java-regex-compatible literal
// subset; unsupported regular expressions must not silently change a guard.
func jenkinsCauseLiteral(cause string) bool {
	if len(cause) == 0 || len(cause) > 256 {
		return false
	}
	for _, char := range cause {
		if !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') && char != '_' {
			return false
		}
	}
	return true
}

func jenkinsCauseMatches(actual, cause string) bool {
	return strings.HasPrefix(strings.ToLower(actual), strings.ToLower(cause))
}

// ValidateJenkinsWhen accepts only the supported, side-effect-free boolean subset.
func ValidateJenkinsWhen(condition string) error {
	if strings.TrimSpace(condition) == "" {
		return nil
	}
	expr, err := parser.ParseExpr(condition)
	if err != nil {
		return fmt.Errorf("unsupported Jenkins expression: %w", err)
	}
	return validateJenkinsExpr(expr)
}

func validateJenkinsExpr(expr ast.Expr) error {
	switch node := expr.(type) {
	case *ast.ParenExpr:
		return validateJenkinsExpr(node.X)
	case *ast.Ident:
		if node.Name == "true" || node.Name == "false" || node.Name == "nil" || node.Name == "null" {
			return nil
		}
	case *ast.BasicLit:
		if node.Kind == token.STRING || node.Kind == token.INT {
			return nil
		}
		if node.Kind == token.FLOAT {
			_, err := parseJenkinsDecimal(node.Value)
			return err
		}
	case *ast.SelectorExpr:
		if root, ok := node.X.(*ast.Ident); ok && (root.Name == "params" || root.Name == "env") {
			return nil
		}
		if root, ok := node.X.(*ast.Ident); ok && root.Name == "currentBuild" && node.Sel.Name == "number" {
			return nil
		}
	case *ast.CallExpr:
		if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "jenkinsTriggeredBy" && len(node.Args) == 3 {
			values := make([]string, 3)
			for i, arg := range node.Args {
				literal, ok := arg.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return fmt.Errorf("triggeredBy arguments must be literal strings")
				}
				value, err := strconv.Unquote(literal.Value)
				if err != nil {
					return err
				}
				values[i] = value
			}
			if !jenkinsCauseLiteral(values[0]) {
				return fmt.Errorf("unsupported triggeredBy cause")
			}
			if values[2] != "true" && values[2] != "false" {
				return fmt.Errorf("invalid triggeredBy detail")
			}
			return nil
		}
		if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "jenkinsBuildingTag" && len(node.Args) == 0 {
			return nil
		}
		if len(node.Args) == 0 {
			if selector, ok := node.Fun.(*ast.SelectorExpr); ok && (selector.Sel.Name == "toString" || selector.Sel.Name == "toBoolean") {
				return validateJenkinsExpr(selector.X)
			}
		}
		if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "jenkinsChangeRequest" && len(node.Args) >= 1 && len(node.Args)%2 == 1 {
			for i, arg := range node.Args {
				literal, ok := arg.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return fmt.Errorf("changeRequest arguments must be literal strings")
				}
				if i > 0 && i%2 == 1 {
					field, err := strconv.Unquote(literal.Value)
					if err != nil || !jenkinsChangeRequestField(field) {
						return fmt.Errorf("invalid changeRequest field")
					}
				}
			}
			return nil
		}
		if ident, ok := node.Fun.(*ast.Ident); ok && ((ident.Name == "jenkinsChangeSet" && len(node.Args) == 3) || (ident.Name == "jenkinsChangeLog" && len(node.Args) == 1)) {
			for _, arg := range node.Args {
				if literal, ok := arg.(*ast.BasicLit); !ok || literal.Kind != token.STRING {
					return fmt.Errorf("Jenkins SCM condition arguments must be literal strings")
				}
			}
			if ident.Name == "jenkinsChangeSet" {
				caseSensitive, err := strconv.Unquote(node.Args[2].(*ast.BasicLit).Value)
				if err != nil || (caseSensitive != "true" && caseSensitive != "false") {
					return fmt.Errorf("changeset caseSensitive must be a literal boolean")
				}
			}
			return nil
		}
		if ident, ok := node.Fun.(*ast.Ident); ok && (((ident.Name == "jenkinsBranch" || ident.Name == "jenkinsTag") && (len(node.Args) == 1 || len(node.Args) == 2)) || (ident.Name == "jenkinsEnvironment" && (len(node.Args) == 2 || len(node.Args) == 3))) {
			for _, arg := range node.Args {
				if literal, ok := arg.(*ast.BasicLit); !ok || literal.Kind != token.STRING {
					return fmt.Errorf("Jenkins condition arguments must be literal strings")
				}
			}
			if ident.Name == "jenkinsEnvironment" && len(node.Args) == 3 {
				value, err := strconv.Unquote(node.Args[2].(*ast.BasicLit).Value)
				if err != nil || (value != "true" && value != "false") {
					return fmt.Errorf("Jenkins environment ignoreCase must be a literal boolean")
				}
			}
			return nil
		}
	case *ast.UnaryExpr:
		if node.Op == token.NOT {
			return validateJenkinsExpr(node.X)
		}
		if node.Op == token.SUB {
			if literal, ok := node.X.(*ast.BasicLit); ok {
				if literal.Kind == token.INT {
					return nil
				}
				if literal.Kind == token.FLOAT {
					_, err := parseJenkinsDecimal("-" + literal.Value)
					return err
				}
			}
		}
	case *ast.BinaryExpr:
		if node.Op == token.LAND || node.Op == token.LOR || node.Op == token.EQL || node.Op == token.NEQ || node.Op == token.LSS || node.Op == token.LEQ || node.Op == token.GTR || node.Op == token.GEQ {
			if err := validateJenkinsExpr(node.X); err != nil {
				return err
			}
			return validateJenkinsExpr(node.Y)
		}
	}
	return fmt.Errorf("unsupported Jenkins boolean expression %T", expr)
}

// EvaluateJenkinsWhen evaluates boolean parameters without executing Groovy code.
func EvaluateJenkinsWhen(condition string, params map[string]interface{}) (bool, error) {
	return EvaluateJenkinsWhenContext(condition, params, nil, "")
}

// EvaluateJenkinsWhenContext applies Jenkins/Groovy truth using the build environment and branch.
func EvaluateJenkinsWhenContext(condition string, params map[string]interface{}, environment map[string]string, branch string) (bool, error) {
	if err := ValidateJenkinsWhen(condition); err != nil {
		return false, err
	}
	expr, err := parser.ParseExpr(condition)
	if err != nil {
		return false, err
	}
	value, err := evalJenkinsExpr(expr, params, environment, branch)
	if err != nil {
		return false, err
	}
	return jenkinsTruth(value)
}

func jenkinsTruth(value interface{}) (bool, error) {
	switch v := value.(type) {
	case nil:
		return false, nil
	case bool:
		return v, nil
	case string:
		return v != "", nil
	case float64:
		return v != 0, nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, json.Number, jenkinsDecimal:
		n, _, err := jenkinsNumeric(v)
		if err != nil {
			return false, err
		}
		return n.Sign() != 0, nil
	default:
		return false, fmt.Errorf("unsupported Jenkins truth value %T", value)
	}
}

func evalJenkinsExpr(expr ast.Expr, params map[string]interface{}, environment map[string]string, branch string) (interface{}, error) {
	switch node := expr.(type) {
	case *ast.ParenExpr:
		return evalJenkinsExpr(node.X, params, environment, branch)
	case *ast.Ident:
		if node.Name == "nil" || node.Name == "null" {
			return nil, nil
		}
		return node.Name == "true", nil
	case *ast.BasicLit:
		if node.Kind == token.INT {
			number, err := strconv.ParseInt(node.Value, 0, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid or excessive Jenkins integer")
			}
			return json.Number(strconv.FormatInt(number, 10)), nil
		}
		if node.Kind == token.FLOAT {
			return parseJenkinsDecimal(node.Value)
		}
		return strconv.Unquote(node.Value)
	case *ast.SelectorExpr:
		if root, ok := node.X.(*ast.Ident); ok && root.Name == "currentBuild" {
			value, exists := environment[jenkinsBuildNumberContextKey]
			if !exists {
				value = environment["BUILD_NUMBER"]
			}
			number, err := strconv.ParseInt(value, 10, 64)
			if err != nil || number < 1 {
				return nil, fmt.Errorf("Jenkins build number is unavailable")
			}
			return json.Number(strconv.FormatInt(number, 10)), nil
		}
		if root, ok := node.X.(*ast.Ident); ok && root.Name == "env" {
			if value, found := environment[node.Sel.Name]; found {
				return value, nil
			}
			return nil, nil
		}
		v, found := params[node.Sel.Name]
		if !found {
			return nil, nil
		}
		switch v.(type) {
		case bool, string, nil, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
			return v, nil
		}
		return nil, fmt.Errorf("unsupported Jenkins parameter %q type %T", node.Sel.Name, v)
	case *ast.CallExpr:
		if ident, ok := node.Fun.(*ast.Ident); ok {
			if ident.Name == "jenkinsTriggeredBy" {
				cause, _ := strconv.Unquote(node.Args[0].(*ast.BasicLit).Value)
				detail, _ := strconv.Unquote(node.Args[1].(*ast.BasicLit).Value)
				hasDetail, _ := strconv.Unquote(node.Args[2].(*ast.BasicLit).Value)
				trigger := environment[jenkinsTriggerTypeContextKey]
				if trigger == "retry" {
					var result jenkinsRetryCauses
					if json.Unmarshal([]byte(environment[jenkinsRetryCausesContextKey]), &result) != nil {
						return nil, fmt.Errorf("Jenkins retry cause is unavailable")
					}
					for _, candidate := range result.Causes {
						if !jenkinsCauseMatches(candidate.Class, cause) {
							continue
						}
						if candidate.Class == "UserIdCause" && cause == "UserIdCause" && hasDetail == "true" {
							if candidate.User == "" {
								return nil, fmt.Errorf("authenticated retry user is unavailable")
							}
							if candidate.User != detail {
								continue
							}
						}
						return true, nil
					}
					if result.UnknownNonUser && !(result.HasManualRetry && cause == "UserIdCause") {
						return nil, fmt.Errorf("original Jenkins trigger cause is unavailable")
					}
					return false, nil
				}
				actual := ""
				switch trigger {
				case "schedule":
					actual = "TimerTriggerCause"
				case "vcs":
					actual = "SCMTriggerCause"
				case "finish":
					actual = "BuildUpstreamCause"
				case "http":
					actual = "RemoteCause"
				case "manual", "api":
					actual = "UserIdCause"
				default:
					return nil, fmt.Errorf("Jenkins trigger cause is unavailable for %q", trigger)
				}
				if !jenkinsCauseMatches(actual, cause) {
					return false, nil
				}
				if actual == "UserIdCause" && cause == "UserIdCause" && hasDetail == "true" {
					user := environment[jenkinsTriggerActorContextKey]
					if user == "" {
						return nil, fmt.Errorf("authenticated trigger user is unavailable")
					}
					return user == detail, nil
				}
				return true, nil
			}
			if ident.Name == "jenkinsBuildingTag" {
				return environment["TAG_NAME"] != "", nil
			}
			if ident.Name == "jenkinsChangeSet" || ident.Name == "jenkinsChangeLog" {
				if environment[jenkinsSCMHistoryUnknownContextKey] == "true" {
					return nil, fmt.Errorf("previous Jenkins SCM checkout history is unavailable")
				}
				var changes []jenkinsSCMChange
				if raw := environment[jenkinsSCMChangesContextKey]; raw != "" {
					if err := json.Unmarshal([]byte(raw), &changes); err != nil {
						return nil, fmt.Errorf("invalid Jenkins SCM changelog: %w", err)
					}
				}
				pattern, _ := strconv.Unquote(node.Args[0].(*ast.BasicLit).Value)
				if ident.Name == "jenkinsChangeLog" {
					return jenkinsMatchesChangeLog(pattern, changes)
				}
				comparator, _ := strconv.Unquote(node.Args[1].(*ast.BasicLit).Value)
				caseSensitive, _ := strconv.Unquote(node.Args[2].(*ast.BasicLit).Value)
				return jenkinsMatchesChangeSet(pattern, comparator, caseSensitive == "true", changes)
			}
			if ident.Name == "jenkinsChangeRequest" {
				if _, exists := environment["CHANGE_ID"]; !exists {
					return false, nil
				}
				comparator, _ := strconv.Unquote(node.Args[0].(*ast.BasicLit).Value)
				for i := 1; i < len(node.Args); i += 2 {
					field, _ := strconv.Unquote(node.Args[i].(*ast.BasicLit).Value)
					pattern, _ := strconv.Unquote(node.Args[i+1].(*ast.BasicLit).Value)
					if pattern == "" {
						continue
					}
					actual := environment[field]
					matched, err := compareJenkinsString(pattern, actual, comparator, "EQUALS")
					if err != nil || !matched {
						return matched, err
					}
				}
				return true, nil
			}
			first, _ := strconv.Unquote(node.Args[0].(*ast.BasicLit).Value)
			switch ident.Name {
			case "jenkinsBranch", "jenkinsTag":
				actual := branch
				if ident.Name == "jenkinsTag" {
					actual = environment["TAG_NAME"]
					if actual == "" {
						return false, nil
					}
					if first == "" {
						return true, nil
					}
				} else if actual == "" || first == "" {
					return actual == "" && first == "", nil
				}
				comparator := "GLOB"
				if len(node.Args) == 2 {
					comparator, _ = strconv.Unquote(node.Args[1].(*ast.BasicLit).Value)
				}
				return compareJenkinsString(first, actual, comparator, "GLOB")
			case "jenkinsEnvironment":
				second, _ := strconv.Unquote(node.Args[1].(*ast.BasicLit).Value)
				if len(node.Args) == 3 {
					ignoreCase, _ := strconv.Unquote(node.Args[2].(*ast.BasicLit).Value)
					if ignoreCase == "true" {
						return jenkinsEqualsIgnoreCase(environment[first], second), nil
					}
				}
				// Jenkins treats an absent environment variable and an empty value
				// as equivalent for this condition.
				return environment[first] == second, nil
			}
		}
		selector := node.Fun.(*ast.SelectorExpr)
		v, err := evalJenkinsExpr(selector.X, params, environment, branch)
		if err != nil {
			return nil, err
		}
		switch selector.Sel.Name {
		case "toString":
			if v == nil {
				return "null", nil
			}
			return fmt.Sprint(v), nil
		case "toBoolean":
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("Jenkins toBoolean requires a string")
			}
			s = strings.TrimSpace(s)
			return strings.EqualFold(s, "true") || strings.EqualFold(s, "y") || s == "1", nil
		}
	case *ast.UnaryExpr:
		if node.Op == token.SUB {
			literal := node.X.(*ast.BasicLit)
			if literal.Kind == token.FLOAT {
				return parseJenkinsDecimal("-" + literal.Value)
			}
			number, err := strconv.ParseInt("-"+literal.Value, 0, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid or excessive Jenkins integer")
			}
			return json.Number(strconv.FormatInt(number, 10)), nil
		}
		v, err := evalJenkinsExpr(node.X, params, environment, branch)
		if err != nil {
			return nil, err
		}
		b, err := jenkinsTruth(v)
		if err != nil {
			return nil, err
		}
		return !b, nil
	case *ast.BinaryExpr:
		left, err := evalJenkinsExpr(node.X, params, environment, branch)
		if err != nil {
			return false, err
		}
		if node.Op == token.LAND || node.Op == token.LOR {
			b, err := jenkinsTruth(left)
			if err != nil {
				return nil, err
			}
			if node.Op == token.LAND && !b {
				return false, nil
			}
			if node.Op == token.LOR && b {
				return true, nil
			}
		}
		right, err := evalJenkinsExpr(node.Y, params, environment, branch)
		if err != nil {
			return false, err
		}
		switch node.Op {
		case token.LAND:
			return jenkinsTruth(right)
		case token.LOR:
			return jenkinsTruth(right)
		case token.EQL:
			return jenkinsEqual(left, right)
		case token.NEQ:
			equal, err := jenkinsEqual(left, right)
			return !equal, err
		case token.LSS, token.LEQ, token.GTR, token.GEQ:
			comparison, err := jenkinsCompare(left, right)
			if err != nil {
				return false, err
			}
			switch node.Op {
			case token.LSS:
				return comparison < 0, nil
			case token.LEQ:
				return comparison <= 0, nil
			case token.GTR:
				return comparison > 0, nil
			default:
				return comparison >= 0, nil
			}
		}
	}
	return false, fmt.Errorf("unsupported Jenkins expression")
}

// jenkinsEqualsIgnoreCase uses Java's simple upper-then-lower case mapping.
func jenkinsEqualsIgnoreCase(left, right string) bool {
	a, b := []rune(left), []rune(right)
	if len(utf16.Encode(a)) != len(utf16.Encode(b)) || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] && unicode.ToLower(unicode.ToUpper(a[i])) != unicode.ToLower(unicode.ToUpper(b[i])) {
			return false
		}
	}
	return true
}

func jenkinsChangeRequestField(field string) bool {
	switch field {
	case "CHANGE_ID", "CHANGE_TARGET", "CHANGE_BRANCH", "CHANGE_FORK", "CHANGE_TITLE", "CHANGE_AUTHOR", "CHANGE_AUTHOR_DISPLAY_NAME", "CHANGE_AUTHOR_EMAIL":
		return true
	default:
		return false
	}
}

func compareJenkinsString(pattern, actual, comparator, fallback string) (bool, error) {
	actual = jenkinsDefaultIfBlank(actual)
	comparator = strings.ToUpper(comparator)
	if comparator != "EQUALS" && comparator != "REGEXP" && comparator != "GLOB" {
		comparator = fallback
	}
	switch comparator {
	case "EQUALS":
		return pattern == actual, nil
	case "REGEXP":
		return regexp.MatchString("^(?:"+pattern+")$", actual)
	default:
		return matchJenkinsBranch(pattern, actual)
	}
}

// Jenkins' Apache StringUtils.defaultIfBlank uses Java Character.isWhitespace,
// which includes information separators but excludes non-breaking spaces.
func jenkinsDefaultIfBlank(value string) string {
	for _, r := range value {
		if r >= '\u001c' && r <= '\u001f' {
			continue
		}
		if !unicode.IsSpace(r) || r == '\u0085' || r == '\u00a0' || r == '\u2007' || r == '\u202f' {
			return value
		}
	}
	return ""
}

// jenkinsEqual keeps numeric values numeric and never coerces strings to numbers.
func jenkinsEqual(left, right interface{}) (bool, error) {
	a, aNumber, err := jenkinsNumeric(left)
	if err != nil {
		return false, err
	}
	b, bNumber, err := jenkinsNumeric(right)
	if err != nil {
		return false, err
	}
	if aNumber || bNumber {
		return aNumber && bNumber && a.Cmp(b) == 0, nil
	}
	return left == right, nil
}

func jenkinsNumeric(value interface{}) (*big.Rat, bool, error) {
	var text string
	switch v := value.(type) {
	case json.Number:
		text = string(v)
	case jenkinsDecimal:
		text = string(v)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		text = fmt.Sprint(v)
	case float32:
		text = strconv.FormatFloat(float64(v), 'g', -1, 32)
	case float64:
		text = strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return nil, false, nil
	}
	if len(text) > 256 {
		return nil, true, fmt.Errorf("Jenkins number is too large")
	}
	if index := strings.IndexAny(text, "eE"); index >= 0 {
		exponent, err := strconv.Atoi(text[index+1:])
		if err != nil || exponent < -1000 || exponent > 1000 {
			return nil, true, fmt.Errorf("Jenkins number exponent is out of range")
		}
	}
	number, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, true, fmt.Errorf("invalid Jenkins number")
	}
	return number, true, nil
}

func jenkinsCompare(left, right interface{}) (int, error) {
	a, aNumber, err := jenkinsNumeric(left)
	if err != nil {
		return 0, err
	}
	b, bNumber, err := jenkinsNumeric(right)
	if err != nil {
		return 0, err
	}
	if aNumber && bNumber {
		return a.Cmp(b), nil
	}
	if l, ok := left.(string); ok {
		if r, ok := right.(string); ok {
			// Java String.compareTo compares UTF-16 code units, including pairs
			// for supplementary characters, rather than Unicode code points.
			a, b := utf16.Encode([]rune(l)), utf16.Encode([]rune(r))
			for i := 0; i < len(a) && i < len(b); i++ {
				if a[i] < b[i] {
					return -1, nil
				}
				if a[i] > b[i] {
					return 1, nil
				}
			}
			if len(a) < len(b) {
				return -1, nil
			}
			if len(a) > len(b) {
				return 1, nil
			}
			return 0, nil
		}
	}
	return 0, fmt.Errorf("Jenkins values are not comparable")
}

// matchJenkinsBranch implements the default case-insensitive Ant-style path glob.
func matchJenkinsBranch(pattern, branch string) (bool, error) {
	return matchJenkinsPath(pattern, branch, false)
}

func matchJenkinsPath(pattern, branch string, caseSensitive bool) (bool, error) {
	compiled, err := compileJenkinsPathPattern(pattern, caseSensitive)
	if err != nil {
		return false, err
	}
	return compiled.matches(branch), nil
}

type jenkinsPathPattern struct {
	patterns      []string
	segments      [][]uint16
	caseSensitive bool
}

func compileJenkinsPathPattern(pattern string, caseSensitive bool) (*jenkinsPathPattern, error) {
	patterns := jenkinsAntPathSegments(pattern)
	compiled := &jenkinsPathPattern{patterns: patterns, segments: make([][]uint16, len(patterns)), caseSensitive: caseSensitive}
	for index, part := range patterns {
		if part == "**" {
			continue
		}
		compiled.segments[index] = utf16.Encode([]rune(part))
	}
	return compiled, nil
}

// Ant discards empty path components but retains an absolute root component.
func jenkinsAntPathSegments(path string) []string {
	if runtime.GOOS == "windows" {
		path = strings.ReplaceAll(path, `\`, "/")
	}
	var root string
	if strings.HasPrefix(path, "/") {
		root = "/"
	} else if runtime.GOOS == "windows" && len(path) >= 3 && path[1] == ':' && path[2] == '/' {
		root, path = path[:3], path[3:]
	}
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
	if root != "" {
		parts = append([]string{root}, parts...)
	}
	return parts
}

// Ant wildcards operate on Java UTF-16 chars, and '*' also matches newlines.
func matchJenkinsAntSegment(pattern []uint16, value string, caseSensitive bool) bool {
	chars := utf16.Encode([]rune(value))
	pi, vi, star, retry := 0, 0, -1, 0
	for vi < len(chars) {
		if pi < len(pattern) && pattern[pi] != '*' && (pattern[pi] == '?' || pattern[pi] == chars[vi] || (!caseSensitive && unicode.ToUpper(rune(pattern[pi])) == unicode.ToUpper(rune(chars[vi])))) {
			pi++
			vi++
		} else if pi < len(pattern) && pattern[pi] == '*' {
			star, retry = pi, vi
			pi++
		} else if star >= 0 {
			retry++
			vi, pi = retry, star+1
		} else {
			return false
		}
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

func (compiled *jenkinsPathPattern) matches(branch string) bool {
	patterns, parts := compiled.patterns, jenkinsAntPathSegments(branch)
	type position struct{ pattern, part int }
	visited := make(map[position]bool)
	var match func(int, int) bool
	match = func(pi, bi int) bool {
		key := position{pi, bi}
		if visited[key] {
			return false
		}
		visited[key] = true
		if pi == len(patterns) {
			return bi == len(parts)
		}
		if patterns[pi] == "**" {
			if match(pi+1, bi) {
				return true
			}
			if bi < len(parts) {
				return match(pi, bi+1)
			}
			return false
		}
		if bi == len(parts) {
			return false
		}
		if !matchJenkinsAntSegment(compiled.segments[pi], parts[bi], compiled.caseSensitive) {
			return false
		}
		return match(pi+1, bi+1)
	}
	return match(0, 0)
}

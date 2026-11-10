package engine

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

// A NUL-containing key cannot originate from an OS environment variable.
const jenkinsBuildNumberContextKey = "\x00buildworld.currentBuild.number"

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
	case *ast.SelectorExpr:
		if root, ok := node.X.(*ast.Ident); ok && (root.Name == "params" || root.Name == "env") {
			return nil
		}
		if root, ok := node.X.(*ast.Ident); ok && root.Name == "currentBuild" && node.Sel.Name == "number" {
			return nil
		}
	case *ast.CallExpr:
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
		if ident, ok := node.Fun.(*ast.Ident); ok && (((ident.Name == "jenkinsBranch" || ident.Name == "jenkinsTag") && (len(node.Args) == 1 || len(node.Args) == 2)) || (ident.Name == "jenkinsEnvironment" && len(node.Args) == 2)) {
			for _, arg := range node.Args {
				if literal, ok := arg.(*ast.BasicLit); !ok || literal.Kind != token.STRING {
					return fmt.Errorf("Jenkins condition arguments must be literal strings")
				}
			}
			return nil
		}
	case *ast.UnaryExpr:
		if node.Op == token.NOT {
			return validateJenkinsExpr(node.X)
		}
		if node.Op == token.SUB {
			if literal, ok := node.X.(*ast.BasicLit); ok && literal.Kind == token.INT {
				return nil
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
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, json.Number:
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
			if ident.Name == "jenkinsBuildingTag" {
				return environment["TAG_NAME"] != "", nil
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
					if strings.TrimSpace(actual) == "" {
						actual = ""
					}
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
				value, found := environment[first]
				return found && value == second, nil
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

func jenkinsChangeRequestField(field string) bool {
	switch field {
	case "CHANGE_ID", "CHANGE_TARGET", "CHANGE_BRANCH", "CHANGE_FORK", "CHANGE_TITLE", "CHANGE_AUTHOR", "CHANGE_AUTHOR_DISPLAY_NAME", "CHANGE_AUTHOR_EMAIL":
		return true
	default:
		return false
	}
}

func compareJenkinsString(pattern, actual, comparator, fallback string) (bool, error) {
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
	patterns, parts := strings.Split(pattern, "/"), strings.Split(branch, "/")
	type position struct{ pattern, part int }
	visited := make(map[position]bool)
	var match func(int, int) (bool, error)
	match = func(pi, bi int) (bool, error) {
		key := position{pi, bi}
		if visited[key] {
			return false, nil
		}
		visited[key] = true
		if pi == len(patterns) {
			return bi == len(parts), nil
		}
		if patterns[pi] == "**" {
			if ok, err := match(pi+1, bi); ok || err != nil {
				return ok, err
			}
			if bi < len(parts) {
				return match(pi, bi+1)
			}
			return false, nil
		}
		if bi == len(parts) {
			return false, nil
		}
		var expression strings.Builder
		expression.WriteString("(?i)^")
		for _, char := range patterns[pi] {
			switch char {
			case '*':
				expression.WriteString(".*")
			case '?':
				expression.WriteString(".")
			default:
				expression.WriteString(regexp.QuoteMeta(string(char)))
			}
		}
		expression.WriteString("$")
		ok, err := regexp.MatchString(expression.String(), parts[bi])
		if err != nil || !ok {
			return false, err
		}
		return match(pi+1, bi+1)
	}
	return match(0, 0)
}

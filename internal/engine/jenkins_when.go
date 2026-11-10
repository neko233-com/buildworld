package engine

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

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
		if node.Kind == token.STRING {
			return nil
		}
	case *ast.SelectorExpr:
		if root, ok := node.X.(*ast.Ident); ok && (root.Name == "params" || root.Name == "env") {
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
	case *ast.BinaryExpr:
		if node.Op == token.LAND || node.Op == token.LOR || node.Op == token.EQL || node.Op == token.NEQ {
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
	case int:
		return v != 0, nil
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
		return strconv.Unquote(node.Value)
	case *ast.SelectorExpr:
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
		case bool, string, nil, float64, int:
			return v, nil
		}
		return nil, fmt.Errorf("unsupported Jenkins parameter %q type %T", node.Sel.Name, v)
	case *ast.CallExpr:
		if ident, ok := node.Fun.(*ast.Ident); ok {
			if ident.Name == "jenkinsBuildingTag" {
				return environment["TAG_NAME"] != "", nil
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
				}
				comparator := "GLOB"
				if len(node.Args) == 2 {
					comparator, _ = strconv.Unquote(node.Args[1].(*ast.BasicLit).Value)
				}
				switch strings.ToUpper(comparator) {
				case "EQUALS":
					return first == actual, nil
				case "REGEXP":
					return regexp.MatchString("^(?:"+first+")$", actual)
				default:
					return matchJenkinsBranch(first, actual)
				}
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
			return left == right, nil
		case token.NEQ:
			return left != right, nil
		}
	}
	return false, fmt.Errorf("unsupported Jenkins expression")
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

package engine

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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
		if node.Name == "true" || node.Name == "false" {
			return nil
		}
	case *ast.BasicLit:
		if node.Kind == token.STRING {
			return nil
		}
	case *ast.SelectorExpr:
		if root, ok := node.X.(*ast.Ident); ok && root.Name == "params" {
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
	if err := ValidateJenkinsWhen(condition); err != nil {
		return false, err
	}
	expr, err := parser.ParseExpr(condition)
	if err != nil {
		return false, err
	}
	value, err := evalJenkinsExpr(expr, params)
	if err != nil {
		return false, err
	}
	result, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("Jenkins when expression must evaluate to boolean")
	}
	return result, nil
}

func evalJenkinsExpr(expr ast.Expr, params map[string]interface{}) (interface{}, error) {
	switch node := expr.(type) {
	case *ast.ParenExpr:
		return evalJenkinsExpr(node.X, params)
	case *ast.Ident:
		return node.Name == "true", nil
	case *ast.BasicLit:
		return strconv.Unquote(node.Value)
	case *ast.SelectorExpr:
		v, found := params[node.Sel.Name]
		if !found {
			return false, fmt.Errorf("Jenkins parameter %q is missing", node.Sel.Name)
		}
		switch v.(type) {
		case bool, string:
			return v, nil
		}
		return nil, fmt.Errorf("Jenkins parameter %q must be boolean or string", node.Sel.Name)
	case *ast.UnaryExpr:
		v, err := evalJenkinsExpr(node.X, params)
		if err != nil {
			return nil, err
		}
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("Jenkins ! operand must be boolean")
		}
		return !b, nil
	case *ast.BinaryExpr:
		left, err := evalJenkinsExpr(node.X, params)
		if err != nil {
			return false, err
		}
		if node.Op == token.LAND || node.Op == token.LOR {
			b, ok := left.(bool)
			if !ok {
				return nil, fmt.Errorf("Jenkins logical operand must be boolean")
			}
			if node.Op == token.LAND && !b {
				return false, nil
			}
			if node.Op == token.LOR && b {
				return true, nil
			}
		}
		right, err := evalJenkinsExpr(node.Y, params)
		if err != nil {
			return false, err
		}
		switch node.Op {
		case token.LAND:
			b, ok := right.(bool)
			if !ok {
				return nil, fmt.Errorf("Jenkins logical operand must be boolean")
			}
			return b, nil
		case token.LOR:
			b, ok := right.(bool)
			if !ok {
				return nil, fmt.Errorf("Jenkins logical operand must be boolean")
			}
			return b, nil
		case token.EQL:
			return left == right, nil
		case token.NEQ:
			return left != right, nil
		}
	}
	return false, fmt.Errorf("unsupported Jenkins expression")
}

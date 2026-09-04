package symbolic

import (
	"fmt"
	"strings"

	"github.com/nkosikhumalo/axiom/internal/ast"
)

// Expression is a symbolic representation of a value or operation.
// It can emit itself as a valid SMT-LIB2 string via ToSMT().
type Expression struct {
	Kind  string // "var" | "literal" | "binop" | "unop" | "unknown"
	Value string // variable name, literal value, or operator symbol
	Left  *Expression
	Right *Expression
}

// ToSMT renders the expression as an SMT-LIB2 string.
func (e *Expression) ToSMT() string {
	if e == nil {
		return "0"
	}
	switch e.Kind {
	case "var", "literal":
		return e.Value
	case "binop":
		op := normOp(e.Value)
		return fmt.Sprintf("(%s %s %s)", op, e.Left.ToSMT(), e.Right.ToSMT())
	case "unop":
		return fmt.Sprintf("(%s %s)", e.Value, e.Left.ToSMT())
	default:
		return e.Value
	}
}

// BuildExpression converts an AST node into a symbolic expression.
func BuildExpression(node *ast.Node) *Expression {
	if node == nil {
		return nil
	}

	// Strip parentheses wrappers
	if node.Type == "parenthesized_expression" && len(node.Children) > 0 {
		for _, c := range node.Children {
			if c.Type != "(" && c.Type != ")" {
				return BuildExpression(c)
			}
		}
	}

	switch node.Type {
	case "identifier", "field_identifier", "name",
		"variable_name": // PHP: $varName
		// Strip PHP's leading $ sigil so the name is a valid SMT-LIB2 identifier
		name := strings.TrimPrefix(node.Content, "$")
		return &Expression{Kind: "var", Value: name}

	case "int_literal", "integer_literal", "number_literal",
		"decimal_integer_literal", "float_literal":
		return &Expression{Kind: "literal", Value: node.Content}

	case "binary_expression", "binary_operator":
		return buildBinary(node)

	case "unary_expression":
		return buildUnary(node)

	case "call_expression":
		// Treat function calls as opaque symbolic variables for now
		return &Expression{Kind: "var", Value: sanitizeIdent(node.Content)}

	default:
		// Fallback: if it has a simple text value, treat as literal
		if strings.TrimSpace(node.Content) != "" {
			return &Expression{Kind: "literal", Value: node.Content}
		}
		return nil
	}
}

func buildBinary(node *ast.Node) *Expression {
	expr := &Expression{Kind: "binop"}
	for _, child := range node.Children {
		switch child.Type {
		// Operator tokens vary by language grammar
		case "+", "-", "*", "/", "%", "==", "!=", "<", ">", "<=", ">=", "&&", "||",
			"and", "or":
			expr.Value = child.Content
		default:
			e := BuildExpression(child)
			if e == nil {
				continue
			}
			if expr.Left == nil {
				expr.Left = e
			} else {
				expr.Right = e
			}
		}
	}
	if expr.Value == "" {
		expr.Value = "+"
	}
	return expr
}

func buildUnary(node *ast.Node) *Expression {
	expr := &Expression{Kind: "unop"}
	for _, child := range node.Children {
		switch child.Type {
		case "-", "!", "not":
			expr.Value = child.Content
		default:
			expr.Left = BuildExpression(child)
		}
	}
	return expr
}

// normOp maps source operators to SMT-LIB2 operators.
func normOp(op string) string {
	switch op {
	case "==":
		return "="
	case "!=":
		return "distinct"
	case "&&", "and":
		return "and"
	case "||", "or":
		return "or"
	case "!":
		return "not"
	default:
		return op // +, -, *, /, <, >, <=, >= are the same in SMT-LIB2
	}
}

// sanitizeIdent strips characters unsafe for SMT-LIB2 identifiers.
func sanitizeIdent(s string) string {
	s = strings.ReplaceAll(s, "(", "_")
	s = strings.ReplaceAll(s, ")", "_")
	s = strings.ReplaceAll(s, " ", "_")
	return s
}

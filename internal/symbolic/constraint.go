package symbolic

import (
	"fmt"
	"strings"

	"github.com/nkosikhumalo/axiom/internal/ast"
)

// PathConstraint represents one execution path through a function.
// It holds the conjunction of branch conditions that lead to a return value.
type PathConstraint struct {
	// Conditions is the AND of all branch decisions on this path.
	Conditions []string
	// ReturnExpr is the SMT-LIB2 expression this path returns.
	ReturnExpr string
	// Line is the source line of the return statement.
	Line uint32
}

// ToSMT renders this path as an SMT-LIB2 `(=> conditions return)` implication.
func (pc PathConstraint) ToSMT() string {
	if len(pc.Conditions) == 0 {
		return pc.ReturnExpr
	}
	cond := conjoin(pc.Conditions)
	return fmt.Sprintf("(ite %s %s %%ELSE%%)", cond, pc.ReturnExpr)
}

// ExtractPaths walks an AST root and returns all execution paths with their
// return expressions. The result is ordered from most-specific (deepest nesting)
// to default (unconditional fallthrough).
func ExtractPaths(root *ast.Node) []PathConstraint {
	// Find the first function body
	body := findFunctionBody(root)
	if body == nil {
		body = root
	}

	var paths []PathConstraint
	collectPaths(body, nil, &paths)
	return paths
}

// BuildFunctionSMT encodes all paths of a function into a single nested SMT-LIB2
// ite (if-then-else) expression representing the function's return value.
func BuildFunctionSMT(paths []PathConstraint) string {
	if len(paths) == 0 {
		return "0"
	}
	return nestITE(paths, 0)
}

// --- internal helpers ---

func nestITE(paths []PathConstraint, idx int) string {
	if idx >= len(paths) {
		return "0" // implicit return 0 as fallback
	}

	p := paths[idx]

	if len(p.Conditions) == 0 {
		// Unconditional / default path — terminal node
		return p.ReturnExpr
	}

	cond := conjoin(p.Conditions)
	thenBranch := p.ReturnExpr
	elseBranch := nestITE(paths, idx+1)

	return fmt.Sprintf("(ite %s %s %s)", cond, thenBranch, elseBranch)
}

func conjoin(conds []string) string {
	if len(conds) == 1 {
		return conds[0]
	}
	return fmt.Sprintf("(and %s)", strings.Join(conds, " "))
}

// collectPaths recursively walks the AST, accumulating branch conditions.
func collectPaths(node *ast.Node, activeConds []string, out *[]PathConstraint) {
	if node == nil {
		return
	}

	switch node.Type {
	case "if_statement":
		collectIfStatement(node, activeConds, out)
		return // children handled inside

	case "return_statement":
		retExpr := extractReturnExpr(node)
		*out = append(*out, PathConstraint{
			Conditions: copyConds(activeConds),
			ReturnExpr: retExpr,
			Line:       node.StartLine,
		})
		return
	}

	for _, child := range node.Children {
		collectPaths(child, activeConds, out)
	}
}

func collectIfStatement(node *ast.Node, activeConds []string, out *[]PathConstraint) {
	condNode := ast.ChildByField(node, "condition")
	if condNode == nil {
		// Fallback: look for parenthesized_expression child (C++ style)
		for _, c := range node.Children {
			if c.Type == "parenthesized_expression" {
				condNode = c
				break
			}
		}
	}

	var condSMT string
	if condNode != nil {
		inner := unwrapCondition(condNode)
		condSMT = buildConditionSMT(inner)
	} else {
		condSMT = "true"
	}

	// Then branch
	thenNode := ast.ChildByField(node, "consequence")
	if thenNode == nil {
		thenNode = ast.ChildByField(node, "body")
	}
	if thenNode != nil {
		thenConds := append(copyConds(activeConds), condSMT)
		collectPaths(thenNode, thenConds, out)
	}

	// Else branch
	elseNode := ast.ChildByField(node, "alternative")
	if elseNode != nil {
		negCond := fmt.Sprintf("(not %s)", condSMT)
		elseConds := append(copyConds(activeConds), negCond)
		// The else child may be another if_statement or a block
		for _, c := range elseNode.Children {
			if c.Type == "if_statement" {
				collectIfStatement(c, elseConds, out)
				return
			}
		}
		collectPaths(elseNode, elseConds, out)
	}
}

func extractReturnExpr(node *ast.Node) string {
	for _, c := range node.Children {
		if c.Type == "return" || c.Type == ";" {
			continue
		}
		// Go return_statement wraps the value in an expression_list
		if c.Type == "expression_list" {
			for _, inner := range c.Children {
				expr := BuildExpression(inner)
				if expr != nil {
					return expr.ToSMT()
				}
			}
			continue
		}
		expr := BuildExpression(c)
		if expr != nil {
			return expr.ToSMT()
		}
	}
	return "0"
}

// buildConditionSMT converts a condition node to a valid SMT-LIB2 boolean expression.
func buildConditionSMT(node *ast.Node) string {
	if node == nil {
		return "true"
	}

	switch node.Type {
	case "binary_expression":
		// Try field-named children first (C++ tree-sitter uses left/operator/right fields)
		leftNode := ast.ChildByField(node, "left")
		opNode := ast.ChildByField(node, "operator")
		rightNode := ast.ChildByField(node, "right")

		if leftNode != nil && opNode != nil && rightNode != nil {
			left := BuildExpression(leftNode)
			right := BuildExpression(rightNode)
			if left != nil && right != nil {
				return fmt.Sprintf("(%s %s %s)", normOp(opNode.Content), left.ToSMT(), right.ToSMT())
			}
		}

		// Fallback: walk children positionally (Go tree-sitter style)
		var left, right *Expression
		var op string
		for _, c := range node.Children {
			switch c.Type {
			case "==", "!=", "<", ">", "<=", ">=", "&&", "||", "and", "or":
				op = c.Content
			default:
				e := BuildExpression(c)
				if e == nil {
					continue
				}
				if left == nil {
					left = e
				} else {
					right = e
				}
			}
		}
		if op != "" && left != nil && right != nil {
			return fmt.Sprintf("(%s %s %s)", normOp(op), left.ToSMT(), right.ToSMT())
		}

	case "parenthesized_expression":
		return buildConditionSMT(stripParens(node))
	case "condition_clause":
		return buildConditionSMT(unwrapCondition(node))
	}

	expr := BuildExpression(node)
	if expr != nil {
		return expr.ToSMT()
	}
	return "true"
}

func findFunctionBody(root *ast.Node) *ast.Node {
	// Go: function_declaration -> block
	// C++: function_definition -> compound_statement
	// Java: method_declaration -> block
	for _, t := range []string{"block", "compound_statement", "function_body"} {
		if n := ast.FindFirst(root, t); n != nil {
			return n
		}
	}
	return nil
}

func stripParens(node *ast.Node) *ast.Node {
	if node.Type == "parenthesized_expression" {
		for _, c := range node.Children {
			if c.Type != "(" && c.Type != ")" {
				return c
			}
		}
	}
	return node
}

// unwrapCondition handles both Go-style (parenthesized_expression wrapping binary_expression)
// and C++-style (condition_clause with a "value" field containing the binary_expression).
func unwrapCondition(node *ast.Node) *ast.Node {
	switch node.Type {
	case "condition_clause":
		// C++: condition_clause -> value -> binary_expression
		if v := ast.ChildByField(node, "value"); v != nil {
			return v
		}
		// fallback: first non-paren child
		for _, c := range node.Children {
			if c.Type != "(" && c.Type != ")" {
				return c
			}
		}
	case "parenthesized_expression":
		for _, c := range node.Children {
			if c.Type != "(" && c.Type != ")" {
				return c
			}
		}
	}
	return node
}

func copyConds(src []string) []string {
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

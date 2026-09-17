package symbolic

import (
	"fmt"
	"regexp"
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
	collectPaths(body, nil, nil, &paths)
	return paths
}

// ExtractPathsForFunction extracts paths from the named function or method.
// It returns no paths when the symbol cannot be found, allowing callers to
// reject ambiguous or misspelled entry points instead of analyzing a random
// function in the file.
func ExtractPathsForFunction(root *ast.Node, name string) []PathConstraint {
	function := findFunction(root, name)
	if function == nil {
		return nil
	}
	body := ast.ChildByField(function, "body")
	if body == nil {
		body = ast.ChildByField(function, "block")
	}
	if body == nil {
		body = findFunctionBody(function)
	}
	if body == nil {
		return nil
	}
	var paths []PathConstraint
	collectPaths(body, nil, nil, &paths)
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
func collectPaths(node *ast.Node, activeConds []string, env map[string]*Expression, out *[]PathConstraint) bool {
	if node == nil {
		return false
	}
	if env == nil {
		env = map[string]*Expression{}
	}

	if node.Type == "block" || node.Type == "compound_statement" || node.Type == "function_body" {
		for _, child := range node.Children {
			if collectPaths(child, activeConds, env, out) {
				return true
			}
		}
		return false
	}

	switch node.Type {
	case "if_statement":
		return collectIfStatement(node, activeConds, env, out)
	case "expression_switch_statement", "switch_statement":
		return collectSwitchStatement(node, activeConds, env, out)

	case "return_statement":
		retExpr := extractReturnExpr(node)
		retExpr = substituteSMT(retExpr, env)
		*out = append(*out, PathConstraint{
			Conditions: copyConds(activeConds),
			ReturnExpr: retExpr,
			Line:       node.StartLine,
		})
		return true
	}

	if applyAssignment(node, env) {
		return false
	}
	return false
}

func collectIfStatement(node *ast.Node, activeConds []string, env map[string]*Expression, out *[]PathConstraint) bool {
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
		condSMT = substituteSMT(buildConditionSMT(inner), env)
	} else {
		condSMT = "true"
	}

	// Then branch
	thenNode := ast.ChildByField(node, "consequence")
	if thenNode == nil {
		thenNode = ast.ChildByField(node, "body")
	}
	thenTerminates := false
	if thenNode != nil {
		thenConds := append(copyConds(activeConds), condSMT)
		thenTerminates = collectPaths(thenNode, thenConds, copyEnv(env), out)
	}

	// Else branch
	elseNode := ast.ChildByField(node, "alternative")
	if elseNode != nil {
		negCond := fmt.Sprintf("(not %s)", condSMT)
		elseConds := append(copyConds(activeConds), negCond)
		// The else child may be another if_statement or a block
		for _, c := range elseNode.Children {
			if c.Type == "if_statement" {
				return thenTerminates && collectIfStatement(c, elseConds, copyEnv(env), out)
			}
		}
		return thenTerminates && collectPaths(elseNode, elseConds, copyEnv(env), out)
	}
	return false
}

func applyAssignment(node *ast.Node, env map[string]*Expression) bool {
	if node.Type != "expression_statement" && node.Type != "assignment_statement" && node.Type != "short_var_declaration" {
		return false
	}
	assignment := ast.ChildByField(node, "expression")
	if assignment == nil {
		assignment = node
	}
	if assignment.Type != "assignment_expression" && assignment.Type != "short_var_declaration" {
		for _, child := range node.Children {
			if child.Type == "assignment_expression" || child.Type == "short_var_declaration" {
				assignment = child
				break
			}
		}
	}
	if assignment == nil {
		return false
	}
	left := ast.ChildByField(assignment, "left")
	right := ast.ChildByField(assignment, "right")
	if left == nil || right == nil {
		return false
	}
	if left.Type == "expression_list" && len(left.Children) == 1 {
		left = left.Children[0]
	}
	if right.Type == "expression_list" && len(right.Children) == 1 {
		right = right.Children[0]
	}
	if left.Type != "identifier" {
		return false
	}
	value := BuildExpression(right)
	if value == nil {
		return false
	}
	env[left.Content] = substituteExpression(value, env)
	return true
}

func collectSwitchStatement(node *ast.Node, activeConds []string, env map[string]*Expression, out *[]PathConstraint) bool {
	valueNode := ast.ChildByField(node, "value")
	if valueNode == nil {
		return false
	}
	switchValue := BuildExpression(valueNode)
	if switchValue == nil {
		return false
	}
	var cases []*ast.Node
	for _, child := range node.Children {
		if child.Type == "expression_case" || child.Type == "case" || child.Type == "default_case" {
			cases = append(cases, child)
		}
	}
	if len(cases) == 0 {
		return false
	}
	var caseConditions []string
	hasDefault := false
	allTerminate := true
	for _, caseNode := range cases {
		caseCond := ""
		if caseNode.Type == "default_case" {
			hasDefault = true
			if len(caseConditions) > 0 {
				caseCond = fmt.Sprintf("(not (or %s))", strings.Join(caseConditions, " "))
			} else {
				caseCond = "true"
			}
		} else {
			value := ast.ChildByField(caseNode, "value")
			if value == nil {
				continue
			}
			var values []*ast.Node
			if value.Type == "expression_list" {
				values = value.Children
			} else {
				values = []*ast.Node{value}
			}
			var alternatives []string
			for _, item := range values {
				if expr := BuildExpression(item); expr != nil {
					alternatives = append(alternatives, fmt.Sprintf("(= %s %s)", switchValue.ToSMT(), expr.ToSMT()))
				}
			}
			if len(alternatives) == 1 {
				caseCond = alternatives[0]
			} else if len(alternatives) > 1 {
				caseCond = fmt.Sprintf("(or %s)", strings.Join(alternatives, " "))
			}
			if caseCond != "" {
				caseConditions = append(caseConditions, caseCond)
			}
		}
		if caseCond == "" {
			continue
		}
		caseEnv := copyEnv(env)
		caseConds := append(copyConds(activeConds), substituteSMT(caseCond, env))
		caseTerminates := false
		for _, child := range caseNode.Children {
			if child.Type != "case" && child.Type != "default" && child.Type != ":" && child.Type != "expression_list" {
				if collectPaths(child, caseConds, caseEnv, out) {
					caseTerminates = true
					break
				}
			}
		}
		allTerminate = allTerminate && caseTerminates
	}
	return hasDefault && allTerminate
}

func copyEnv(src map[string]*Expression) map[string]*Expression {
	dst := make(map[string]*Expression, len(src))
	for name, expr := range src {
		dst[name] = expr
	}
	return dst
}

func substituteExpression(expr *Expression, env map[string]*Expression) *Expression {
	if expr == nil {
		return nil
	}
	if expr.Kind == "var" {
		if replacement, ok := env[expr.Value]; ok {
			return replacement
		}
	}
	result := *expr
	result.Left = substituteExpression(expr.Left, env)
	result.Right = substituteExpression(expr.Right, env)
	return &result
}

func substituteSMT(s string, env map[string]*Expression) string {
	if len(env) == 0 {
		return s
	}
	for name, expr := range env {
		pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
		s = pattern.ReplaceAllString(s, expr.ToSMT())
	}
	return s
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

func findFunction(root *ast.Node, name string) *ast.Node {
	for _, nodeType := range []string{"function_declaration", "function_definition", "method_declaration", "constructor_declaration"} {
		for _, node := range ast.FindAll(root, nodeType) {
			if functionName(node) == name {
				return node
			}
		}
	}
	return nil
}

func functionName(node *ast.Node) string {
	if name := ast.ChildByField(node, "name"); name != nil {
		return name.Content
	}
	if declarator := ast.ChildByField(node, "declarator"); declarator != nil {
		for _, nodeType := range []string{"identifier", "field_identifier", "name"} {
			if name := ast.FindFirst(declarator, nodeType); name != nil {
				return name.Content
			}
		}
	}
	return ""
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

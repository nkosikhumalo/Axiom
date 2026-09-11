package solver

import (
	"fmt"
	"strings"
)

// Query holds a complete SMT-LIB2 program ready to be fed to Z3.
type Query struct {
	Variables []Variable
	Legacy    string // SMT-LIB2 expression for legacy function output
	Modern    string // SMT-LIB2 expression for modern function output
}

// Variable is a symbolic integer parameter shared by both functions.
type Variable struct {
	Name    string
	SMTType string // "Int", "Bool", etc.
}

// Build generates the SMT-LIB2 query that asks Z3:
// "Does there exist an input where legacy(x) != modern(x)?"
// UNSAT = no such input exists = rewrite is equivalent.
// SAT   = counterexample found = logic drift detected.
func Build(q Query) string {
	var b strings.Builder

	b.WriteString("; Axiom equivalence check\n")
	b.WriteString("; UNSAT = equivalent | SAT = logic drift detected\n\n")
	b.WriteString("(set-option :timeout 5000)\n\n")

	// Declare shared symbolic input variables
	for _, v := range q.Variables {
		fmt.Fprintf(&b, "(declare-const %s %s)\n", v.Name, v.SMTType)
	}

	b.WriteString("\n; Define function outputs as symbolic expressions\n")
	legacyType := inferSMTType(q.Legacy)
	modernType := inferSMTType(q.Modern)
	if legacyType != modernType {
		legacyType = "Int"
		modernType = "Int"
	}
	fmt.Fprintf(&b, "(define-fun legacy_out () %s\n  %s\n)\n\n", legacyType, q.Legacy)
	fmt.Fprintf(&b, "(define-fun modern_out () %s\n  %s\n)\n\n", modernType, q.Modern)

	// Assert negation of equivalence — Z3 tries to find a counterexample
	b.WriteString("; Assert: legacy != modern (negation of equivalence)\n")
	b.WriteString("(assert (not (= legacy_out modern_out)))\n\n")
	b.WriteString("(check-sat)\n")
	b.WriteString("(get-model)\n")

	return b.String()
}

func inferSMTType(expr string) string {
	tokens := tokenize(expr)
	if len(tokens) == 0 {
		return "Int"
	}
	if tokens[0] == "true" || tokens[0] == "false" {
		return "Bool"
	}
	switch tokens[0] {
	case "=", "distinct", "<", ">", "<=", ">=", "and", "or", "not":
		return "Bool"
	case "ite":
		if len(tokens) > 2 && (tokens[2] == "true" || tokens[2] == "false") {
			return "Bool"
		}
	}
	return "Int"
}

// ExtractVars scans both SMT expressions and returns all unique symbolic
// variable names (single-word tokens that are not SMT keywords or numbers).
func ExtractVars(legacy, modern string) []Variable {
	seen := map[string]bool{}
	boolVars := inferBoolVars(legacy + " " + modern)
	var vars []Variable

	for _, token := range tokenize(legacy + " " + modern) {
		if !seen[token] && isUserVar(token) {
			seen[token] = true
			smtType := "Int"
			if boolVars[token] {
				smtType = "Bool"
			}
			vars = append(vars, Variable{Name: token, SMTType: smtType})
		}
	}

	return vars
}

func inferBoolVars(s string) map[string]bool {
	boolVars := map[string]bool{}
	tokens := tokenize(s)
	for index, token := range tokens {
		if index == 0 {
			continue
		}
		previous := tokens[index-1]
		if previous == "not" || previous == "and" || previous == "or" || previous == "ite" {
			if isUserVar(token) {
				boolVars[token] = true
			}
		}
	}
	return boolVars
}

// smtKeywords are SMT-LIB2 reserved tokens we don't want to declare as variables.
var smtKeywords = map[string]bool{
	"ite": true, "and": true, "or": true, "not": true,
	"=": true, "distinct": true, "true": true, "false": true,
	"+": true, "-": true, "*": true, "/": true,
	"<": true, ">": true, "<=": true, ">=": true,
	"0": true, "Int": true, "Bool": true,
}

func tokenize(s string) []string {
	s = strings.NewReplacer("(", " ", ")", " ").Replace(s)
	raw := strings.Fields(s)
	var tokens []string
	for _, t := range raw {
		tokens = append(tokens, strings.TrimSpace(t))
	}
	return tokens
}

func isUserVar(token string) bool {
	if smtKeywords[token] {
		return false
	}
	// Reject pure numbers
	allDigit := true
	for _, r := range token {
		if r < '0' || r > '9' {
			allDigit = false
			break
		}
	}
	if allDigit {
		return false
	}
	// Must start with a letter or underscore
	if len(token) == 0 {
		return false
	}
	first := token[0]
	return (first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_'
}

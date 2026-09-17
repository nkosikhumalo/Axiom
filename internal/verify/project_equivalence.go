package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nkosikhumalo/axiom/internal/ast"
	"github.com/nkosikhumalo/axiom/internal/project"
	"github.com/nkosikhumalo/axiom/internal/solver"
	"github.com/nkosikhumalo/axiom/internal/symbolic"
)

const (
	StatusProvenEquivalent      = "proven_equivalent"
	StatusEquivalentAssumptions = "equivalent_under_assumptions"
	StatusNotEquivalent         = "not_equivalent"
	StatusCouldNotProve         = "could_not_prove"
)

type Outcome struct {
	Status       string               `json:"status"`
	Reason       string               `json:"reason"`
	LegacyEntry  string               `json:"legacy_entry"`
	ModernEntry  string               `json:"modern_entry"`
	Countermodel string               `json:"countermodel,omitempty"`
	Assumptions  []project.Assumption `json:"assumptions,omitempty"`
}

func VerifyProjects(legacyRoot, modernRoot, legacyEntry, modernEntry string) (*Outcome, error) {
	legacyIndex, err := project.Discover(legacyRoot)
	if err != nil {
		return nil, fmt.Errorf("discover legacy project: %w", err)
	}
	modernIndex, err := project.Discover(modernRoot)
	if err != nil {
		return nil, fmt.Errorf("discover modern project: %w", err)
	}
	legacySymbol, err := project.ResolveEntry(legacyIndex, legacyEntry)
	if err != nil {
		return nil, fmt.Errorf("legacy entry: %w", err)
	}
	modernSymbol, err := project.ResolveEntry(modernIndex, modernEntry)
	if err != nil {
		return nil, fmt.Errorf("modern entry: %w", err)
	}
	outcome := &Outcome{LegacyEntry: legacyEntry, ModernEntry: modernEntry}
	if !compatibleSignatures(legacySymbol, modernSymbol) {
		outcome.Status = StatusNotEquivalent
		outcome.Reason = "entry signatures are incompatible"
		return outcome, nil
	}
	legacyReachable, err := project.Reachable(legacyIndex, legacyEntry)
	if err != nil {
		return nil, fmt.Errorf("legacy reachable graph: %w", err)
	}
	modernReachable, err := project.Reachable(modernIndex, modernEntry)
	if err != nil {
		return nil, fmt.Errorf("modern reachable graph: %w", err)
	}
	outcome.Assumptions = append(outcome.Assumptions, legacyReachable.Assumptions...)
	outcome.Assumptions = append(outcome.Assumptions, modernReachable.Assumptions...)
	if reason, blocked := blockedByAnalysis(legacyReachable, modernReachable); blocked {
		outcome.Status = StatusCouldNotProve
		outcome.Reason = reason
		return outcome, nil
	}
	legacySMT, err := expandEntrySMT(legacyRoot, legacyReachable, legacySymbol, nil)
	if err != nil {
		outcome.Status = StatusCouldNotProve
		outcome.Reason = err.Error()
		return outcome, nil
	}
	modernSMT, err := expandEntrySMT(modernRoot, modernReachable, modernSymbol, nil)
	if err != nil {
		outcome.Status = StatusCouldNotProve
		outcome.Reason = err.Error()
		return outcome, nil
	}
	result, err := solver.RunZ3(solver.Build(solver.Query{Variables: solver.ExtractVars(legacySMT, modernSMT), Legacy: legacySMT, Modern: modernSMT}))
	if err != nil {
		return nil, fmt.Errorf("solve project equivalence: %w", err)
	}
	if result.Sat {
		outcome.Status = StatusNotEquivalent
		outcome.Reason = "Z3 found an input where the entry outputs differ"
		outcome.Countermodel = result.Model
		return outcome, nil
	}
	if len(outcome.Assumptions) > 0 {
		outcome.Status = StatusEquivalentAssumptions
		outcome.Reason = "outputs match under recorded assumptions"
		return outcome, nil
	}
	outcome.Status = StatusProvenEquivalent
	outcome.Reason = "Z3 found no counterexample for the selected entries"
	return outcome, nil
}

func compatibleSignatures(legacy, modern project.Symbol) bool {
	if len(legacy.Parameters) != len(modern.Parameters) {
		return false
	}
	for index := range legacy.Parameters {
		if legacy.Parameters[index].Type != modern.Parameters[index].Type {
			return false
		}
	}
	return legacy.ReturnType == modern.ReturnType
}

func blockedByAnalysis(indexes ...*project.Index) (string, bool) {
	for _, index := range indexes {
		for _, summary := range index.Summaries {
			switch summary.Classification {
			case project.ClassificationUnsupported:
				return "unsupported behavior affects a reachable dependency: " + summary.Symbol, true
			case project.ClassificationStateful, project.ClassificationExternal:
				return "reachable stateful or external dependency is not symbolically summarized: " + summary.Symbol, true
			}
		}
		for _, edge := range index.Edges {
			if edge.Status == "ambiguous" {
				return "ambiguous reachable dependency: " + edge.Call, true
			}
		}
	}
	return "", false
}

func expandEntrySMT(root string, index *project.Index, symbol project.Symbol, visiting map[string]bool) (string, error) {
	identity := identityOf(symbol)
	if visiting == nil {
		visiting = map[string]bool{}
	}
	if visiting[identity] {
		return "", fmt.Errorf("cyclic pure dependency at %s", identity)
	}
	visiting[identity] = true
	defer delete(visiting, identity)
	sourcePath := filepath.Join(root, symbol.File)
	if _, err := os.Stat(sourcePath); err != nil {
		return "", fmt.Errorf("read entry file: %w", err)
	}
	parsed, err := ast.ParseFile(sourcePath)
	if err != nil {
		return "", err
	}
	paths := symbolic.ExtractPathsForFunction(ast.Walk(parsed), symbol.Name)
	if len(paths) == 0 {
		return "", fmt.Errorf("no return paths found for %s", symbol.QualifiedName)
	}
	result := symbolic.BuildFunctionSMT(paths)
	for _, invocation := range symbol.Invocations {
		target, ok := pureTarget(index, symbol, invocation)
		if !ok {
			continue
		}
		callee, err := expandEntrySMT(root, index, target, visiting)
		if err != nil {
			return "", err
		}
		callee, err = bindArguments(callee, target, invocation.Arguments)
		if err != nil {
			return "", err
		}
		result = replaceCall(result, invocation.Content, callee)
	}
	return result, nil
}

func pureTarget(index *project.Index, source project.Symbol, invocation project.Invocation) (project.Symbol, bool) {
	for _, edge := range index.Edges {
		if edge.From != identityOf(source) || edge.Call != invocation.Name || edge.Status != "resolved" {
			continue
		}
		for _, file := range index.Files {
			for _, target := range file.Symbols {
				if identityOf(target) != edge.To {
					continue
				}
				for _, summary := range index.Summaries {
					if summary.Symbol == edge.To && summary.Classification == project.ClassificationPure {
						return target, true
					}
				}
			}
		}
	}
	return project.Symbol{}, false
}

func bindArguments(expression string, symbol project.Symbol, arguments []string) (string, error) {
	if len(symbol.Parameters) != len(arguments) {
		return "", fmt.Errorf("argument count mismatch calling %s", symbol.Signature)
	}
	for index, parameter := range symbol.Parameters {
		value, err := simpleArgument(arguments[index])
		if err != nil {
			return "", fmt.Errorf("bind %s: %w", symbol.Signature, err)
		}
		expression = regexp.MustCompile(`\b`+regexp.QuoteMeta(parameter.Name)+`\b`).ReplaceAllString(expression, value)
	}
	return expression, nil
}

func simpleArgument(argument string) (string, error) {
	argument = strings.TrimSpace(argument)
	if argument == "" {
		return "", fmt.Errorf("empty argument")
	}
	if strings.HasPrefix(argument, "\"") || strings.HasPrefix(argument, "'") {
		return "", fmt.Errorf("string arguments are not modeled")
	}
	for _, character := range argument {
		if !(character == '_' || character == '.' || character == '-' || character == '+' || character == '*' || character == '/' || character == ' ' || character >= '0' && character <= '9' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z') {
			return "", fmt.Errorf("unsupported argument %q", argument)
		}
	}
	return argument, nil
}

func replaceCall(expression, content, replacement string) string {
	call := strings.NewReplacer("(", "_", ")", "_", " ", "_", ".", "_").Replace(content)
	if call == "" {
		return expression
	}
	return regexp.MustCompile(`\b`+regexp.QuoteMeta(call)+`\b`).ReplaceAllString(expression, replacement)
}

func identityOf(symbol project.Symbol) string {
	if symbol.Signature != "" {
		return symbol.Signature
	}
	return symbol.QualifiedName
}

package refactor

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/nkosikhumalo/axiom/internal/project"
)

// Plan is a reviewable restructuring proposal. It contains no source mutation.
type Plan struct {
	Language string
	Entry    string
	Actions  []string
	Warnings []string
	Findings []Finding
}

// Finding is an evidence-backed refactoring recommendation for one symbol.
type Finding struct {
	Symbol         string
	File           string
	Line           uint32
	Severity       string
	Metrics        project.Metrics
	Reasons        []string
	Recommendation string
}

// BuildJavaPlan creates conservative Java restructuring suggestions from a project index.
func BuildJavaPlan(index *project.Index, entry string) Plan {
	plan := Plan{Language: "java", Entry: entry}
	javaFiles := 0
	summaries := map[string]project.Summary{}
	for _, summary := range index.Summaries {
		summaries[summary.Symbol] = summary
	}
	for _, file := range index.Files {
		if file.Language != "java" {
			continue
		}
		javaFiles++
		for _, symbol := range file.Symbols {
			if symbol.Kind == "method" || symbol.Kind == "constructor" {
				finding := analyzeSymbol(symbol, summaries[identity(symbol)])
				if len(finding.Reasons) > 0 {
					finding.File = file.Path
					finding.Recommendation = fmt.Sprintf("Refactor %s at %s:%d into focused methods or collaborators.", finding.Symbol, finding.File, finding.Line)
					plan.Findings = append(plan.Findings, finding)
					plan.Actions = append(plan.Actions, finding.Recommendation)
				} else {
					plan.Actions = append(plan.Actions, fmt.Sprintf("Review %s in %s for extraction into focused methods or collaborators.", symbol.Name, file.Path))
				}
			}
		}
	}
	if javaFiles == 0 {
		plan.Warnings = append(plan.Warnings, "No Java source files were found in the selected project.")
	}
	if len(plan.Actions) == 0 && javaFiles > 0 {
		plan.Actions = append(plan.Actions, "Inspect Java classes manually; no supported method declarations were indexed.")
	}
	plan.Warnings = append(plan.Warnings,
		"This plan does not modify source files.",
		"Compile and run characterization tests before applying any restructuring.",
		"Generate characterization tests for public behavior that is not already covered.",
		"Review reflection, inheritance, shared state, I/O, and external service behavior manually.")
	for _, assumption := range index.Assumptions {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("Unresolved dependency from %s: %s (%s).", assumption.Symbol, assumption.Call, assumption.Reason))
	}
	sort.Strings(plan.Actions)
	sort.Slice(plan.Findings, func(i, j int) bool {
		if plan.Findings[i].File == plan.Findings[j].File {
			return plan.Findings[i].Line < plan.Findings[j].Line
		}
		return plan.Findings[i].File < plan.Findings[j].File
	})
	return plan
}

func analyzeSymbol(symbol project.Symbol, summary project.Summary) Finding {
	finding := Finding{Symbol: identity(symbol), Line: symbol.StartLine, Metrics: symbol.Metrics, Severity: "medium"}
	if symbol.Metrics.LineCount >= 40 {
		finding.Reasons = append(finding.Reasons, fmt.Sprintf("method spans %d lines", symbol.Metrics.LineCount))
	}
	if symbol.Metrics.StatementCount >= 20 {
		finding.Reasons = append(finding.Reasons, fmt.Sprintf("method contains %d statements", symbol.Metrics.StatementCount))
	}
	if symbol.Metrics.MaxNesting >= 4 {
		finding.Reasons = append(finding.Reasons, fmt.Sprintf("control flow reaches %d nested levels", symbol.Metrics.MaxNesting))
	}
	if symbol.Metrics.ParameterCount >= 5 {
		finding.Reasons = append(finding.Reasons, fmt.Sprintf("method accepts %d parameters", symbol.Metrics.ParameterCount))
	}
	if symbol.Metrics.RepeatedConditions > 0 {
		finding.Reasons = append(finding.Reasons, fmt.Sprintf("repeats %d condition(s)", symbol.Metrics.RepeatedConditions))
	}
	for _, effect := range symbol.Effects {
		if effect == "mutates_state" {
			finding.Reasons = append(finding.Reasons, "mutates state")
		}
		if effect == "throws" {
			finding.Reasons = append(finding.Reasons, "throws exceptions")
		}
	}
	if summary.Classification == project.ClassificationExternal {
		finding.Reasons = append(finding.Reasons, "depends on an external or unresolved operation")
	}
	if summary.Classification == project.ClassificationUnsupported {
		finding.Reasons = append(finding.Reasons, "contains unsupported or ambiguous behavior")
		finding.Severity = "high"
	}
	for _, invocation := range symbol.Invocations {
		name := strings.ToLower(invocation.Name)
		if strings.Contains(name, "save") || strings.Contains(name, "write") || strings.Contains(name, "update") || strings.Contains(name, "delete") || strings.Contains(name, "log") || strings.Contains(name, "send") {
			finding.Reasons = append(finding.Reasons, "mixes side effects with business logic through "+invocation.Name)
		}
	}
	if len(finding.Reasons) > 0 {
		finding.Reasons = unique(finding.Reasons)
		finding.Recommendation = fmt.Sprintf("Refactor %s at %s:%d into focused methods or collaborators.", finding.Symbol, finding.File, finding.Line)
	}
	return finding
}

func identity(symbol project.Symbol) string {
	if symbol.Signature != "" {
		return symbol.Signature
	}
	return symbol.QualifiedName
}

func unique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

// WriteMarkdown writes a human-reviewable refactoring plan.
func (plan Plan) WriteMarkdown(path string) error {
	var builder strings.Builder
	builder.WriteString("# Axiom Refactoring Plan\n\n")
	builder.WriteString("This is a proposal. No source files were modified.\n\n")
	fmt.Fprintf(&builder, "- Language: `%s`\n- Entry: `%s`\n\n", plan.Language, plan.Entry)
	builder.WriteString("## Proposed Actions\n\n")
	for _, action := range plan.Actions {
		fmt.Fprintf(&builder, "- %s\n", action)
	}
	if len(plan.Findings) > 0 {
		builder.WriteString("\n## Evidence-Based Findings\n\n")
		for _, finding := range plan.Findings {
			fmt.Fprintf(&builder, "- `%s` at `%s:%d` [%s]: %s. Recommendation: %s\n", finding.Symbol, finding.File, finding.Line, finding.Severity, strings.Join(finding.Reasons, "; "), finding.Recommendation)
		}
	}
	builder.WriteString("\n## Warnings and Required Review\n\n")
	for _, warning := range plan.Warnings {
		fmt.Fprintf(&builder, "- %s\n", warning)
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		return fmt.Errorf("write refactoring plan: %w", err)
	}
	return nil
}

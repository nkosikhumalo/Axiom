package project

import "strings"

const (
	ClassificationPure        = "pure"
	ClassificationStateful    = "stateful"
	ClassificationExternal    = "external"
	ClassificationUnsupported = "unsupported"
)

// Summary describes the proof boundary for one discovered symbol.
type Summary struct {
	Symbol         string   `json:"symbol"`
	Classification string   `json:"classification"`
	ReturnType     string   `json:"return_type,omitempty"`
	Dependencies   []string `json:"dependencies,omitempty"`
	Reasons        []string `json:"reasons,omitempty"`
}

// Assumption records behavior Axiom could not derive from source.
type Assumption struct {
	Symbol   string `json:"symbol"`
	Call     string `json:"call"`
	Kind     string `json:"kind"`
	Reason   string `json:"reason"`
	Severity string `json:"severity"`
}

// AnalyzeSummaries classifies symbols conservatively and records assumptions for opaque calls.
func AnalyzeSummaries(index *Index) {
	summaries := make(map[string]Summary)
	for _, file := range index.Files {
		for _, symbol := range file.Symbols {
			key := qualifiedName(symbol)
			classification, reasons := initialClassification(symbol, index.Edges)
			summaries[key] = Summary{
				Symbol:         key,
				Classification: classification,
				ReturnType:     symbol.ReturnType,
				Reasons:        reasons,
			}
		}
	}

	for changed := true; changed; {
		changed = false
		for _, edge := range index.Edges {
			if edge.Status != "resolved" {
				continue
			}
			source, sourceOK := summaries[edge.From]
			target, targetOK := summaries[edge.To]
			if !sourceOK || !targetOK {
				continue
			}
			if promote(&source, target.Classification, "depends on "+target.Symbol) {
				summaries[edge.From] = source
				changed = true
			}
		}
	}

	index.Summaries = make([]Summary, 0, len(summaries))
	for _, summary := range summaries {
		for _, edge := range index.Edges {
			if edge.From != summary.Symbol {
				continue
			}
			if edge.Status == "resolved" && edge.To != "" {
				summary.Dependencies = append(summary.Dependencies, edge.To)
			}
		}
		index.Summaries = append(index.Summaries, summary)
	}
	sortSummaries(index.Summaries)

	index.Assumptions = make([]Assumption, 0)
	for _, edge := range index.Edges {
		if edge.Status == "resolved" {
			continue
		}
		severity := "warning"
		if edge.Status == "ambiguous" {
			severity = "error"
		}
		index.Assumptions = append(index.Assumptions, Assumption{
			Symbol:   edge.From,
			Call:     edge.Call,
			Kind:     edge.Status,
			Reason:   "implementation is not available as a resolved project symbol",
			Severity: severity,
		})
	}
}

func initialClassification(symbol Symbol, edges []Edge) (string, []string) {
	classification := ClassificationPure
	var reasons []string
	if symbol.Kind == "constructor" {
		classification = ClassificationStateful
		reasons = append(reasons, "constructor may initialize object state")
	}
	for _, edge := range edges {
		if edge.From != qualifiedName(symbol) {
			continue
		}
		if edge.Status == "external_or_unresolved" {
			classification = ClassificationExternal
			reasons = append(reasons, "calls unresolved or external symbol "+edge.Call)
		}
		if edge.Status == "ambiguous" {
			classification = ClassificationUnsupported
			reasons = append(reasons, "calls ambiguous symbol "+edge.Call)
		}
	}
	for _, invocation := range symbol.Invocations {
		name := strings.ToLower(invocation.Name)
		switch {
		case strings.Contains(name, "reflect"), strings.Contains(name, "invoke"), strings.Contains(name, "native"), strings.Contains(name, "eval"):
			classification = ClassificationUnsupported
			reasons = append(reasons, "uses dynamic or unsupported operation "+invocation.Name)
		case strings.Contains(name, "save"), strings.Contains(name, "write"), strings.Contains(name, "update"), strings.Contains(name, "delete"), strings.Contains(name, "insert"), strings.Contains(name, "persist"), strings.Contains(name, "send"), strings.Contains(name, "log"):
			if classification != ClassificationUnsupported {
				classification = ClassificationStateful
			}
			reasons = append(reasons, "may perform side effect through "+invocation.Name)
		}
	}
	for _, effect := range symbol.Effects {
		if effect == "throws" {
			reasons = append(reasons, "may throw an exception")
		}
		if effect == "mutates_state" && classification != ClassificationUnsupported {
			classification = ClassificationStateful
			reasons = append(reasons, "mutates state")
		}
	}
	return classification, uniqueStrings(reasons)
}

func promote(summary *Summary, target, reason string) bool {
	rank := map[string]int{ClassificationPure: 0, ClassificationStateful: 1, ClassificationExternal: 2, ClassificationUnsupported: 3}
	if rank[target] <= rank[summary.Classification] {
		return false
	}
	summary.Classification = target
	summary.Reasons = uniqueStrings(append(summary.Reasons, reason))
	return true
}

func sortSummaries(summaries []Summary) {
	for i := 1; i < len(summaries); i++ {
		for j := i; j > 0 && summaries[j].Symbol < summaries[j-1].Symbol; j-- {
			summaries[j], summaries[j-1] = summaries[j-1], summaries[j]
		}
	}
}

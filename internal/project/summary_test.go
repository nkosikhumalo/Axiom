package project

import "testing"

func TestAnalyzeSummariesClassifiesMethodsAndAssumptions(t *testing.T) {
	index := &Index{
		Files: []File{{Symbols: []Symbol{
			{QualifiedName: "pure", Signature: "pure()"},
			{QualifiedName: "writer", Signature: "writer()", Invocations: []Invocation{{Name: "save"}}},
			{QualifiedName: "dynamic", Signature: "dynamic()", Invocations: []Invocation{{Name: "reflectInvoke"}}},
			{QualifiedName: "mutator", Signature: "mutator()", Effects: []string{"mutates_state", "throws"}},
			{QualifiedName: "external", Signature: "external()"},
		}}},
		Edges: []Edge{{From: "external()", Call: "database", Status: "external_or_unresolved"}},
	}

	AnalyzeSummaries(index)
	classes := map[string]string{}
	for _, summary := range index.Summaries {
		classes[summary.Symbol] = summary.Classification
	}
	if classes["pure()"] != ClassificationPure {
		t.Fatalf("expected pure classification, got %#v", classes)
	}
	if classes["writer()"] != ClassificationStateful {
		t.Fatalf("expected stateful classification, got %#v", classes)
	}
	if classes["dynamic()"] != ClassificationUnsupported {
		t.Fatalf("expected unsupported classification, got %#v", classes)
	}
	if classes["mutator()"] != ClassificationStateful {
		t.Fatalf("expected mutation classification, got %#v", classes)
	}
	if classes["external()"] != ClassificationExternal || len(index.Assumptions) != 1 {
		t.Fatalf("expected external assumption, got summaries=%#v assumptions=%#v", index.Summaries, index.Assumptions)
	}
}

func TestAnalyzeSummariesPropagatesDependencyRisk(t *testing.T) {
	index := &Index{
		Files: []File{{Symbols: []Symbol{
			{QualifiedName: "entry", Signature: "entry()"},
			{QualifiedName: "repository", Signature: "repository()", Invocations: []Invocation{{Name: "save"}}},
		}}},
		Edges: []Edge{{From: "entry", To: "repository", Call: "repository", Status: "resolved"}},
	}

	AnalyzeSummaries(index)
	for _, summary := range index.Summaries {
		if summary.Symbol == "entry" && summary.Classification != ClassificationStateful {
			t.Fatalf("expected dependency risk to propagate, got %#v", summary)
		}
	}
}

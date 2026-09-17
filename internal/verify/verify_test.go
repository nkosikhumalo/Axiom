package verify

import (
	"os"
	"path/filepath"
	"testing"
)

func writeJavaProject(t *testing.T, source string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Discount.java"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestVerifyProjectsProvesEquivalentDirectEntries(t *testing.T) {
	legacy := writeJavaProject(t, `class Discount { public int calculate(int amount) { if (amount > 0) { return amount - 1; } return amount; } }`)
	modern := writeJavaProject(t, `class Discount { public int calculate(int amount) { if (amount > 0) { return amount - 1; } return amount; } }`)

	outcome, err := VerifyProjects(legacy, modern, "Discount.calculate(int)", "Discount.calculate(int)")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != StatusProvenEquivalent {
		t.Fatalf("expected proven equivalence, got %#v", outcome)
	}
}

func TestVerifyProjectsReportsCounterexample(t *testing.T) {
	legacy := writeJavaProject(t, `class Discount { public int calculate(int amount) { return amount; } }`)
	modern := writeJavaProject(t, `class Discount { public int calculate(int amount) { return amount + 1; } }`)

	outcome, err := VerifyProjects(legacy, modern, "Discount.calculate(int)", "Discount.calculate(int)")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != StatusNotEquivalent || outcome.Countermodel == "" {
		t.Fatalf("expected counterexample, got %#v", outcome)
	}
}

func TestVerifyProjectsRejectsIncompatibleSignatures(t *testing.T) {
	legacy := writeJavaProject(t, `class Discount { public int calculate(int amount) { return amount; } }`)
	modern := writeJavaProject(t, `class Discount { public long calculate(int amount) { return amount; } }`)

	outcome, err := VerifyProjects(legacy, modern, "Discount.calculate(int)", "Discount.calculate(int)")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != StatusNotEquivalent || outcome.Reason != "entry signatures are incompatible" {
		t.Fatalf("expected signature mismatch, got %#v", outcome)
	}
}

func TestVerifyProjectsExpandsEquivalentReachableDependency(t *testing.T) {
	legacy := t.TempDir()
	modern := t.TempDir()
	for _, root := range []string{legacy, modern} {
		source := `class Discount { public int calculate(Rules rules, int amount) { return rules.adjust(amount); } }`
		if err := os.WriteFile(filepath.Join(root, "Discount.java"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		rules := `class Rules { public int adjust(int amount) { return amount; } }`
		if err := os.WriteFile(filepath.Join(root, "Rules.java"), []byte(rules), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	outcome, err := VerifyProjects(legacy, modern, "Discount.calculate(Rules,int)", "Discount.calculate(Rules,int)")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != StatusProvenEquivalent {
		t.Fatalf("expected expanded dependency proof, got %#v", outcome)
	}
}

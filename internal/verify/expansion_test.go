package verify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyProjectsExpandsPureJavaDependency(t *testing.T) {
	legacy := t.TempDir()
	modern := t.TempDir()
	legacyFiles := map[string]string{
		"Discount.java": `class Discount { public int calculate(Rules rules, int amount) { return rules.adjust(amount); } }`,
		"Rules.java":    `class Rules { public int adjust(int amount) { return amount + 1; } }`,
	}
	modernFiles := map[string]string{
		"Discount.java": `class Discount { public int calculate(Rules rules, int amount) { return rules.adjust(amount); } }`,
		"Rules.java":    `class Rules { public int adjust(int amount) { return amount + 2; } }`,
	}
	for root, files := range map[string]map[string]string{legacy: legacyFiles, modern: modernFiles} {
		for name, source := range files {
			if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	outcome, err := VerifyProjects(legacy, modern, "Discount.calculate(Rules,int)", "Discount.calculate(Rules,int)")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != StatusNotEquivalent {
		t.Fatalf("expected expanded pure dependency to expose mismatch, got %#v", outcome)
	}
}

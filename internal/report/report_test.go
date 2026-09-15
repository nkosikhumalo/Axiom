package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nkosikhumalo/axiom/internal/project"
	"github.com/nkosikhumalo/axiom/internal/verify"
)

func TestWriteProjectReportsJSONAndSARIF(t *testing.T) {
	index := &project.Index{
		Files:       []project.File{{Path: "Broken.java", Diagnostic: "syntax error", Symbols: []project.Symbol{{Signature: "Service.run()", File: "Broken.java", StartLine: 17}}}},
		Edges:       []project.Edge{{From: "Service.run()", Call: "db", Status: "external_or_unresolved"}},
		Assumptions: []project.Assumption{{Kind: "external_or_unresolved", Symbol: "Service.run()", Call: "db", Reason: "external", Severity: "warning"}},
		Summaries:   []project.Summary{{Symbol: "Service.run()", Classification: project.ClassificationUnsupported, Reasons: []string{"reflection"}}},
	}
	root := t.TempDir()
	jsonPath := filepath.Join(root, "analysis.json")
	sarifPath := filepath.Join(root, "analysis.sarif")
	if err := WriteProject(jsonPath, "json", index); err != nil {
		t.Fatal(err)
	}
	if err := WriteProject(sarifPath, "sarif", index); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{jsonPath, sarifPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if path == sarifPath {
			text := string(data)
			for _, expected := range []string{"external_or_unresolved", "dependency-edge", "unsupported-behavior", "Broken.java", "startLine", "17"} {
				if !strings.Contains(text, expected) {
					t.Fatalf("expected SARIF to contain %q: %s", expected, data)
				}
			}
		}
	}
}

func TestWriteProjectSARIFIncludesSelectedEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "analysis.sarif")
	index := &project.Index{
		Entry:     "Service.run()",
		EntryFile: "src/Service.java",
		EntryLine: 12,
		Files:     []project.File{{Path: "src/Service.java", Symbols: []project.Symbol{{Signature: "Service.run()", StartLine: 12}}}},
	}
	if err := WriteProject(path, "sarif", index); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{"selected-entry", "Service.run()", "src/Service.java", "startLine", "12"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected selected entry metadata %q: %s", expected, data)
		}
	}
}

func TestWriteVerificationSARIFContainsStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verify.sarif")
	outcome := &verify.Outcome{Status: verify.StatusNotEquivalent, Reason: "counterexample found", Countermodel: "amount = 1000"}
	if err := WriteVerification(path, "sarif", outcome); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), verify.StatusNotEquivalent) || !strings.Contains(string(data), "counterexample found") || !strings.Contains(string(data), "amount = 1000") {
		t.Fatalf("expected verification SARIF result: %s", data)
	}
}

func TestWriteVerificationJSONUsesStableFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verify.json")
	outcome := &verify.Outcome{Status: verify.StatusCouldNotProve, Reason: "unsupported dependency", LegacyEntry: "old.Service.run", ModernEntry: "new.Service.run"}
	if err := WriteVerification(path, "json", outcome); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"status", "reason", "legacy_entry", "modern_entry"} {
		if _, ok := fields[field]; !ok {
			t.Fatalf("expected stable JSON field %q: %s", field, data)
		}
	}
}

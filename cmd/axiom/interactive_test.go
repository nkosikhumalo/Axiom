package main

import (
	"os"
	"path/filepath"
	"testing"

	projectindex "github.com/nkosikhumalo/axiom/internal/project"
)

func TestSelectEntryDeduplicatesSymbols(t *testing.T) {
	entries := []string{"Service.run()", "Service.run()", "Service.other()"}
	unique := uniqueEntries(entries)
	if len(unique) != 2 || unique[0] != "Service.run()" || unique[1] != "Service.other()" {
		t.Fatalf("unexpected unique entries: %#v", unique)
	}
}

func TestPathValidators(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "source.java")
	if err := os.WriteFile(file, []byte("class Source {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := directoryExists(root); err != nil {
		t.Fatalf("expected directory to validate: %v", err)
	}
	if err := fileExists(file); err != nil {
		t.Fatalf("expected file to validate: %v", err)
	}
	if directoryExists(file) == nil || fileExists(root) == nil {
		t.Fatal("expected directory and file validators to reject the wrong path type")
	}
}

func TestHasSelectableEntry(t *testing.T) {
	if hasSelectableEntry(projectindex.File{Symbols: []projectindex.Symbol{{Kind: "field"}}}) {
		t.Fatal("field-only file should not be selectable")
	}
	if !hasSelectableEntry(projectindex.File{Symbols: []projectindex.Symbol{{Kind: "method"}}}) {
		t.Fatal("method file should be selectable")
	}
}

func TestFileLinkFallsBackToAbsolutePathOutsideTerminal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	link := fileLink(path)
	if link != path {
		t.Fatalf("expected plain path outside a terminal, got %q", link)
	}
}

func TestFileInScopeSeparatesProductionAndTests(t *testing.T) {
	production := projectindex.File{Path: "src/main/java/Service.java"}
	testFile := projectindex.File{Path: "src/test/java/ServiceTest.java"}
	if !fileInScope(production, interactiveMain) || fileInScope(testFile, interactiveMain) {
		t.Fatal("expected production scope to exclude test files")
	}
	if !fileInScope(testFile, interactiveTests) || fileInScope(production, interactiveTests) {
		t.Fatal("expected test scope to include only test files")
	}
}

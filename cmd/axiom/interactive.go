package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	projectindex "github.com/nkosikhumalo/axiom/internal/project"
)

// interactiveScope controls which files are presented to the user during
// interactive project selection.
type interactiveScope int

const (
	interactiveMain  interactiveScope = iota // production source files
	interactiveTests                         // test source files
)

func runInteractive() error {
	reader := bufio.NewReader(os.Stdin)

	readRequired := func(label string) (string, error) {
		fmt.Printf("%s: ", label)
		value, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return "", fmt.Errorf("%s is required", label)
		}
		return value, nil
	}

	legacy, err := readRequired("Legacy source file")
	if err != nil {
		return err
	}

	modern, err := readRequired("Modern source file")
	if err != nil {
		return err
	}

	// Set the package-level vars directly to avoid an initialization cycle
	// (rootCmd → run → runInteractive → rootCmd).
	legacyFile = legacy
	modernFile = modern

	return run(rootCmd, nil)
}

// uniqueEntries deduplicates a slice of entry symbol strings, preserving order.
func uniqueEntries(entries []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, e := range entries {
		if !seen[e] {
			seen[e] = true
			result = append(result, e)
		}
	}
	return result
}

// directoryExists returns nil if path is an existing directory, or an error.
func directoryExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path does not exist: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}
	return nil
}

// fileExists returns nil if path is an existing regular file, or an error.
func fileExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("file does not exist: %s", path)
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory, not a file: %s", path)
	}
	return nil
}

// hasSelectableEntry returns true if the file contains at least one
// method or constructor symbol that can be used as a verification entry.
func hasSelectableEntry(file projectindex.File) bool {
	for _, sym := range file.Symbols {
		if sym.Kind == "method" || sym.Kind == "constructor" || sym.Kind == "function" {
			return true
		}
	}
	return false
}

// fileInScope reports whether file should be shown for the given interactive scope.
// Production scope excludes test directories; test scope includes only test directories.
func fileInScope(file projectindex.File, scope interactiveScope) bool {
	path := file.Path
	isTest := strings.Contains(path, "src/test/") ||
		strings.Contains(path, "_test.go") ||
		strings.HasSuffix(path, "Test.java") ||
		strings.HasSuffix(path, "Tests.java") ||
		strings.HasSuffix(path, "Spec.java")

	switch scope {
	case interactiveMain:
		return !isTest
	case interactiveTests:
		return isTest
	default:
		return true
	}
}

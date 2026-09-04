package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/nkosikhumalo/axiom/internal/ast"
	"github.com/nkosikhumalo/axiom/internal/reporter"
	"github.com/nkosikhumalo/axiom/internal/solver"
	"github.com/nkosikhumalo/axiom/internal/symbolic"
)

var (
	legacyFile string
	modernFile string
)

var rootCmd = &cobra.Command{
	Use:   "axiom",
	Short: "Axiom — formal verification engine for legacy rewrites",
	Long:  `Mathematically prove 100% logic equivalence when rewriting legacy code into modern languages.`,
	RunE:  run,
}

func init() {
	rootCmd.Flags().StringVar(&legacyFile, "legacy", "", "Path to legacy source file (e.g. discount.cpp)")
	rootCmd.Flags().StringVar(&modernFile, "modern", "", "Path to modern source file (e.g. discount.go)")
}

func run(_ *cobra.Command, _ []string) error {
	if legacyFile == "" || modernFile == "" {
		return fmt.Errorf("both --legacy and --modern flags are required")
	}

	fmt.Printf("[AXIOM] Parsing ASTs...\n")

	legacyTree, err := ast.ParseFile(legacyFile)
	if err != nil {
		return fmt.Errorf("legacy parse: %w", err)
	}

	modernTree, err := ast.ParseFile(modernFile)
	if err != nil {
		return fmt.Errorf("modern parse: %w", err)
	}

	fmt.Printf("[AXIOM] Extracting symbolic paths...\n")

	legacyRoot := ast.Walk(legacyTree)
	modernRoot := ast.Walk(modernTree)

	legacyPaths := symbolic.ExtractPaths(legacyRoot)
	modernPaths := symbolic.ExtractPaths(modernRoot)

	if len(legacyPaths) == 0 {
		return fmt.Errorf("no return paths found in legacy file — is it a supported function?")
	}
	if len(modernPaths) == 0 {
		return fmt.Errorf("no return paths found in modern file — is it a supported function?")
	}

	legacySMT := symbolic.BuildFunctionSMT(legacyPaths)
	modernSMT := symbolic.BuildFunctionSMT(modernPaths)

	fmt.Printf("[AXIOM] Building SMT-LIB2 query...\n")

	vars := solver.ExtractVars(legacySMT, modernSMT)
	query := solver.Build(solver.Query{
		Variables: vars,
		Legacy:    legacySMT,
		Modern:    modernSMT,
	})

	fmt.Printf("[AXIOM] Invoking Z3 theorem prover...\n\n")

	result, err := solver.RunZ3(query)
	if err != nil {
		return fmt.Errorf("z3: %w", err)
	}

	reporter.Report(result, legacyFile, modernFile)
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

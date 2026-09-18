package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/nkosikhumalo/axiom/internal/ast"
	projectindex "github.com/nkosikhumalo/axiom/internal/project"
	"github.com/nkosikhumalo/axiom/internal/refactor"
	"github.com/nkosikhumalo/axiom/internal/report"
	"github.com/nkosikhumalo/axiom/internal/reporter"
	"github.com/nkosikhumalo/axiom/internal/solver"
	"github.com/nkosikhumalo/axiom/internal/symbolic"
	"github.com/nkosikhumalo/axiom/internal/verify"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	legacyFile      string
	modernFile      string
	projectDir      string
	indexOut        string
	legacyProject   string
	modernProject   string
	legacyEntry     string
	modernEntry     string
	legacyFunc      string
	modernFunc      string
	projectEntry    string
	refactorMode    bool
	refactorOut     string
	approveRefactor bool
	rewriteMode     bool
	targetLang      string
	reportFormat    string
	reportOutput    string
)

var rootCmd = &cobra.Command{
	Use:   "axiom",
	Short: "Axiom — formal verification engine for legacy rewrites",
	Long:  `Mathematically prove 100% logic equivalence when rewriting legacy code into modern languages.`,
}

func init() {
	rootCmd.RunE = run
	rootCmd.Flags().StringVar(&legacyFile, "legacy", "", "Path to legacy source file (e.g. discount.cpp)")
	rootCmd.Flags().StringVar(&modernFile, "modern", "", "Path to modern source file (e.g. discount.go)")
	rootCmd.Flags().StringVar(&projectDir, "project", "", "Analyze a source project and build its dependency graph")
	rootCmd.Flags().StringVar(&indexOut, "index-output", "", "Path for the project dependency graph JSON artifact")
	rootCmd.Flags().StringVar(&legacyProject, "legacy-project", "", "Legacy project root for project-level verification")
	rootCmd.Flags().StringVar(&modernProject, "modern-project", "", "Modern project root for project-level verification")
	rootCmd.Flags().StringVar(&legacyEntry, "legacy-entry", "", "Fully qualified legacy project entry symbol")
	rootCmd.Flags().StringVar(&modernEntry, "modern-entry", "", "Fully qualified modern project entry symbol")
	rootCmd.Flags().StringVar(&legacyFunc, "legacy-function", "", "Function or method name to verify in the legacy file")
	rootCmd.Flags().StringVar(&modernFunc, "modern-function", "", "Function or method name to verify in the modern file")
	rootCmd.Flags().StringVar(&projectEntry, "entry", "", "Fully qualified project entry symbol for analysis or refactoring")
	rootCmd.Flags().BoolVar(&refactorMode, "refactor", false, "Generate a non-destructive refactoring plan for a project")
	rootCmd.Flags().StringVar(&refactorOut, "refactor-output", "", "Path for the refactoring plan Markdown artifact")
	rootCmd.Flags().BoolVar(&approveRefactor, "approve-refactor", false, "Approve generation of Java scaffolds and patches")
	rootCmd.Flags().BoolVar(&rewriteMode, "rewrite", false, "Generate an isolated rewrite of the selected entry method")
	rootCmd.Flags().StringVar(&targetLang, "target-lang", "java", "Target language for rewrite output: java or go")
	rootCmd.Flags().StringVar(&reportFormat, "format", "text", "Report format: text, json, or sarif")
	rootCmd.Flags().StringVar(&reportOutput, "report", "", "Path for a machine-readable report")
}

func run(command *cobra.Command, _ []string) error {
	if !hasExplicitFlags(command) {
		if !interactiveTerminal() {
			return fmt.Errorf("no command supplied; run axiom in a terminal or provide CLI flags (use --help for examples)")
		}
		return runInteractive()
	}
	if reportFormat != "text" && reportFormat != "json" && reportFormat != "sarif" {
		return fmt.Errorf("unsupported report format: %s", reportFormat)
	}
	if legacyProject != "" || modernProject != "" || legacyEntry != "" || modernEntry != "" {
		if legacyProject == "" || modernProject == "" || legacyEntry == "" || modernEntry == "" {
			return fmt.Errorf("--legacy-project, --modern-project, --legacy-entry, and --modern-entry are required together")
		}
		if projectDir != "" || legacyFile != "" || modernFile != "" || projectEntry != "" || refactorMode {
			return fmt.Errorf("project verification flags cannot be combined with analysis or single-file flags")
		}
		return verifyProjects(legacyProject, modernProject, legacyEntry, modernEntry)
	}
	if projectDir != "" {
		if legacyFile != "" || modernFile != "" || legacyFunc != "" || modernFunc != "" {
			return fmt.Errorf("--project cannot be combined with --legacy or --modern")
		}
		if (approveRefactor || rewriteMode) && !refactorMode {
			return fmt.Errorf("--approve-refactor and --rewrite require --refactor")
		}
		return analyzeProject(projectDir, indexOut, projectEntry, refactorMode, refactorOut, approveRefactor)
	}
	if refactorMode || projectEntry != "" || approveRefactor || rewriteMode {
		return fmt.Errorf("--refactor and --entry require --project")
	}
	if legacyFile == "" || modernFile == "" {
		return fmt.Errorf("both --legacy and --modern flags are required")
	}
	if (legacyFunc == "") != (modernFunc == "") {
		return fmt.Errorf("--legacy-function and --modern-function must be provided together")
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
	if legacyFunc != "" {
		legacyPaths = symbolic.ExtractPathsForFunction(legacyRoot, legacyFunc)
		modernPaths = symbolic.ExtractPathsForFunction(modernRoot, modernFunc)
	}

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

func interactiveTerminal() bool {
	stdin, stdinErr := os.Stdin.Stat()
	stdout, stdoutErr := os.Stdout.Stat()
	if stdinErr != nil || stdoutErr != nil {
		return false
	}
	return stdin.Mode()&os.ModeCharDevice != 0 && stdout.Mode()&os.ModeCharDevice != 0
}

func hasExplicitFlags(command *cobra.Command) bool {
	if command == nil {
		return true
	}
	explicit := false
	command.Flags().Visit(func(_ *pflag.Flag) { explicit = true })
	return explicit
}

func verifyProjects(legacyRoot, modernRoot, legacyEntryName, modernEntryName string) error {
	fmt.Printf("[AXIOM] Verifying project entries...\n")
	outcome, err := verify.VerifyProjects(legacyRoot, modernRoot, legacyEntryName, modernEntryName)
	if err != nil {
		return err
	}
	fmt.Printf("[AXIOM] Status: %s\n", outcome.Status)
	fmt.Printf("[AXIOM] Reason: %s\n", outcome.Reason)
	for _, assumption := range outcome.Assumptions {
		fmt.Printf("[AXIOM] Assumption (%s): %s calls %s (%s)\n", assumption.Severity, assumption.Symbol, assumption.Call, assumption.Reason)
	}
	if outcome.Countermodel != "" {
		fmt.Printf("[AXIOM] Countermodel:\n%s\n", outcome.Countermodel)
	}
	if reportFormat != "text" {
		path := reportOutput
		if path == "" {
			path = filepath.Join(legacyRoot, ".axiom", "verification-report."+reportExtension(reportFormat))
		}
		if err := report.WriteVerification(path, reportFormat, outcome); err != nil {
			return err
		}
		fmt.Printf("[AXIOM] Verification report written to %s\n", fileLink(path))
	}
	if outcome.Status == verify.StatusNotEquivalent {
		return &cliExitError{Code: 1, Message: outcome.Reason}
	}
	if outcome.Status == verify.StatusCouldNotProve {
		return &cliExitError{Code: 2, Message: outcome.Reason}
	}
	return nil
}

func analyzeProject(root, output, entry string, refactorMode bool, refactorOutput string, approve bool) error {
	fmt.Printf("[AXIOM] Indexing project: %s\n", root)
	index, err := projectindex.Discover(root)
	if err != nil {
		return fmt.Errorf("project analysis: %w", err)
	}
	if entry != "" {
		index, err = projectindex.Reachable(index, entry)
		if err != nil {
			return fmt.Errorf("resolve project entry: %w", err)
		}
		fmt.Printf("[AXIOM] Restricted analysis to reachable entry: %s\n", entry)
	}
	if output == "" {
		output = filepath.Join(root, ".axiom", "dependency-graph.json")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return fmt.Errorf("create index output directory: %w", err)
	}
	if err := index.WriteJSON(output); err != nil {
		return err
	}
	if reportFormat != "text" {
		path := reportOutput
		if path == "" {
			path = filepath.Join(root, ".axiom", "analysis-report."+reportExtension(reportFormat))
		}
		if err := report.WriteProject(path, reportFormat, index); err != nil {
			return err
		}
		fmt.Printf("[AXIOM] Analysis report written to %s\n", fileLink(path))
	}
	fmt.Printf("[AXIOM] Indexed %d files and %d dependency edges.\n", len(index.Files), len(index.Edges))
	classificationCounts := map[string]int{}
	for _, summary := range index.Summaries {
		classificationCounts[summary.Classification]++
	}
	fmt.Printf("[AXIOM] Method summaries: pure=%d stateful=%d external=%d unsupported=%d.\n",
		classificationCounts[projectindex.ClassificationPure],
		classificationCounts[projectindex.ClassificationStateful],
		classificationCounts[projectindex.ClassificationExternal],
		classificationCounts[projectindex.ClassificationUnsupported])
	for _, file := range index.Files {
		if file.Diagnostic != "" {
			fmt.Printf("[AXIOM] Diagnostic: %s: %s\n", file.Path, file.Diagnostic)
		}
	}
	for _, assumption := range index.Assumptions {
		fmt.Printf("[AXIOM] Assumption (%s): %s calls %s (%s)\n", assumption.Severity, assumption.Symbol, assumption.Call, assumption.Reason)
	}
	if index.Entry != "" {
		fmt.Printf("[AXIOM] Selected entry: %s\n", index.Entry)
		if index.EntryFile != "" {
			fmt.Printf("[AXIOM] Source: %s\n", sourceLink(index.Root, index.EntryFile, index.EntryLine))
		}
	}
	fmt.Printf("[AXIOM] Dependency graph written to %s\n", fileLink(output))
	if refactorMode {
		if refactorOutput == "" {
			refactorOutput = filepath.Join(root, ".axiom", "refactor-plan.md")
		}
		if err := os.MkdirAll(filepath.Dir(refactorOutput), 0o755); err != nil {
			return fmt.Errorf("create refactor output directory: %w", err)
		}
		plan := refactor.BuildJavaPlan(index, entry)
		if err := plan.WriteMarkdown(refactorOutput); err != nil {
			return err
		}
		fmt.Printf("[AXIOM] Refactoring plan written to %s\n", fileLink(refactorOutput))
		if approve {
			artifactDir := filepath.Dir(refactorOutput)
			var artifacts refactor.Artifacts
			if rewriteMode {
				artifacts, err = refactor.GenerateJavaArtifacts(index, refactor.Plan{Entry: entry}, artifactDir)
			} else {
				artifacts, err = refactor.GenerateJavaArtifacts(index, plan, artifactDir)
			}
			if err != nil {
				return err
			}
			if rewriteMode && entry != "" {
				switch targetLang {
				case "go":
					goRewrite, err := refactor.GenerateGoRewrite(index, entry, artifactDir)
					if err != nil {
						return err
					}
					artifacts.Files = append(artifacts.Files, goRewrite)
					fmt.Printf("[AXIOM] Go rewrite generated at %s\n", fileLink(filepath.Join(artifactDir, filepath.FromSlash(goRewrite.Path))))
					fmt.Printf("[AXIOM] SMT verification: %s\n", goRewrite.Finding[strings.Index(goRewrite.Finding, "smt-verification: ")+len("smt-verification: "):])
				default:
					rewrite, err := refactor.GenerateJavaRewrite(index, entry, artifactDir)
					if err != nil {
						return err
					}
					artifacts.Files = append(artifacts.Files, rewrite)
					fmt.Printf("[AXIOM] Improved Java rewrite generated at %s\n", fileLink(filepath.Join(artifactDir, filepath.FromSlash(rewrite.Path))))
				}
			}
			fmt.Printf("[AXIOM] Approved artifacts generated: %d files under %s\n", len(artifacts.Files), artifactDir)
			if err := refactor.ValidateJavaArtifacts(index, &artifacts, artifactDir); err != nil {
				return err
			}
			fmt.Printf("[AXIOM] Generated artifact validation: %s\n", artifacts.Validation.Status)
			fmt.Printf("[AXIOM] Generated formatting: %s; project tests: %s; differential: %s\n", artifacts.Validation.FormatStatus, artifacts.Validation.TestStatus, artifacts.Validation.DifferentialStatus)
			if artifacts.Validation.Output != "" {
				fmt.Printf("[AXIOM] Validation output: %s\n", artifacts.Validation.Output)
			}
		}
	}
	return nil
}

func sourceLink(root, relativePath string, line uint32) string {
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	label := path
	if line > 0 {
		label = fmt.Sprintf("%s:%d", path, line)
	}
	return fileLinkWithLabel(path, label)
}

func fileLink(path string) string {
	return fileLinkWithLabel(path, path)
}

func fileLinkWithLabel(path, label string) string {
	absolutePath, err := filepath.Abs(path)
	if err != nil || !interactiveTerminal() {
		if err == nil {
			return absolutePath
		}
		return path
	}
	uri := (&url.URL{Scheme: "file", Path: absolutePath}).String()
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", uri, label)
}

func reportExtension(format string) string {
	if format == "sarif" {
		return "sarif"
	}
	return "json"
}

type cliExitError struct {
	Code    int
	Message string
}

func (e *cliExitError) Error() string { return e.Message }

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if exitErr, ok := err.(*cliExitError); ok {
			os.Exit(exitErr.Code)
		}
		os.Exit(1)
	}
}

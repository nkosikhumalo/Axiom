package refactor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nkosikhumalo/axiom/internal/ast"
	"github.com/nkosikhumalo/axiom/internal/project"
	"github.com/nkosikhumalo/axiom/internal/rewrite"
)

// Artifact describes one generated file and its source finding.
type Artifact struct {
	Path    string `json:"path"`
	Patch   string `json:"patch"`
	Source  string `json:"source"`
	Finding string `json:"finding"`
}

// Artifacts is the manifest written for an approved refactoring plan.
type Artifacts struct {
	Approved   bool        `json:"approved"`
	Plan       string      `json:"plan"`
	Files      []Artifact  `json:"files"`
	Validation *Validation `json:"validation,omitempty"`
}

// Validation records compilation of generated artifacts.
type Validation struct {
	Status             string `json:"status"`
	Tool               string `json:"tool,omitempty"`
	ProjectType        string `json:"project_type,omitempty"`
	Output             string `json:"output,omitempty"`
	FormatStatus       string `json:"format_status,omitempty"`
	FormatTool         string `json:"format_tool,omitempty"`
	FormatOutput       string `json:"format_output,omitempty"`
	TestStatus         string `json:"test_status,omitempty"`
	TestTool           string `json:"test_tool,omitempty"`
	TestOutput         string `json:"test_output,omitempty"`
	DifferentialStatus string `json:"differential_status,omitempty"`
	DifferentialReason string `json:"differential_reason,omitempty"`
}

// GenerateJavaArtifacts creates reviewable Java delegation adapters and unified patches. It never writes
// into the analyzed source tree; outputDir must be separate from the project root.
func GenerateJavaArtifacts(index *project.Index, plan Plan, outputDir string) (Artifacts, error) {
	artifacts := Artifacts{Approved: true, Plan: plan.Entry, Files: make([]Artifact, 0)}
	generatedDir := filepath.Join(outputDir, "generated", "java")
	patchDir := filepath.Join(outputDir, "patches")
	if err := os.MkdirAll(generatedDir, 0o755); err != nil {
		return artifacts, fmt.Errorf("create generated source directory: %w", err)
	}
	if err := os.MkdirAll(patchDir, 0o755); err != nil {
		return artifacts, fmt.Errorf("create patch directory: %w", err)
	}

	used := map[string]int{}
	for _, finding := range plan.Findings {
		symbol, ok := findSymbol(index, finding.Symbol)
		if !ok {
			continue
		}
		if symbol.Package == "" {
			for _, file := range index.Files {
				if file.Path == symbol.File {
					symbol.Package = file.Package
					break
				}
			}
		}
		className := className(symbol)
		if className == "" {
			continue
		}
		used[className]++
		artifactName := className + "Refactoring"
		if used[className] > 1 {
			artifactName += fmt.Sprintf("%d", used[className])
		}
		artifactPath := filepath.ToSlash(filepath.Join("generated", "java", artifactName+".java"))
		content := delegationAdapter(symbol, finding, artifactName)
		if err := os.WriteFile(filepath.Join(outputDir, artifactPath), []byte(content), 0o644); err != nil {
			return artifacts, fmt.Errorf("write generated Java file: %w", err)
		}
		patchName := fmt.Sprintf("%03d-%s.patch", len(artifacts.Files)+1, strings.ToLower(artifactName))
		patchPath := filepath.ToSlash(filepath.Join("patches", patchName))
		patch := unifiedAddPatch(artifactPath, content)
		if err := os.WriteFile(filepath.Join(outputDir, patchPath), []byte(patch), 0o644); err != nil {
			return artifacts, fmt.Errorf("write Java patch: %w", err)
		}
		artifacts.Files = append(artifacts.Files, Artifact{Path: artifactPath, Patch: patchPath, Source: symbol.File, Finding: finding.Symbol})
	}
	sort.Slice(artifacts.Files, func(i, j int) bool { return artifacts.Files[i].Path < artifacts.Files[j].Path })
	manifest, err := json.MarshalIndent(artifacts, "", "  ")
	if err != nil {
		return artifacts, fmt.Errorf("encode artifact manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "artifacts.json"), append(manifest, '\n'), 0o644); err != nil {
		return artifacts, fmt.Errorf("write artifact manifest: %w", err)
	}
	return artifacts, nil
}

// GenerateJavaRewrite emits a standalone Java class containing the selected method body.
// It is intentionally limited to one method; callers must compile and review the result before use.
func GenerateJavaRewrite(index *project.Index, entry, outputDir string) (Artifact, error) {
	symbol, ok := findSymbol(index, entry)
	if !ok {
		return Artifact{}, fmt.Errorf("rewrite entry not found: %s", entry)
	}
	sourcePath := filepath.Join(index.Root, filepath.FromSlash(symbol.File))
	parsed, err := ast.ParseFile(sourcePath)
	if err != nil {
		return Artifact{}, fmt.Errorf("parse rewrite source: %w", err)
	}
	method := findSourceMethod(ast.Walk(parsed), symbol)
	if method == nil {
		return Artifact{}, fmt.Errorf("rewrite method body not found: %s", entry)
	}
	name := className(symbol) + "Modern"
	imports, fields := javaClassContext(ast.Walk(parsed), symbol)
	methodIR, err := rewrite.FromJavaASTWithContext(symbol, method, imports, fields)
	if err != nil {
		return Artifact{}, fmt.Errorf("build rewrite representation: %w", err)
	}
	content := methodIR.RenderJava(name)
	artifactPath := filepath.ToSlash(filepath.Join("generated", "java", name+".java"))
	if err := os.MkdirAll(filepath.Join(outputDir, "generated", "java"), 0o755); err != nil {
		return Artifact{}, fmt.Errorf("create rewrite directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, artifactPath), []byte(content), 0o644); err != nil {
		return Artifact{}, fmt.Errorf("write Java rewrite: %w", err)
	}
	patchPath := filepath.ToSlash(filepath.Join("patches", "rewrite-"+strings.ToLower(name)+".patch"))
	if err := os.MkdirAll(filepath.Join(outputDir, "patches"), 0o755); err != nil {
		return Artifact{}, fmt.Errorf("create rewrite patch directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, patchPath), []byte(unifiedAddPatch(artifactPath, content)), 0o644); err != nil {
		return Artifact{}, fmt.Errorf("write rewrite patch: %w", err)
	}
	return Artifact{Path: artifactPath, Patch: patchPath, Source: symbol.File, Finding: entry}, nil
}

func javaClassContext(root *ast.Node, symbol project.Symbol) ([]string, []string) {
	var imports []string
	for _, node := range ast.FindAll(root, "import_declaration") {
		imports = append(imports, strings.TrimSpace(node.Content))
	}
	for _, class := range ast.FindAll(root, "class_declaration") {
		name := ast.ChildByField(class, "name")
		if name == nil || name.Content != symbol.Class {
			continue
		}
		body := ast.ChildByField(class, "body")
		if body == nil {
			break
		}
		var fields []string
		for _, field := range ast.FindAll(body, "field_declaration") {
			fields = append(fields, field.Content)
		}
		return imports, fields
	}
	return imports, nil
}

func findSourceMethod(root *ast.Node, symbol project.Symbol) *ast.Node {
	for _, method := range ast.FindAll(root, "method_declaration") {
		if method.StartLine == symbol.StartLine && ast.ChildByField(method, "name") != nil && ast.ChildByField(method, "name").Content == symbol.Name {
			return method
		}
	}
	for _, method := range ast.FindAll(root, "method_declaration") {
		if ast.ChildByField(method, "name") != nil && ast.ChildByField(method, "name").Content == symbol.Name {
			return method
		}
	}
	return nil
}

// ValidateJavaArtifacts compiles project Java sources and generated adapters in an isolated
// classes directory and records the result in artifacts.json.
func ValidateJavaArtifacts(index *project.Index, artifacts *Artifacts, outputDir string) error {
	projectType := detectProjectType(index.Root, index.Files)
	if len(artifacts.Files) == 0 {
		artifacts.Validation = &Validation{Status: "skipped", Tool: "javac", ProjectType: projectType}
		markDifferentialUnavailable(artifacts.Validation)
		formatGeneratedSources(outputDir, artifacts.Validation, artifacts.Files)
		runProjectTests(index.Root, artifacts.Validation)
		return writeManifest(outputDir, *artifacts)
	}
	javac, err := exec.LookPath("javac")
	if err != nil {
		artifacts.Validation = &Validation{Status: "tool_unavailable", Tool: "javac", ProjectType: projectType, Output: err.Error()}
		markDifferentialUnavailable(artifacts.Validation)
		formatGeneratedSources(outputDir, artifacts.Validation, artifacts.Files)
		runProjectTests(index.Root, artifacts.Validation)
		return writeManifest(outputDir, *artifacts)
	}
	validation := &Validation{Status: "passed", Tool: "javac", ProjectType: projectType}
	markDifferentialUnavailable(validation)
	formatGeneratedSources(outputDir, validation, artifacts.Files)
	if validation.Status == "failed" || validation.Status == "timeout" {
		artifacts.Validation = validation
		return writeManifest(outputDir, *artifacts)
	}
	classesDir := filepath.Join(outputDir, "classes")
	if err := os.MkdirAll(classesDir, 0o755); err != nil {
		return fmt.Errorf("create Java classes directory: %w", err)
	}
	arguments := []string{"-d", classesDir}
	for _, file := range index.Files {
		if file.Language == "java" && file.Diagnostic == "" {
			arguments = append(arguments, filepath.Join(index.Root, filepath.FromSlash(file.Path)))
		}
	}
	for _, artifact := range artifacts.Files {
		arguments = append(arguments, filepath.Join(outputDir, filepath.FromSlash(artifact.Path)))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, javac, arguments...)
	output, commandErr := command.CombinedOutput()
	validation.Output = strings.TrimSpace(string(output))
	if ctx.Err() == context.DeadlineExceeded {
		validation.Status = "timeout"
	} else if commandErr != nil {
		validation.Status = "failed"
	}
	artifacts.Validation = validation
	runProjectTests(index.Root, artifacts.Validation)
	return writeManifest(outputDir, *artifacts)
}

func markDifferentialUnavailable(validation *Validation) {
	validation.DifferentialStatus = "skipped"
	validation.DifferentialReason = "requires a project-specific executable input harness"
}

func formatGeneratedSources(outputDir string, validation *Validation, artifacts []Artifact) {
	formatter, err := exec.LookPath("google-java-format")
	if err != nil {
		validation.FormatStatus = "skipped"
		validation.FormatTool = "google-java-format"
		return
	}
	paths := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		paths = append(paths, filepath.Join(outputDir, filepath.FromSlash(artifact.Path)))
	}
	if len(paths) == 0 {
		validation.FormatStatus = "skipped"
		validation.FormatTool = "google-java-format"
		return
	}
	arguments := append([]string{"--dry-run", "--set-exit-if-changed"}, paths...)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, formatter, arguments...)
	output, commandErr := command.CombinedOutput()
	validation.FormatTool = "google-java-format"
	validation.FormatOutput = strings.TrimSpace(string(output))
	if ctx.Err() == context.DeadlineExceeded {
		validation.FormatStatus = "timeout"
		validation.Status = "timeout"
	} else if commandErr != nil {
		validation.FormatStatus = "failed"
		validation.Status = "failed"
	} else {
		validation.FormatStatus = "passed"
	}
}

func runProjectTests(root string, validation *Validation) {
	command, arguments, tool := testCommand(root)
	if command == "" {
		validation.TestStatus = "skipped"
		validation.TestTool = "none"
		return
	}
	validation.TestTool = tool
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, command, arguments...)
	process.Dir = root
	output, err := process.CombinedOutput()
	validation.TestOutput = strings.TrimSpace(string(output))
	if ctx.Err() == context.DeadlineExceeded {
		validation.TestStatus = "timeout"
		validation.Status = "timeout"
	} else if err != nil {
		validation.TestStatus = "failed"
		if validation.Status == "passed" || validation.Status == "skipped" {
			validation.Status = "failed"
		}
	} else {
		validation.TestStatus = "passed"
	}
}

func testCommand(root string) (string, []string, string) {
	if _, err := os.Stat(filepath.Join(root, "pom.xml")); err == nil {
		if command, err := exec.LookPath("mvn"); err == nil {
			return command, []string{"-q", "test"}, "maven"
		}
	}
	if _, err := os.Stat(filepath.Join(root, "gradlew")); err == nil {
		return "/bin/sh", []string{"./gradlew", "test", "--no-daemon"}, "gradle-wrapper"
	}
	if _, err := os.Stat(filepath.Join(root, "build.gradle")); err == nil {
		if command, err := exec.LookPath("gradle"); err == nil {
			return command, []string{"test", "--no-daemon"}, "gradle"
		}
	}
	return "", nil, ""
}

func detectProjectType(root string, files []project.File) string {
	if _, err := os.Stat(filepath.Join(root, "pom.xml")); err == nil {
		return "maven"
	}
	for _, name := range []string{"settings.gradle", "settings.gradle.kts", "build.gradle", "build.gradle.kts", "gradlew"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return "gradle"
		}
	}
	for _, file := range files {
		if file.Language == "java" {
			return "plain-java"
		}
	}
	return "unknown"
}

func writeManifest(outputDir string, artifacts Artifacts) error {
	manifest, err := json.MarshalIndent(artifacts, "", "  ")
	if err != nil {
		return fmt.Errorf("encode artifact manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "artifacts.json"), append(manifest, '\n'), 0o644); err != nil {
		return fmt.Errorf("write artifact manifest: %w", err)
	}
	return nil
}

func findSymbol(index *project.Index, identity string) (project.Symbol, bool) {
	for _, file := range index.Files {
		for _, symbol := range file.Symbols {
			key := symbol.Signature
			if key == "" {
				key = symbol.QualifiedName
			}
			if key == identity {
				return symbol, true
			}
		}
	}
	return project.Symbol{}, false
}

func className(symbol project.Symbol) string {
	if symbol.Class != "" {
		return symbol.Class
	}
	parts := strings.Split(symbol.QualifiedName, ".")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return ""
}

func delegationAdapter(symbol project.Symbol, finding Finding, artifactName string) string {
	var builder strings.Builder
	if symbol.Package != "" {
		fmt.Fprintf(&builder, "package %s;\n\n", symbol.Package)
	}
	builder.WriteString("/**\n")
	builder.WriteString(" * Behavior-preserving delegation adapter generated by Axiom.\n")
	fmt.Fprintf(&builder, " * Candidate extracted from %s at line %d.\n", symbol.File, finding.Line)
	builder.WriteString(" * Validate this adapter before applying the patch.\n")
	builder.WriteString(" */\n")
	fmt.Fprintf(&builder, "public final class %s {\n", artifactName)
	fmt.Fprintf(&builder, "    private final %s delegate;\n\n", symbol.Class)
	fmt.Fprintf(&builder, "    public %s(%s delegate) {\n        this.delegate = delegate;\n    }\n", artifactName, symbol.Class)
	if symbol.Kind == "constructor" {
		builder.WriteString("\n    public ")
		builder.WriteString(symbol.Class)
		builder.WriteString(" delegate() {\n        return delegate;\n    }\n")
	} else {
		fmt.Fprintf(&builder, "\n    public %s %s(%s) {\n", javaType(symbol.ReturnType), symbol.Name, javaParameters(symbol.Parameters))
		if symbol.ReturnType != "void" {
			builder.WriteString("        return ")
		}
		fmt.Fprintf(&builder, "delegate.%s(%s);\n    }\n", symbol.Name, parameterNames(symbol.Parameters))
	}
	builder.WriteString("}\n")
	return builder.String()
}

func javaType(typeName string) string {
	if typeName == "" {
		return "Object"
	}
	return typeName
}

func javaParameters(parameters []project.Parameter) string {
	parts := make([]string, 0, len(parameters))
	for _, parameter := range parameters {
		parts = append(parts, fmt.Sprintf("%s %s", javaType(parameter.Type), parameter.Name))
	}
	return strings.Join(parts, ", ")
}

func parameterNames(parameters []project.Parameter) string {
	parts := make([]string, 0, len(parameters))
	for _, parameter := range parameters {
		parts = append(parts, parameter.Name)
	}
	return strings.Join(parts, ", ")
}

func unifiedAddPatch(path, content string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", path, strings.Count(content, "\n"))
	for _, line := range strings.SplitAfter(content, "\n") {
		builder.WriteByte('+')
		builder.WriteString(line)
	}
	return builder.String()
}

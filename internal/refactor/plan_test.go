package refactor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nkosikhumalo/axiom/internal/project"
)

func TestBuildJavaPlanIsNonDestructive(t *testing.T) {
	plan := BuildJavaPlan(&project.Index{Files: []project.File{{
		Path:     "Discount.java",
		Language: "java",
		Symbols:  []project.Symbol{{Name: "calculate", Kind: "method", File: "Discount.java"}},
	}}}, "Discount.calculate")

	if len(plan.Actions) != 1 || !strings.Contains(plan.Actions[0], "calculate") {
		t.Fatalf("expected a method restructuring action, got %#v", plan.Actions)
	}
	if len(plan.Warnings) == 0 {
		t.Fatal("expected explicit review warnings")
	}
}

func TestBuildJavaPlanReportsEvidenceBasedFindings(t *testing.T) {
	plan := BuildJavaPlan(&project.Index{
		Files: []project.File{{
			Path:     "DiscountService.java",
			Language: "java",
			Symbols: []project.Symbol{{
				Name: "calculate", QualifiedName: "DiscountService.calculate", Signature: "DiscountService.calculate(int,int,int,int,int)",
				Kind: "method", File: "DiscountService.java", StartLine: 12,
				Metrics:     project.Metrics{LineCount: 55, StatementCount: 28, MaxNesting: 5, ParameterCount: 5},
				Effects:     []string{"mutates_state"},
				Invocations: []project.Invocation{{Name: "saveAudit"}},
			}},
		}},
	}, "DiscountService.calculate")

	if len(plan.Findings) != 1 {
		t.Fatalf("expected one finding, got %#v", plan.Findings)
	}
	finding := plan.Findings[0]
	if finding.File != "DiscountService.java" || finding.Line != 12 || len(finding.Reasons) < 5 {
		t.Fatalf("expected detailed finding, got %#v", finding)
	}
	path := filepath.Join(t.TempDir(), "plan.md")
	if err := plan.WriteMarkdown(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Evidence-Based Findings") || !strings.Contains(string(data), "DiscountService.java:12") {
		t.Fatalf("expected finding evidence in Markdown: %s", data)
	}
}

func TestGenerateJavaArtifactsIsolatedAndReviewable(t *testing.T) {
	output := t.TempDir()
	index := &project.Index{Files: []project.File{{
		Path: "DiscountService.java", Language: "java", Package: "com.example",
		Symbols: []project.Symbol{{QualifiedName: "com.example.DiscountService.calculate", Signature: "com.example.DiscountService.calculate(int)", Class: "DiscountService", Name: "calculate", File: "DiscountService.java"}},
	}}}
	plan := Plan{Entry: "com.example.DiscountService.calculate", Findings: []Finding{{
		Symbol: "com.example.DiscountService.calculate(int)", File: "DiscountService.java", Line: 10,
	}}}

	artifacts, err := GenerateJavaArtifacts(index, plan, output)
	if err != nil {
		t.Fatal(err)
	}
	if !artifacts.Approved || len(artifacts.Files) != 1 {
		t.Fatalf("expected one approved artifact, got %#v", artifacts)
	}
	generatedPath := filepath.Join(output, "generated", "java", "DiscountServiceRefactoring.java")
	patchPath := filepath.Join(output, artifacts.Files[0].Patch)
	for _, path := range []string{generatedPath, patchPath, filepath.Join(output, "artifacts.json")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected generated artifact %s: %v", path, err)
		}
	}
	data, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "package com.example;") || !strings.Contains(string(data), "DiscountServiceRefactoring") || !strings.Contains(string(data), "delegate.calculate") {
		t.Fatalf("unexpected generated Java scaffold: %s", data)
	}
}

func TestGenerateJavaRewriteCopiesMethodBody(t *testing.T) {
	root := t.TempDir()
	source := `package com.example;
public class DiscountService {
	public static int calculate(int amount) {
        if (amount > 100) { return amount - 10; }
        return amount;
    }
}`
	if err := os.WriteFile(filepath.Join(root, "DiscountService.java"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	index := &project.Index{Root: root, Files: []project.File{{
		Path: "DiscountService.java", Language: "java", Package: "com.example",
		Symbols: []project.Symbol{{QualifiedName: "com.example.DiscountService.calculate", Signature: "com.example.DiscountService.calculate(int)", Class: "DiscountService", Name: "calculate", File: "DiscountService.java", Package: "com.example", ReturnType: "int", Parameters: []project.Parameter{{Name: "amount", Type: "int"}}, StartLine: 3}},
	}}}
	output := t.TempDir()
	artifact, err := GenerateJavaRewrite(index, "com.example.DiscountService.calculate(int)", output)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(artifact.Path)))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{"class DiscountServiceModern", "public static int calculate(int amount)", "amount > 100", "amount - 10"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected generated rewrite to contain %q: %s", expected, data)
		}
	}
}

func TestValidateJavaArtifactsCompilesGeneratedAdapter(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "DiscountService.java"), []byte(`package com.example;
public class DiscountService {
    public int calculate(int amount) { return amount + 1; }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	index := &project.Index{Root: root, Files: []project.File{{
		Path: "DiscountService.java", Language: "java", Package: "com.example",
		Symbols: []project.Symbol{{QualifiedName: "com.example.DiscountService.calculate", Signature: "com.example.DiscountService.calculate(int)", Class: "DiscountService", Package: "com.example", Name: "calculate", File: "DiscountService.java", ReturnType: "int", Parameters: []project.Parameter{{Name: "amount", Type: "int"}}}},
	}}}
	plan := Plan{Entry: "com.example.DiscountService.calculate", Findings: []Finding{{Symbol: "com.example.DiscountService.calculate(int)", File: "DiscountService.java", Line: 3}}}
	output := t.TempDir()
	artifacts, err := GenerateJavaArtifacts(index, plan, output)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateJavaArtifacts(index, &artifacts, output); err != nil {
		t.Fatal(err)
	}
	if artifacts.Validation.Status != "passed" && artifacts.Validation.Status != "tool_unavailable" {
		t.Fatalf("expected compilation pass or unavailable tool, got %#v", artifacts.Validation)
	}
	if artifacts.Validation.ProjectType != "plain-java" {
		t.Fatalf("expected plain Java project detection, got %#v", artifacts.Validation)
	}
	manifest, err := os.ReadFile(filepath.Join(output, "artifacts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), `"project_type": "plain-java"`) {
		t.Fatalf("expected project type in manifest: %s", manifest)
	}
}

func TestDetectProjectTypeRecognizesBuildTools(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pom.xml"), []byte("<project/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := detectProjectType(root, nil); got != "maven" {
		t.Fatalf("expected Maven project, got %q", got)
	}

	gradleRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(gradleRoot, "build.gradle.kts"), []byte("plugins {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := detectProjectType(gradleRoot, nil); got != "gradle" {
		t.Fatalf("expected Gradle project, got %q", got)
	}
}

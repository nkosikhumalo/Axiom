package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverIndexesSupportedFilesAndSymbols(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "legacy.cpp"), []byte(`int discount(int amount) { return amount - 1; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "modern.go"), []byte(`package main
func discount(amount int) int { return amount - 1 }`), 0o644); err != nil {
		t.Fatal(err)
	}

	index, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Files) != 2 {
		t.Fatalf("expected two indexed files, got %d", len(index.Files))
	}
	if len(index.Files[0].Symbols) == 0 || len(index.Files[1].Symbols) == 0 {
		t.Fatalf("expected symbols in every indexed file: %#v", index.Files)
	}
}

func TestDiscoverIndexesCppNamespacesAcrossHeaderAndSource(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Discount.h"), []byte(`namespace legacy { int calculate(int amount); }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Discount.cpp"), []byte(`namespace legacy { int calculate(int amount) { return amount - 1; } }`), 0o644); err != nil {
		t.Fatal(err)
	}
	index, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Files) != 2 {
		t.Fatalf("expected header and source files, got %#v", index.Files)
	}
	found := false
	for _, file := range index.Files {
		if file.Path == "Discount.cpp" && file.Package == "legacy" {
			for _, symbol := range file.Symbols {
				if symbol.QualifiedName == "legacy.calculate" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf("expected namespaced C++ function, got %#v", index.Files)
	}
}

func TestDiscoverResolvesNamespacedCppFunctionCall(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"Rules.h":     `namespace legacy { int adjust(int amount); }`,
		"Rules.cpp":   `namespace legacy { int adjust(int amount) { return amount - 1; } }`,
		"Service.cpp": `namespace legacy { int calculate(int amount) { return adjust(amount); } }`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	index, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, edge := range index.Edges {
		if edge.From == "legacy.calculate(int)" && edge.Call == "adjust" {
			if edge.Status != "resolved" || edge.To != "legacy.adjust(int)" {
				t.Fatalf("expected namespaced free function call to resolve, got %#v", edge)
			}
			return
		}
	}
	t.Fatal("expected C++ free function edge")
}

func TestDiscoverIndexesJavaStructure(t *testing.T) {
	root := t.TempDir()
	source := `package com.example;
public class DiscountService extends BaseService {
    private int limit;
    public int calculate(Customer customer, int tier) { return tier; }
}`
	if err := os.WriteFile(filepath.Join(root, "DiscountService.java"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	index, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Files) != 1 {
		t.Fatalf("expected one indexed Java file, got %d", len(index.Files))
	}
	file := index.Files[0]
	if file.Package != "com.example" || len(file.Types) != 1 {
		t.Fatalf("expected Java package and class metadata, got %#v", file)
	}
	if file.Types[0].Name != "DiscountService" || file.Types[0].Superclass != "BaseService" || len(file.Types[0].Fields) != 1 {
		t.Fatalf("unexpected Java type metadata: %#v", file.Types[0])
	}
	if len(file.Symbols) != 1 {
		t.Fatalf("expected one Java method, got %#v", file.Symbols)
	}
	symbol := file.Symbols[0]
	if symbol.QualifiedName != "com.example.DiscountService.calculate" || symbol.ReturnType != "int" || symbol.Visibility != "public" || len(symbol.Parameters) != 2 {
		t.Fatalf("unexpected Java method metadata: %#v", symbol)
	}
}

func TestDiscoverResolvesJavaRelationshipsAndOverloads(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"Base.java":   `package com.example; public class Base {}`,
		"Policy.java": `package com.example; public interface Policy { int apply(int amount); }`,
		"DiscountService.java": `package com.example;
public class DiscountService extends Base implements Policy {
    public DiscountService() {}
    public int apply(int amount) { return amount; }
    public int calculate(int amount) { return amount - 1; }
    public int calculate(String amount) { return amount.length(); }
	public int dispatchInt() { return calculate(1); }
	public int dispatchString() { return calculate("x"); }
}`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	index, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	var service File
	for _, file := range index.Files {
		if file.Path == "DiscountService.java" {
			service = file
		}
	}
	if len(service.Types) != 1 || service.Types[0].SuperclassQualified != "com.example.Base" || len(service.Types[0].InterfacesQualified) != 1 || service.Types[0].InterfacesQualified[0] != "com.example.Policy" {
		t.Fatalf("unexpected resolved Java relationships: %#v", service.Types)
	}
	if _, err := FindSymbol(index, "com.example.DiscountService.calculate", []string{"int"}); err != nil {
		t.Fatalf("expected int overload to resolve: %v", err)
	}
	if _, err := FindSymbol(index, "com.example.DiscountService.calculate", []string{"String"}); err != nil {
		t.Fatalf("expected String overload to resolve: %v", err)
	}
	if _, err := FindSymbol(index, "com.example.DiscountService.calculate", nil); err == nil {
		t.Fatal("expected overload lookup without parameter types to be ambiguous")
	}
	if _, err := FindSymbol(index, "com.example.DiscountService.DiscountService", []string{}); err != nil {
		t.Fatalf("expected constructor to resolve: %v", err)
	}
	if _, err := FindSymbol(index, "com.example.Policy.apply", []string{"int"}); err != nil {
		t.Fatalf("expected interface method to resolve: %v", err)
	}
	var intDispatch, stringDispatch bool
	for _, edge := range index.Edges {
		if edge.Call == "calculate" && edge.Status == "resolved" && edge.To == "com.example.DiscountService.calculate(int)" {
			intDispatch = true
		}
		if edge.Call == "calculate" && edge.Status == "resolved" && edge.To == "com.example.DiscountService.calculate(String)" {
			stringDispatch = true
		}
	}
	if !intDispatch || !stringDispatch {
		t.Fatalf("expected both overload calls to resolve, got %#v", index.Edges)
	}
}

func TestDiscoverResolvesJavaCrossFileCalls(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"com/rules/PricingRules.java": `package com.rules; public class PricingRules { public PricingRules() {} public int getDiscount(int amount) { return amount - 1; } public static int staticDiscount(int amount) { return amount; } }`,
		"DiscountService.java": `package com.example; import com.rules.PricingRules;
public class DiscountService {
    public int calculate(PricingRules rules, int amount) { return rules.getDiscount(amount); }
    public int external(Customer customer) { return customer.value(); }
		public int staticCall() { return PricingRules.staticDiscount(1); }
		public int constructCall() { return new PricingRules().getDiscount(1); }
}`,
	}
	for name, source := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	index, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	var resolved, external, staticCall, constructorCall bool
	for _, edge := range index.Edges {
		if edge.Call == "getDiscount" && edge.Status == "resolved" && edge.To == "com.rules.PricingRules.getDiscount(int)" {
			resolved = true
		}
		if edge.Call == "value" && edge.Status == "external_or_unresolved" {
			external = true
		}
		if edge.Call == "staticDiscount" && edge.Status == "resolved" && edge.To == "com.rules.PricingRules.staticDiscount(int)" {
			staticCall = true
		}
		if edge.Call == "PricingRules" && edge.Status == "resolved" && edge.To == "com.rules.PricingRules.PricingRules()" {
			constructorCall = true
		}
	}
	if !resolved || !external || !staticCall || !constructorCall {
		t.Fatalf("expected resolved and external edges, got %#v", index.Edges)
	}
}

func TestDiscoverResolvesJavaFieldReceiverCalls(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"SchedulerService.java": `package com.example; public class SchedulerService { public void manualTrigger(String source) {} }`,
		"CrawlController.java": `package com.example; public class CrawlController {
    private final SchedulerService schedulerService;
    public CrawlController(SchedulerService schedulerService) { this.schedulerService = schedulerService; }
		public void trigger() { String source = "manual"; schedulerService.manualTrigger(source); }
}`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	index, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, edge := range index.Edges {
		if edge.From == "com.example.CrawlController.trigger()" && edge.Call == "manualTrigger" {
			if edge.Status != "resolved" || edge.To != "com.example.SchedulerService.manualTrigger(String)" {
				t.Fatalf("expected field receiver call to resolve, got %#v", edge)
			}
			return
		}
	}
	t.Fatal("expected field receiver edge")
}

func TestReachableFiltersProjectFromEntry(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"Rules.java":   `package com.example; public class Rules { public int get(int amount) { return amount - 1; } }`,
		"Unused.java":  `package com.example; public class Unused { public int ignored() { return 0; } }`,
		"Service.java": `package com.example; public class Service { public int calculate(Rules rules, int amount) { return rules.get(amount); } public int unrelated() { return 1; } }`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	index, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	reachable, err := Reachable(index, "com.example.Service.calculate")
	if err != nil {
		t.Fatal(err)
	}
	if !reachable.Reachable || reachable.Entry != "com.example.Service.calculate" {
		t.Fatalf("expected reachable index metadata, got %#v", reachable)
	}
	if reachable.EntryFile != "Service.java" || reachable.EntryLine == 0 {
		t.Fatalf("expected selected entry location, got %#v", reachable)
	}
	if len(reachable.Files) != 2 {
		t.Fatalf("expected Service and Rules only, got %#v", reachable.Files)
	}
	for _, file := range reachable.Files {
		if file.Path == "Unused.java" {
			t.Fatal("unreachable file was included")
		}
		for _, symbol := range file.Symbols {
			if symbol.Name == "unrelated" {
				t.Fatal("unreachable method was included")
			}
		}
	}
	if len(reachable.Edges) != 1 || reachable.Edges[0].Status != "resolved" {
		t.Fatalf("expected one resolved reachable edge, got %#v", reachable.Edges)
	}
}

func TestResolveEntryRejectsMissingAndAmbiguousSymbols(t *testing.T) {
	index := &Index{Files: []File{{Symbols: []Symbol{
		{QualifiedName: "com.example.Service.calculate", Signature: "com.example.Service.calculate(int)"},
		{QualifiedName: "com.example.Service.calculate", Signature: "com.example.Service.calculate(String)"},
	}}}}
	if _, err := ResolveEntry(index, "com.example.Service.missing"); err == nil {
		t.Fatal("expected missing entry to fail")
	}
	if _, err := ResolveEntry(index, "com.example.Service.calculate"); err == nil {
		t.Fatal("expected overloaded entry without signature to fail")
	}
	if _, err := ResolveEntry(index, "com.example.Service.calculate(int)"); err != nil {
		t.Fatalf("expected signed entry to resolve: %v", err)
	}
}

package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nkosikhumalo/axiom/internal/ast"
)

// Symbol describes a function, method, or constructor discovered in a project file.
type Symbol struct {
	Name          string       `json:"name"`
	QualifiedName string       `json:"qualified_name,omitempty"`
	Kind          string       `json:"kind"`
	File          string       `json:"file"`
	Package       string       `json:"package,omitempty"`
	Class         string       `json:"class,omitempty"`
	Visibility    string       `json:"visibility,omitempty"`
	ReturnType    string       `json:"return_type,omitempty"`
	Parameters    []Parameter  `json:"parameters,omitempty"`
	Signature     string       `json:"signature,omitempty"`
	StartLine     uint32       `json:"start_line,omitempty"`
	StartColumn   uint32       `json:"start_column,omitempty"`
	Calls         []string     `json:"calls,omitempty"`
	Invocations   []Invocation `json:"invocations,omitempty"`
	Effects       []string     `json:"effects,omitempty"`
	Metrics       Metrics      `json:"metrics,omitempty"`
}

// Metrics are lightweight structural measurements used by the refactoring planner.
type Metrics struct {
	LineCount          int `json:"line_count,omitempty"`
	StatementCount     int `json:"statement_count,omitempty"`
	MaxNesting         int `json:"max_nesting,omitempty"`
	ParameterCount     int `json:"parameter_count,omitempty"`
	ConditionCount     int `json:"condition_count,omitempty"`
	RepeatedConditions int `json:"repeated_conditions,omitempty"`
}

// Invocation describes a call made from a discovered symbol.
type Invocation struct {
	Name          string   `json:"name"`
	Receiver      string   `json:"receiver,omitempty"`
	ArgumentCount int      `json:"argument_count"`
	ArgumentTypes []string `json:"argument_types,omitempty"`
	Arguments     []string `json:"arguments,omitempty"`
	Content       string   `json:"content,omitempty"`
	Kind          string   `json:"kind"`
}

// Parameter describes a method or function parameter when the grammar exposes its type.
type Parameter struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// Type describes a Java class or interface declaration.
type Type struct {
	Name                string            `json:"name"`
	Kind                string            `json:"kind"`
	Superclass          string            `json:"superclass,omitempty"`
	SuperclassQualified string            `json:"superclass_qualified,omitempty"`
	Interfaces          []string          `json:"interfaces,omitempty"`
	InterfacesQualified []string          `json:"interfaces_qualified,omitempty"`
	Fields              []string          `json:"fields,omitempty"`
	FieldTypes          map[string]string `json:"field_types,omitempty"`
	StartLine           uint32            `json:"start_line,omitempty"`
	StartColumn         uint32            `json:"start_column,omitempty"`
}

// File describes one parsed source file in the project index.
type File struct {
	Path       string   `json:"path"`
	Language   string   `json:"language"`
	Package    string   `json:"package,omitempty"`
	Types      []Type   `json:"types,omitempty"`
	Imports    []string `json:"imports,omitempty"`
	Symbols    []Symbol `json:"symbols,omitempty"`
	Diagnostic string   `json:"diagnostic,omitempty"`
}

// Edge represents a best-effort call relationship between discovered symbols.
type Edge struct {
	From   string `json:"from"`
	To     string `json:"to,omitempty"`
	Call   string `json:"call"`
	Status string `json:"status"`
}

// Index is the project structure used by analysis and future verification modes.
type Index struct {
	Root        string       `json:"root"`
	Entry       string       `json:"entry,omitempty"`
	EntryFile   string       `json:"entry_file,omitempty"`
	EntryLine   uint32       `json:"entry_line,omitempty"`
	EntryColumn uint32       `json:"entry_column,omitempty"`
	Reachable   bool         `json:"reachable_only,omitempty"`
	Files       []File       `json:"files"`
	Edges       []Edge       `json:"edges"`
	Summaries   []Summary    `json:"summaries"`
	Assumptions []Assumption `json:"assumptions"`
}

var supportedExtensions = map[string]ast.Language{
	".go": ast.LangGo, ".cpp": ast.LangCPP, ".cc": ast.LangCPP, ".cxx": ast.LangCPP,
	".c": ast.LangCPP, ".h": ast.LangCPP, ".hpp": ast.LangCPP, ".java": ast.LangJava, ".php": ast.LangPHP,
}

var symbolTypes = map[string]string{
	"function_declaration":    "function",
	"function_definition":     "function",
	"method_declaration":      "method",
	"constructor_declaration": "constructor",
	"declaration":             "function",
}

// Discover walks root, parses supported files, and builds a best-effort dependency index.
// Individual file parse failures are retained as diagnostics so one bad file does not hide the
// rest of the project structure.
func Discover(root string) (*Index, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("stat project root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("project path is not a directory: %s", root)
	}

	index := &Index{Root: absoluteRoot}
	err = filepath.WalkDir(absoluteRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if shouldSkipDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		language, supported := supportedExtensions[strings.ToLower(filepath.Ext(path))]
		if !supported {
			return nil
		}

		relativePath, err := filepath.Rel(absoluteRoot, path)
		if err != nil {
			return err
		}
		file := File{Path: filepath.ToSlash(relativePath), Language: string(language)}
		parsed, err := ast.ParseFile(path)
		if err != nil {
			file.Diagnostic = err.Error()
			index.Files = append(index.Files, file)
			return nil
		}
		root := ast.Walk(parsed)
		file.Imports = collectImports(root)
		file.Package, file.Types = collectJavaTypes(root)
		if file.Package == "" {
			file.Package = collectNamespace(root)
		}
		file.Symbols = collectSymbols(root, file.Path, file.Package, file.Types)
		index.Files = append(index.Files, file)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover project: %w", err)
	}

	sort.Slice(index.Files, func(i, j int) bool { return index.Files[i].Path < index.Files[j].Path })
	resolveTypeRelationships(index)
	index.Edges = resolveEdges(index)
	AnalyzeSummaries(index)
	return index, nil
}

// WriteJSON writes the index as an inspectable machine-readable artifact.
func (index *Index) WriteJSON(path string) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("encode project index: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write project index: %w", err)
	}
	return nil
}

func collectImports(root *ast.Node) []string {
	var imports []string
	for _, nodeType := range []string{"import_declaration", "import_statement", "preproc_include", "namespace_use_declaration"} {
		for _, node := range ast.FindAll(root, nodeType) {
			imports = append(imports, strings.TrimSpace(node.Content))
		}
	}
	sort.Strings(imports)
	return imports
}

func collectSymbols(root *ast.Node, filePath, packageName string, types []Type) []Symbol {
	var symbols []Symbol
	for nodeType, kind := range symbolTypes {
		for _, node := range ast.FindAll(root, nodeType) {
			if nodeType == "declaration" && ast.ChildByField(node, "declarator") == nil {
				continue
			}
			name := symbolName(node)
			if name == nil {
				continue
			}
			symbol := Symbol{Name: name.Content, Kind: kind, File: filePath, Package: packageName, StartLine: node.StartLine, StartColumn: node.StartColumn}
			if len(types) == 1 {
				symbol.Class = types[0].Name
			}
			symbol.Visibility = visibility(node)
			symbol.ReturnType = fieldContent(node, "type")
			symbol.Parameters = parameters(node)
			symbol.QualifiedName = qualifiedSymbolName(symbol)
			symbol.Signature = symbolSignature(symbol)
			symbol.Invocations = collectInvocations(node, symbol.Parameters)
			symbol.Effects = collectEffects(node)
			symbol.Metrics = collectMetrics(node, len(symbol.Parameters))
			for _, invocation := range symbol.Invocations {
				symbol.Calls = append(symbol.Calls, invocation.Name)
			}
			sort.Strings(symbol.Calls)
			symbols = append(symbols, symbol)
		}
	}
	sort.Slice(symbols, func(i, j int) bool { return symbols[i].Name < symbols[j].Name })
	return symbols
}

func collectMetrics(node *ast.Node, parameterCount int) Metrics {
	metrics := Metrics{LineCount: strings.Count(node.Content, "\n") + 1, ParameterCount: parameterCount}
	conditions := map[string]int{}
	var visit func(*ast.Node, int)
	visit = func(current *ast.Node, nesting int) {
		if current.Type == "if_statement" || current.Type == "conditional_expression" {
			condition := ast.ChildByField(current, "condition")
			if condition != nil {
				conditionText := strings.TrimSpace(condition.Content)
				if conditionText != "" {
					metrics.ConditionCount++
					conditions[conditionText]++
				}
			}
		}
		if current != node && isStatementOrDeclaration(current.Type) {
			metrics.StatementCount++
		}
		if isNestingNode(current.Type) {
			nesting++
			if nesting > metrics.MaxNesting {
				metrics.MaxNesting = nesting
			}
		}
		for _, child := range current.Children {
			visit(child, nesting)
		}
	}
	visit(node, 0)
	for _, count := range conditions {
		if count > 1 {
			metrics.RepeatedConditions += count - 1
		}
	}
	return metrics
}

func isStatementOrDeclaration(nodeType string) bool {
	return strings.HasSuffix(nodeType, "_statement") || strings.HasSuffix(nodeType, "_declaration") || nodeType == "expression_statement"
}

func isNestingNode(nodeType string) bool {
	switch nodeType {
	case "if_statement", "for_statement", "while_statement", "do_statement", "switch_statement", "expression_switch_statement", "try_statement", "catch_clause", "synchronized_statement":
		return true
	default:
		return false
	}
}

func collectEffects(node *ast.Node) []string {
	var effects []string
	if len(ast.FindAll(node, "throw_statement")) > 0 {
		effects = append(effects, "throws")
	}
	if len(ast.FindAll(node, "assignment_expression")) > 0 || len(ast.FindAll(node, "update_expression")) > 0 {
		effects = append(effects, "mutates_state")
	}
	return effects
}

func collectJavaTypes(root *ast.Node) (string, []Type) {
	packageNode := ast.FindFirst(root, "package_declaration")
	packageName := ""
	if packageNode != nil {
		packageName = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(packageNode.Content, "package")), ";")
	}
	var types []Type
	for _, nodeType := range []string{"class_declaration", "interface_declaration"} {
		for _, node := range ast.FindAll(root, nodeType) {
			name := ast.ChildByField(node, "name")
			if name == nil {
				continue
			}
			javaType := Type{Name: name.Content, Kind: strings.TrimSuffix(nodeType, "_declaration"), StartLine: node.StartLine, StartColumn: node.StartColumn}
			if superclass := ast.ChildByField(node, "superclass"); superclass != nil {
				javaType.Superclass = strings.TrimSpace(strings.TrimPrefix(superclass.Content, "extends"))
			}
			if interfaces := ast.ChildByField(node, "interfaces"); interfaces != nil {
				for _, typeNode := range ast.FindAll(interfaces, "type_identifier") {
					javaType.Interfaces = append(javaType.Interfaces, typeNode.Content)
				}
			}
			if body := ast.ChildByField(node, "body"); body != nil {
				for _, field := range ast.FindAll(body, "field_declaration") {
					if declarator := ast.ChildByField(field, "declarator"); declarator != nil {
						if fieldName := ast.ChildByField(declarator, "name"); fieldName != nil {
							javaType.Fields = append(javaType.Fields, fieldName.Content)
							if fieldType := fieldContent(field, "type"); fieldType != "" {
								if javaType.FieldTypes == nil {
									javaType.FieldTypes = make(map[string]string)
								}
								javaType.FieldTypes[fieldName.Content] = fieldType
							}
						}
					}
				}
			}
			types = append(types, javaType)
		}
	}
	return packageName, types
}

func collectNamespace(root *ast.Node) string {
	namespace := ast.FindFirst(root, "namespace_definition")
	if namespace == nil {
		return ""
	}
	name := ast.ChildByField(namespace, "name")
	if name == nil {
		return ""
	}
	return strings.TrimSpace(name.Content)
}

func fieldContent(node *ast.Node, field string) string {
	if child := ast.ChildByField(node, field); child != nil {
		return strings.TrimSpace(child.Content)
	}
	return ""
}

func parameters(node *ast.Node) []Parameter {
	list := ast.ChildByField(node, "parameters")
	if list == nil {
		if declarator := ast.ChildByField(node, "declarator"); declarator != nil {
			list = ast.ChildByField(declarator, "parameters")
		}
	}
	if list == nil {
		return nil
	}
	var result []Parameter
	for _, parameter := range list.Children {
		if parameter.Type != "formal_parameter" && parameter.Type != "parameter_declaration" {
			continue
		}
		name := ast.ChildByField(parameter, "name")
		if name == nil {
			name = ast.ChildByField(parameter, "declarator")
		}
		if name == nil {
			name = ast.FindFirst(parameter, "identifier")
		}
		if name == nil {
			continue
		}
		result = append(result, Parameter{Name: name.Content, Type: fieldContent(parameter, "type")})
	}
	return result
}

func visibility(node *ast.Node) string {
	if modifiers := ast.ChildByField(node, "modifiers"); modifiers != nil {
		return modifierVisibility(modifiers)
	}
	for _, child := range node.Children {
		if child.Type == "modifiers" {
			return modifierVisibility(child)
		}
	}
	return "package"
}

func modifierVisibility(modifiers *ast.Node) string {
	for _, modifier := range modifiers.Children {
		switch modifier.Type {
		case "public", "private", "protected", "static":
			if modifier.Type != "static" {
				return modifier.Type
			}
		}
	}
	return "package"
}

func qualifiedSymbolName(symbol Symbol) string {
	parts := []string{symbol.Package}
	if symbol.Class != "" {
		parts = append(parts, symbol.Class)
	}
	parts = append(parts, symbol.Name)
	return strings.Trim(strings.Join(parts, "."), ".")
}

func symbolSignature(symbol Symbol) string {
	types := make([]string, 0, len(symbol.Parameters))
	for _, parameter := range symbol.Parameters {
		types = append(types, parameter.Type)
	}
	return fmt.Sprintf("%s(%s)", symbol.QualifiedName, strings.Join(types, ","))
}

// FindSymbol resolves a qualified symbol and optional parameter types. A nil parameter list
// is accepted only when the qualified name is unambiguous.
func FindSymbol(index *Index, qualifiedName string, parameterTypes []string) (Symbol, error) {
	var matches []Symbol
	for _, file := range index.Files {
		for _, symbol := range file.Symbols {
			if symbol.QualifiedName != qualifiedName {
				continue
			}
			if parameterTypes == nil || sameParameterTypes(symbol, parameterTypes) {
				matches = append(matches, symbol)
			}
		}
	}
	if len(matches) == 0 {
		return Symbol{}, fmt.Errorf("symbol not found: %s", qualifiedName)
	}
	if len(matches) > 1 {
		return Symbol{}, fmt.Errorf("symbol is ambiguous: %s", qualifiedName)
	}
	return matches[0], nil
}

func sameParameterTypes(symbol Symbol, types []string) bool {
	if len(symbol.Parameters) == 0 && symbol.Signature != "" {
		expected := fmt.Sprintf("%s(%s)", symbol.QualifiedName, strings.Join(types, ","))
		return symbol.Signature == expected
	}
	if len(symbol.Parameters) != len(types) {
		return false
	}
	for index, parameter := range symbol.Parameters {
		if parameter.Type != types[index] {
			return false
		}
	}
	return true
}

func resolveTypeRelationships(index *Index) {
	types := map[string]string{}
	for _, file := range index.Files {
		for _, declared := range file.Types {
			qualified := strings.Trim(strings.Join([]string{file.Package, declared.Name}, "."), ".")
			types[qualified] = qualified
		}
	}
	for fileIndex := range index.Files {
		file := &index.Files[fileIndex]
		for typeIndex := range file.Types {
			declared := &file.Types[typeIndex]
			declared.SuperclassQualified = resolveTypeName(declared.Superclass, file.Package, types)
			for _, interfaceName := range declared.Interfaces {
				if resolved := resolveTypeName(interfaceName, file.Package, types); resolved != "" {
					declared.InterfacesQualified = append(declared.InterfacesQualified, resolved)
				}
			}
		}
	}
}

func resolveTypeName(name, packageName string, types map[string]string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if resolved, ok := types[name]; ok {
		return resolved
	}
	if resolved, ok := types[strings.Trim(strings.Join([]string{packageName, name}, "."), ".")]; ok {
		return resolved
	}
	var matches []string
	for qualified := range types {
		lastDot := strings.LastIndex(qualified, ".")
		if lastDot >= 0 && qualified[lastDot+1:] == name {
			matches = append(matches, qualified)
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}
	return ""
}

func symbolName(node *ast.Node) *ast.Node {
	if name := ast.ChildByField(node, "name"); name != nil {
		return name
	}
	if declarator := ast.ChildByField(node, "declarator"); declarator != nil {
		for _, nodeType := range []string{"identifier", "field_identifier", "name"} {
			if name := ast.FindFirst(declarator, nodeType); name != nil {
				return name
			}
		}
	}
	return nil
}

func collectInvocations(node *ast.Node, parameters []Parameter) []Invocation {
	var invocations []Invocation
	for _, call := range ast.FindAll(node, "call_expression") {
		invocations = append(invocations, Invocation{Name: callName(call.Content), ArgumentCount: argumentCount(call), ArgumentTypes: argumentTypes(call, parameters), Arguments: argumentValues(call), Content: call.Content, Kind: "call"})
	}
	for _, call := range ast.FindAll(node, "method_invocation") {
		name := fieldContent(call, "name")
		if name == "" {
			name = callName(call.Content)
		}
		receiver := fieldContent(call, "object")
		invocations = append(invocations, Invocation{Name: name, Receiver: receiver, ArgumentCount: argumentCount(call), ArgumentTypes: argumentTypes(call, parameters), Arguments: argumentValues(call), Content: call.Content, Kind: "method"})
	}
	for _, creation := range ast.FindAll(node, "object_creation_expression") {
		name := fieldContent(creation, "type")
		if name != "" {
			invocations = append(invocations, Invocation{Name: name, ArgumentCount: argumentCount(creation), ArgumentTypes: argumentTypes(creation, parameters), Arguments: argumentValues(creation), Content: creation.Content, Kind: "constructor"})
		}
	}
	sort.Slice(invocations, func(i, j int) bool {
		if invocations[i].Name == invocations[j].Name {
			return invocations[i].Receiver < invocations[j].Receiver
		}
		return invocations[i].Name < invocations[j].Name
	})
	return invocations
}

func argumentValues(node *ast.Node) []string {
	arguments := ast.ChildByField(node, "arguments")
	if arguments == nil {
		return nil
	}
	var values []string
	for _, child := range arguments.Children {
		if child.Type != "(" && child.Type != ")" && child.Type != "," {
			values = append(values, strings.TrimSpace(child.Content))
		}
	}
	return values
}

func argumentCount(node *ast.Node) int {
	arguments := ast.ChildByField(node, "arguments")
	if arguments == nil {
		return 0
	}
	count := 0
	for _, child := range arguments.Children {
		if child.Type != "(" && child.Type != ")" && child.Type != "," {
			count++
		}
	}
	return count
}

func argumentTypes(node *ast.Node, parameters []Parameter) []string {
	arguments := ast.ChildByField(node, "arguments")
	if arguments == nil {
		return nil
	}
	var types []string
	for _, child := range arguments.Children {
		if child.Type == "(" || child.Type == ")" || child.Type == "," {
			continue
		}
		argumentType := ""
		if child.Type == "identifier" {
			for _, parameter := range parameters {
				if parameter.Name == child.Content {
					argumentType = parameter.Type
					break
				}
			}
		}
		switch child.Type {
		case "int_literal", "decimal_integer_literal", "integer_literal":
			argumentType = "int"
		case "string_literal":
			argumentType = "String"
		case "true", "false", "boolean_literal":
			argumentType = "boolean"
		}
		types = append(types, argumentType)
	}
	return types
}

func resolveEdges(index *Index) []Edge {
	var symbols []Symbol
	for _, file := range index.Files {
		symbols = append(symbols, file.Symbols...)
	}
	edges := make([]Edge, 0)
	for _, source := range symbols {
		for _, invocation := range source.Invocations {
			matches := resolveInvocation(index, source, invocation)
			edge := Edge{From: qualifiedName(source), Call: invocation.Name, Status: "unresolved"}
			switch len(matches) {
			case 1:
				edge.To = qualifiedName(matches[0])
				edge.Status = "resolved"
			case 0:
				edge.Status = "external_or_unresolved"
			default:
				edge.Status = "ambiguous"
			}
			edges = append(edges, edge)
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			return edges[i].To < edges[j].To
		}
		return edges[i].From < edges[j].From
	})
	return edges
}

func resolveInvocation(index *Index, source Symbol, invocation Invocation) []Symbol {
	methodName := invocation.Name
	if invocation.Receiver == "" {
		if matches := functionsNamed(index, source.Package, methodName, invocation.ArgumentCount, invocation.ArgumentTypes); len(matches) > 0 {
			return matches
		}
	}
	classNames := receiverClasses(index, source, invocation.Receiver)
	if invocation.Receiver == "" {
		if source.Class != "" {
			classNames = append(classNames, qualifiedTypeName(source.Package, source.Class))
		}
		for _, file := range index.Files {
			if file.Package != source.Package {
				continue
			}
			for _, declared := range file.Types {
				classNames = append(classNames, qualifiedTypeName(file.Package, declared.Name))
			}
		}
		for _, imported := range importedTypes(index, source.File) {
			classNames = append(classNames, imported)
		}
	}
	var matches []Symbol
	for _, className := range uniqueStrings(classNames) {
		matches = append(matches, methodsOnType(index, className, methodName, invocation.ArgumentCount, invocation.ArgumentTypes, nil)...)
	}
	return uniqueSymbols(matches)
}

func functionsNamed(index *Index, packageName, name string, argumentCount int, argumentTypes []string) []Symbol {
	matches := make([]Symbol, 0)
	for _, file := range index.Files {
		for _, symbol := range file.Symbols {
			if symbol.Class == "" && symbol.Package == packageName && symbol.Name == name && (symbol.Kind == "function" || symbol.Kind == "method") && matchesInvocation(symbol, argumentCount, argumentTypes) {
				matches = append(matches, symbol)
			}
		}
	}
	return uniqueSymbols(matches)
}

func receiverClasses(index *Index, source Symbol, receiver string) []string {
	if receiver == "" {
		return nil
	}
	for _, parameter := range source.Parameters {
		if parameter.Name == receiver {
			return resolveTypeCandidates(index, source.Package, parameter.Type, importedTypes(index, source.File))
		}
	}
	for _, file := range index.Files {
		if file.Path != source.File {
			continue
		}
		for _, declared := range file.Types {
			if declared.Name != source.Class || declared.FieldTypes == nil {
				continue
			}
			if fieldType, ok := declared.FieldTypes[receiver]; ok {
				return resolveTypeCandidates(index, source.Package, fieldType, importedTypes(index, source.File))
			}
		}
	}
	return resolveTypeCandidates(index, source.Package, receiver, importedTypes(index, source.File))
}

func resolveTypeCandidates(index *Index, packageName, name string, imports []string) []string {
	var candidates []string
	for _, imported := range imports {
		if strings.HasSuffix(imported, "."+name) {
			candidates = append(candidates, imported)
		}
	}
	if len(candidates) > 0 {
		return uniqueStrings(candidates)
	}
	for _, file := range index.Files {
		for _, declared := range file.Types {
			qualified := qualifiedTypeName(file.Package, declared.Name)
			if name == qualified || name == declared.Name || name == strings.TrimPrefix(qualified, packageName+".") {
				candidates = append(candidates, qualified)
			}
		}
	}
	return uniqueStrings(candidates)
}

func methodsOnType(index *Index, className, methodName string, argumentCount int, argumentTypes []string, visited map[string]bool) []Symbol {
	if visited == nil {
		visited = map[string]bool{}
	}
	if visited[className] {
		return nil
	}
	visited[className] = true
	var matches []Symbol
	for _, file := range index.Files {
		for _, symbol := range file.Symbols {
			if symbol.Class == className || symbol.QualifiedName == className+"."+methodName {
				if symbol.Name == methodName && matchesInvocation(symbol, argumentCount, argumentTypes) {
					matches = append(matches, symbol)
				}
			}
		}
	}
	for _, file := range index.Files {
		for _, declared := range file.Types {
			if qualifiedTypeName(file.Package, declared.Name) != className {
				continue
			}
			if declared.SuperclassQualified != "" {
				matches = append(matches, methodsOnType(index, declared.SuperclassQualified, methodName, argumentCount, argumentTypes, visited)...)
			}
			for _, implemented := range declared.InterfacesQualified {
				matches = append(matches, methodsOnType(index, implemented, methodName, argumentCount, argumentTypes, visited)...)
			}
		}
	}
	return matches
}

func matchesInvocation(symbol Symbol, argumentCount int, argumentTypes []string) bool {
	if len(symbol.Parameters) != argumentCount {
		return false
	}
	if len(argumentTypes) != argumentCount {
		return true
	}
	for index, argumentType := range argumentTypes {
		if argumentType != "" && symbol.Parameters[index].Type != "" && argumentType != symbol.Parameters[index].Type {
			return false
		}
	}
	return true
}

func importedTypes(index *Index, filePath string) []string {
	for _, file := range index.Files {
		if file.Path != filePath {
			continue
		}
		var result []string
		for _, importText := range file.Imports {
			name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(importText, "import"), "static"), ";"))
			if !strings.HasSuffix(name, ".*") {
				result = append(result, name)
			}
		}
		return result
	}
	return nil
}

func uniqueSymbols(symbols []Symbol) []Symbol {
	seen := map[string]bool{}
	result := make([]Symbol, 0, len(symbols))
	for _, symbol := range symbols {
		if !seen[symbol.Signature] {
			seen[symbol.Signature] = true
			result = append(result, symbol)
		}
	}
	return result
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func qualifiedTypeName(packageName, typeName string) string {
	return strings.Trim(strings.Join([]string{packageName, typeName}, "."), ".")
}

func qualifiedName(symbol Symbol) string {
	if symbol.Signature != "" {
		return symbol.Signature
	}
	if symbol.QualifiedName != "" {
		return symbol.QualifiedName
	}
	return symbol.File + ":" + symbol.Name
}

func callName(content string) string {
	content = strings.TrimSpace(content)
	if index := strings.IndexByte(content, '('); index >= 0 {
		content = content[:index]
	}
	if index := strings.LastIndexAny(content, ".:"); index >= 0 {
		content = content[index+1:]
	}
	return strings.TrimSpace(content)
}

func shouldSkipDirectory(name string) bool {
	switch name {
	case ".git", ".axiom", "node_modules", "target", "build", "dist":
		return true
	default:
		return false
	}
}

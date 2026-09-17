package rewrite

import (
	"fmt"
	"strings"

	"github.com/nkosikhumalo/axiom/internal/ast"
	"github.com/nkosikhumalo/axiom/internal/project"
)

// JavaMethod is the language-neutral method representation used by the first rewrite pipeline.
type JavaMethod struct {
	Package    string
	Class      string
	Imports    []string
	Fields     []string
	Name       string
	Visibility string
	Static     bool
	ReturnType string
	Parameters []project.Parameter
	Body       string
}

// FromJavaASTWithContext converts an indexed Java symbol and its AST declaration into an
// intermediate method, including any imports and fields needed by the selected class.
func FromJavaASTWithContext(symbol project.Symbol, declaration *ast.Node, imports, fields []string) (*JavaMethod, error) {
	if declaration == nil {
		return nil, fmt.Errorf("method declaration is nil")
	}
	body := ast.ChildByField(declaration, "body")
	if body == nil {
		return nil, fmt.Errorf("method %s has no body", symbol.Name)
	}
	visibility := symbol.Visibility
	if visibility == "" || visibility == "package" {
		visibility = "public"
	}
	static := false
	if modifiers := ast.ChildByField(declaration, "modifiers"); modifiers != nil {
		for _, modifier := range modifiers.Children {
			if modifier.Type == "static" {
				static = true
			}
		}
	}
	for _, child := range declaration.Children {
		if child.Type != "static" && child.Type != "modifiers" {
			continue
		}
		if child.Type == "static" {
			static = true
		}
		for _, modifier := range child.Children {
			if modifier.Type == "static" {
				static = true
			}
		}
	}
	return &JavaMethod{
		Package:    symbol.Package,
		Class:      symbol.Class,
		Imports:    append([]string(nil), imports...),
		Fields:     append([]string(nil), fields...),
		Name:       symbol.Name,
		Visibility: visibility,
		Static:     static,
		ReturnType: symbol.ReturnType,
		Parameters: append([]project.Parameter(nil), symbol.Parameters...),
		Body:       body.Content,
	}, nil
}

// RenderJava renders the intermediate method into an isolated Java class.
func (method *JavaMethod) RenderJava(className string) string {
	var builder strings.Builder
	if method.Package != "" {
		fmt.Fprintf(&builder, "package %s;\n\n", method.Package)
	}
	for _, importLine := range method.Imports {
		fmt.Fprintf(&builder, "%s\n", importLine)
	}
	if len(method.Imports) > 0 {
		builder.WriteString("\n")
	}
	fmt.Fprintf(&builder, "/** Generated rewrite of %s.%s. Review before applying. */\n", method.Class, method.Name)
	fmt.Fprintf(&builder, "public final class %s {\n", className)
	for _, field := range method.Fields {
		for _, line := range strings.Split(strings.TrimSpace(field), "\n") {
			fmt.Fprintf(&builder, "    %s\n", strings.TrimSpace(line))
		}
	}
	if len(method.Fields) > 0 {
		builder.WriteString("\n")
	}
	static := ""
	if method.Static {
		static = " static"
	}
	fmt.Fprintf(&builder, "    %s%s %s %s(%s) %s\n", method.Visibility, static, javaType(method.ReturnType), method.Name, javaParameters(method.Parameters), method.Body)
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

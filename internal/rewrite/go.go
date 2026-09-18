package rewrite

import (
	"fmt"
	"strings"

	"github.com/nkosikhumalo/axiom/internal/project"
)

// RenderGo renders the intermediate method into an isolated Go source file.
// The original source is never touched; the output is written to a separate directory.
func (method *JavaMethod) RenderGo(packageName string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "package %s\n\n", goPackageName(packageName))
	fmt.Fprintf(&b, "// Generated rewrite of %s.%s. Review before applying.\n", method.Class, method.Name)

	returnType := goType(method.ReturnType)
	if returnType != "" {
		fmt.Fprintf(&b, "func %s(%s) %s {\n", goFuncName(method.Name), goParameters(method.Parameters), returnType)
	} else {
		fmt.Fprintf(&b, "func %s(%s) {\n", goFuncName(method.Name), goParameters(method.Parameters))
	}

	for _, line := range bodyLines(method.Body) {
		fmt.Fprintf(&b, "\t%s\n", translateLine(line))
	}

	b.WriteString("}\n")
	return b.String()
}

// bodyLines strips the outer braces of a Java block body and returns trimmed non-empty lines.
func bodyLines(body string) []string {
	body = strings.TrimSpace(body)
	body = strings.TrimPrefix(body, "{")
	body = strings.TrimSuffix(body, "}")
	var lines []string
	for _, line := range strings.Split(body, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

// translateLine applies token-level Java → Go transformations on a single statement.
func translateLine(line string) string {
	line = strings.TrimSuffix(strings.TrimSpace(line), ";")

	// Brace-only lines
	if line == "}" {
		return "}"
	}
	if line == "else {" {
		return "} else {"
	}
	if strings.HasPrefix(line, "else if") {
		return "} " + line
	}

	// Single-line `if (cond) return expr` — no braces in Java
	if strings.HasPrefix(line, "if (") {
		// Find the closing paren of the condition
		depth, closeIdx := 0, -1
		for i, ch := range line {
			if ch == '(' {
				depth++
			} else if ch == ')' {
				depth--
				if depth == 0 {
					closeIdx = i
					break
				}
			}
		}
		if closeIdx > 0 {
			cond := line[4:closeIdx] // strip leading "if ("
			rest := strings.TrimSpace(line[closeIdx+1:])
			if rest == "{" || rest == "" {
				// Block-style `if (cond) {`
				return "if " + cond + " {"
			}
			// Single-line `if (cond) return expr` → Go block form
			return "if " + cond + " {\n\t\t" + translateLine(rest) + "\n\t}"
		}
	}

	// `Type name = value` → `name := value`
	for _, jType := range []string{
		"int ", "long ", "double ", "float ",
		"boolean ", "String ", "char ", "byte ", "short ",
	} {
		if strings.HasPrefix(line, jType) {
			rest := strings.TrimPrefix(line, jType)
			if idx := strings.Index(rest, " = "); idx >= 0 {
				name := strings.TrimSpace(rest[:idx])
				value := strings.TrimSpace(rest[idx+3:])
				return name + " := " + value
			}
		}
	}

	return line
}

// goType maps Java primitive / common types to their Go equivalents.
func goType(javaType string) string {
	switch strings.TrimSpace(javaType) {
	case "int", "Integer":
		return "int"
	case "long", "Long":
		return "int64"
	case "double", "Double", "float", "Float":
		return "float64"
	case "boolean", "Boolean":
		return "bool"
	case "String":
		return "string"
	case "void", "":
		return ""
	default:
		return javaType
	}
}

// goParameters renders a Go-style parameter list from Java parameters.
func goParameters(parameters []project.Parameter) string {
	parts := make([]string, 0, len(parameters))
	for _, p := range parameters {
		parts = append(parts, fmt.Sprintf("%s %s", p.Name, goType(p.Type)))
	}
	return strings.Join(parts, ", ")
}

// goPackageName takes the last segment of a Java package name as the Go package.
func goPackageName(pkg string) string {
	if pkg == "" {
		return "generated"
	}
	parts := strings.Split(pkg, ".")
	return strings.ToLower(parts[len(parts)-1])
}

// goFuncName uppercases the first letter so the function is exported.
func goFuncName(name string) string {
	if name == "" {
		return "Run"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

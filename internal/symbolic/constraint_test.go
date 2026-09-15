package symbolic

import (
	"strings"
	"testing"

	"github.com/nkosikhumalo/axiom/internal/ast"
)

func TestExtractPathsStopsAfterReturn(t *testing.T) {
	root := &ast.Node{Type: "block", Children: []*ast.Node{
		{Type: "return_statement", Children: []*ast.Node{{Type: "int_literal", Content: "1"}}},
		{Type: "return_statement", Children: []*ast.Node{{Type: "int_literal", Content: "2"}}},
	}}

	paths := ExtractPaths(root)
	if len(paths) != 1 || paths[0].ReturnExpr != "1" {
		t.Fatalf("expected only the first return path, got %#v", paths)
	}
}

func TestExtractPathsTracksAssignment(t *testing.T) {
	root := &ast.Node{Type: "block", Children: []*ast.Node{
		{Type: "short_var_declaration", Children: []*ast.Node{
			{Type: "expression_list", FieldName: "left", Children: []*ast.Node{{Type: "identifier", Content: "y"}}},
			{Type: ":="},
			{Type: "expression_list", FieldName: "right", Children: []*ast.Node{{Type: "identifier", Content: "x"}}},
		}},
		{Type: "return_statement", Children: []*ast.Node{{Type: "identifier", Content: "y"}}},
	}}

	paths := ExtractPaths(root)
	if len(paths) != 1 || paths[0].ReturnExpr != "x" {
		t.Fatalf("expected assigned symbol in return, got %#v", paths)
	}
}

func TestBuildFunctionSMTSupportsTernary(t *testing.T) {
	expr := BuildExpression(&ast.Node{Type: "conditional_expression", Children: []*ast.Node{
		{Type: "identifier", Content: "ok"},
		{Type: "?"},
		{Type: "int_literal", Content: "1"},
		{Type: ":"},
		{Type: "int_literal", Content: "0"},
	}})
	if expr == nil || !strings.Contains(expr.ToSMT(), "(ite ok 1 0)") {
		t.Fatalf("expected ternary SMT expression, got %v", expr)
	}
}

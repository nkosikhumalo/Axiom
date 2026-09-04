package ast

import (
	sitter "github.com/smacker/go-tree-sitter"
)

// Node is a simplified, language-agnostic AST node.
type Node struct {
	Type        string
	Content     string
	FieldName   string // named field from parent (e.g. "condition", "consequence")
	Children    []*Node
	StartLine   uint32
	StartColumn uint32
}

// Walk traverses a tree-sitter tree into a language-agnostic Node tree.
func Walk(tree *ParsedTree) *Node {
	return walkNode(tree.Tree.RootNode(), tree.Source, "")
}

// FindAll returns all nodes matching the given type anywhere in the subtree.
func FindAll(root *Node, nodeType string) []*Node {
	var result []*Node
	findAll(root, nodeType, &result)
	return result
}

// FindFirst returns the first node matching the given type in the subtree.
func FindFirst(root *Node, nodeType string) *Node {
	nodes := FindAll(root, nodeType)
	if len(nodes) == 0 {
		return nil
	}
	return nodes[0]
}

// ChildByField returns the first child with the given field name.
func ChildByField(node *Node, field string) *Node {
	for _, c := range node.Children {
		if c.FieldName == field {
			return c
		}
	}
	return nil
}

func walkNode(n *sitter.Node, source []byte, fieldName string) *Node {
	node := &Node{
		Type:        n.Type(),
		Content:     n.Content(source),
		FieldName:   fieldName,
		StartLine:   n.StartPoint().Row + 1,
		StartColumn: n.StartPoint().Column,
	}

	for i := 0; i < int(n.ChildCount()); i++ {
		child := n.Child(i)
		// Resolve the named field for this child position if available.
		field := n.FieldNameForChild(i)
		node.Children = append(node.Children, walkNode(child, source, field))
	}

	return node
}

func findAll(node *Node, nodeType string, out *[]*Node) {
	if node == nil {
		return
	}
	if node.Type == nodeType {
		*out = append(*out, node)
	}
	for _, c := range node.Children {
		findAll(c, nodeType, out)
	}
}

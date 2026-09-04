package ast

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/php"
)

// Language represents a supported source language.
type Language string

const (
	LangGo   Language = "go"
	LangCPP  Language = "cpp"
	LangJava Language = "java"
	LangPHP  Language = "php"
)

// ParsedTree holds the tree-sitter parse result for a source file.
type ParsedTree struct {
	Tree     *sitter.Tree
	Source   []byte
	Language Language
}

// ParseFile reads a file from disk, detects its language by extension, and parses it.
func ParseFile(path string) (*ParsedTree, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	lang, err := detectLanguage(path)
	if err != nil {
		return nil, err
	}

	return Parse(source, lang)
}

// Parse parses source bytes for the given language.
func Parse(source []byte, lang Language) (*ParsedTree, error) {
	sitterLang, err := resolveLanguage(lang)
	if err != nil {
		return nil, err
	}

	parser := sitter.NewParser()
	parser.SetLanguage(sitterLang)

	tree, err := parser.ParseCtx(context.Background(), nil, source)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	return &ParsedTree{
		Tree:     tree,
		Source:   source,
		Language: lang,
	}, nil
}

// detectLanguage infers the Language from a file extension.
func detectLanguage(path string) (Language, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return LangGo, nil
	case ".cpp", ".cc", ".cxx", ".c":
		return LangCPP, nil
	case ".java":
		return LangJava, nil
	case ".php":
		return LangPHP, nil
	default:
		return "", fmt.Errorf("unsupported file extension: %s", filepath.Ext(path))
	}
}

// resolveLanguage maps a Language constant to its tree-sitter grammar.
func resolveLanguage(lang Language) (*sitter.Language, error) {
	switch lang {
	case LangGo:
		return golang.GetLanguage(), nil
	case LangCPP:
		return cpp.GetLanguage(), nil
	case LangJava:
		return java.GetLanguage(), nil
	case LangPHP:
		return php.GetLanguage(), nil
	default:
		return nil, fmt.Errorf("unsupported language: %s", lang)
	}
}

package project

import (
	"fmt"
	"strings"
)

// ResolveEntry finds an unambiguous fully qualified symbol. An optional signature can be
// supplied as `pkg.Type.method(Type1,Type2)` to select an overload.
func ResolveEntry(index *Index, entry string) (Symbol, error) {
	name, parameterTypes, hasSignature, err := parseEntry(entry)
	if err != nil {
		return Symbol{}, err
	}
	if !hasSignature {
		parameterTypes = nil
	}
	return FindSymbol(index, name, parameterTypes)
}

// Reachable returns a copy of index containing the selected entry and every symbol reachable
// through resolved dependency edges. Unresolved and external edges from reachable symbols are
// retained so callers can see what prevented a complete proof.
func Reachable(index *Index, entry string) (*Index, error) {
	entrySymbol, err := ResolveEntry(index, entry)
	if err != nil {
		return nil, err
	}
	entryKey := qualifiedName(entrySymbol)

	edgesBySource := map[string][]Edge{}
	for _, edge := range index.Edges {
		edgesBySource[edge.From] = append(edgesBySource[edge.From], edge)
	}
	reachable := map[string]bool{entryKey: true}
	queue := []string{entryKey}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range edgesBySource[current] {
			if edge.Status != "resolved" || edge.To == "" || reachable[edge.To] {
				continue
			}
			reachable[edge.To] = true
			queue = append(queue, edge.To)
		}
	}

	result := &Index{
		Root:        index.Root,
		Entry:       entry,
		EntryFile:   entrySymbol.File,
		EntryLine:   entrySymbol.StartLine,
		EntryColumn: entrySymbol.StartColumn,
		Reachable:   true,
		Files:       make([]File, 0),
		Edges:       make([]Edge, 0),
	}
	for _, file := range index.Files {
		var symbols []Symbol
		for _, symbol := range file.Symbols {
			if reachable[qualifiedName(symbol)] {
				symbols = append(symbols, symbol)
			}
		}
		if len(symbols) == 0 {
			continue
		}
		file.Symbols = symbols
		result.Files = append(result.Files, file)
	}
	for _, edge := range index.Edges {
		if reachable[edge.From] {
			result.Edges = append(result.Edges, edge)
		}
	}
	AnalyzeSummaries(result)
	return result, nil
}

func parseEntry(entry string) (string, []string, bool, error) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return "", nil, false, fmt.Errorf("entry symbol cannot be empty")
	}
	open := strings.IndexByte(entry, '(')
	if open < 0 {
		return entry, nil, false, nil
	}
	if !strings.HasSuffix(entry, ")") {
		return "", nil, false, fmt.Errorf("invalid entry signature: %s", entry)
	}
	name := strings.TrimSpace(entry[:open])
	arguments := strings.TrimSpace(entry[open+1 : len(entry)-1])
	if name == "" {
		return "", nil, false, fmt.Errorf("invalid entry signature: %s", entry)
	}
	if arguments == "" {
		return name, []string{}, true, nil
	}
	parts := strings.Split(arguments, ",")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
		if parts[index] == "" {
			return "", nil, false, fmt.Errorf("invalid entry signature: %s", entry)
		}
	}
	return name, parts, true, nil
}

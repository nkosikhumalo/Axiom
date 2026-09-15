package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nkosikhumalo/axiom/internal/project"
	"github.com/nkosikhumalo/axiom/internal/verify"
)

// WriteProject writes a project index as JSON or SARIF.
func WriteProject(path, format string, index *project.Index) error {
	if format == "json" {
		return writeJSON(path, index)
	}
	if format == "sarif" {
		return writeJSON(path, projectSARIF(index))
	}
	return fmt.Errorf("unsupported report format: %s", format)
}

// WriteVerification writes a project verification outcome as JSON or SARIF.
func WriteVerification(path, format string, outcome *verify.Outcome) error {
	if format == "json" {
		return writeJSON(path, outcome)
	}
	if format == "sarif" {
		return writeJSON(path, verificationSARIF(outcome))
	}
	return fmt.Errorf("unsupported report format: %s", format)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

type sarif struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name string `json:"name"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	Physical sarifPhysical `json:"physicalLocation"`
}

type sarifPhysical struct {
	Artifact sarifArtifact `json:"artifactLocation"`
	Region   *sarifRegion  `json:"region,omitempty"`
}

type sarifArtifact struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine uint32 `json:"startLine,omitempty"`
}

func projectSARIF(index *project.Index) sarif {
	results := make([]sarifResult, 0)
	locations := symbolLocations(index)
	if index.Entry != "" {
		results = append(results, sarifResult{
			RuleID:    "selected-entry",
			Level:     "note",
			Message:   sarifMessage{Text: "Selected entry: " + index.Entry},
			Locations: locations[index.Entry],
		})
	}
	for _, file := range index.Files {
		if file.Diagnostic != "" {
			results = append(results, sarifResult{RuleID: "parse-error", Level: "error", Message: sarifMessage{Text: file.Diagnostic}, Locations: []sarifLocation{{Physical: sarifPhysical{Artifact: sarifArtifact{URI: file.Path}}}}})
		}
	}
	for _, edge := range index.Edges {
		level := "note"
		if edge.Status != "resolved" {
			level = "warning"
		}
		results = append(results, sarifResult{RuleID: "dependency-edge", Level: level, Message: sarifMessage{Text: edge.From + " calls " + edge.Call + " (" + edge.Status + ")"}, Locations: locations[edge.From]})
	}
	for _, assumption := range index.Assumptions {
		level := "warning"
		if assumption.Severity == "error" {
			level = "error"
		}
		results = append(results, sarifResult{RuleID: assumption.Kind, Level: level, Message: sarifMessage{Text: assumption.Symbol + " calls " + assumption.Call + ": " + assumption.Reason}, Locations: locations[assumption.Symbol]})
	}
	for _, summary := range index.Summaries {
		if summary.Classification == project.ClassificationUnsupported {
			results = append(results, sarifResult{RuleID: "unsupported-behavior", Level: "error", Message: sarifMessage{Text: summary.Symbol + ": " + strings.Join(summary.Reasons, "; ")}, Locations: locations[summary.Symbol]})
		}
	}
	return sarifDocument(results)
}

func verificationSARIF(outcome *verify.Outcome) sarif {
	level := "note"
	if outcome.Status == verify.StatusNotEquivalent {
		level = "error"
	} else if outcome.Status == verify.StatusCouldNotProve || outcome.Status == verify.StatusEquivalentAssumptions {
		level = "warning"
	}
	message := outcome.Reason
	if outcome.Countermodel != "" {
		message += " Counterexample: " + outcome.Countermodel
	}
	result := sarifResult{RuleID: outcome.Status, Level: level, Message: sarifMessage{Text: message}}
	return sarifDocument([]sarifResult{result})
}

func symbolLocations(index *project.Index) map[string][]sarifLocation {
	locations := make(map[string][]sarifLocation)
	for _, file := range index.Files {
		for _, symbol := range file.Symbols {
			name := symbol.Signature
			if name == "" {
				name = symbol.QualifiedName
			}
			if name == "" {
				continue
			}
			region := (*sarifRegion)(nil)
			if symbol.StartLine > 0 {
				region = &sarifRegion{StartLine: symbol.StartLine}
			}
			locations[name] = []sarifLocation{{Physical: sarifPhysical{Artifact: sarifArtifact{URI: file.Path}, Region: region}}}
		}
	}
	return locations
}

func sarifDocument(results []sarifResult) sarif {
	return sarif{Version: "2.1.0", Schema: "https://json.schemastore.org/sarif-2.1.0.json", Runs: []sarifRun{{Tool: sarifTool{Driver: sarifDriver{Name: "Axiom"}}, Results: results}}}
}

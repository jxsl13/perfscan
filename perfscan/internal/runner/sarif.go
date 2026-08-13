package runner

import (
	"encoding/json"
	"io"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// SARIF 2.1.0 output for GitHub Code Scanning and other SARIF consumers.

type sarifLog struct {
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
	Name           string      `json:"name"`
	Version        string      `json:"version,omitempty"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string            `json:"id"`
	ShortDescription sarifText         `json:"shortDescription"`
	HelpURI          string            `json:"helpUri"`
	Properties       map[string]string `json:"properties,omitempty"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID string `json:"ruleId"`
	// RuleIndex is the 0-based position of RuleID in the run's tool.driver.rules
	// array — the SARIF-recommended way to resolve a result to its rule, robust
	// when a ruleId string is ambiguous. GitHub Code Scanning honors it.
	RuleIndex int             `json:"ruleIndex"`
	Level     string          `json:"level"`
	Message   sarifText       `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysical `json:"physicalLocation"`
}

type sarifPhysical struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           sarifRegion   `json:"region"`
}

type sarifArtifact struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// Version is stamped by the CLI so SARIF logs carry the release.
var Version = "dev"

func emitSARIF(w io.Writer, findings []Finding) {
	ruleIndex := make(map[string]int, len(findings))
	rules := make([]sarifRule, 0, len(findings))
	results := make([]sarifResult, 0, len(findings))
	for _, f := range findings {
		if _, ok := ruleIndex[f.Check.ID]; !ok {
			ruleIndex[f.Check.ID] = len(rules)
			rules = append(rules, sarifRule{
				ID:               f.Check.ID,
				ShortDescription: sarifText{Text: f.Check.Doc.Title},
				HelpURI:          "https://github.com/jxsl13/perfscan/perfscan/blob/main/docs/checks/" + f.Check.ID + ".md",
				Properties: map[string]string{
					"category": f.Check.Category,
					"fixLevel": f.Check.Level.String(),
				},
			})
		}
		results = append(results, sarifResult{
			RuleID:    f.Check.ID,
			RuleIndex: ruleIndex[f.Check.ID],
			Level:     sarifLevel(f.Check.Level),
			Message:   sarifText{Text: f.Message},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysical{
					ArtifactLocation: sarifArtifact{URI: relPath(f.Pos.Filename)},
					Region: sarifRegion{
						StartLine:   f.Pos.Line,
						StartColumn: f.Pos.Column,
						EndLine:     f.End.Line,
						EndColumn:   f.End.Column,
					},
				},
			}},
		})
	}
	log := sarifLog{
		Version: "2.1.0",
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "perfscan",
				Version:        Version,
				InformationURI: "https://github.com/jxsl13/perfscan/perfscan",
				Rules:          rules,
			}},
			Results: results,
		}},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(log)
}

// sarifLevel maps fix levels to SARIF severities: L1/L2 findings are
// ordinary warnings; L3 advisories are notes (they require benchmarks and
// judgment, not action).
func sarifLevel(l lint.Level) string {
	if l >= lint.LevelAggressive {
		return "note"
	}
	return "warning"
}

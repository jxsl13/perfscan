// Package report renders clang-tidy diagnostics in perfscan's output
// formats: human text, JSON and SARIF 2.1.0 — with catalog metadata (PX id,
// fix level) attached and -level gating applied.
package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/jxsl13/perfscanxx/internal/catalog"
	"github.com/jxsl13/perfscanxx/internal/fixes"
)

// Finding is one reportable diagnostic, enriched with catalog metadata.
type Finding struct {
	// ID is the PX catalog id, or the raw clang-tidy name for
	// pass-through diagnostics (clang-diagnostic-*).
	ID       string `json:"id"`
	TidyName string `json:"check"`
	Level    string `json:"level,omitempty"`
	Category string `json:"category,omitempty"`
	Message  string `json:"message"`
	File     string `json:"file"`
	// Line/Col are 1-based and derived from the byte offset; 0 when the
	// source file could not be read.
	Line   int `json:"line,omitempty"`
	Col    int `json:"col,omitempty"`
	Offset int `json:"offset"`
	// Fixes counts the fix-it replacements clang-tidy offered.
	Fixes int `json:"fixes"`
}

// ReadFile is the file loader used to translate byte offsets into
// line:column; a variable so tests can inject sources.
var ReadFile = os.ReadFile

// FromExport converts a parsed --export-fixes document into findings,
// applying catalog lookup and -level gating.
//
// Diagnostics from checks outside the curated catalog (in practice only
// clang-diagnostic-* compile errors, since perfscanxx runs clang-tidy with
// -checks=-*,<curated>) pass through ungated so a broken build is never
// silent.
func FromExport(ef *fixes.ExportFile, maxLevel catalog.Level) []Finding {
	var out []Finding
	for _, d := range ef.Diagnostics {
		f := Finding{
			ID:       d.DiagnosticName,
			TidyName: d.DiagnosticName,
			Message:  d.DiagnosticMessage.Message,
			File:     d.DiagnosticMessage.FilePath,
			Offset:   d.DiagnosticMessage.FileOffset,
			Fixes:    len(d.DiagnosticMessage.Replacements),
		}
		if e, ok := catalog.ByTidyName(d.DiagnosticName); ok {
			if e.Level > maxLevel {
				continue
			}
			f.ID = e.ID
			f.Level = e.Level.String()
			f.Category = e.Category
		}
		f.Line, f.Col = lineCol(f.File, f.Offset)
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Offset < out[j].Offset
	})
	return out
}

// lineCol maps a byte offset to a 1-based line:column pair by reading the
// source file. It returns (0, 0) when the file is unreadable or the offset
// is out of range.
func lineCol(path string, offset int) (line, col int) {
	if path == "" {
		return 0, 0
	}
	data, err := ReadFile(path)
	if err != nil || offset < 0 || offset > len(data) {
		return 0, 0
	}
	prefix := data[:offset]
	line = 1 + bytes.Count(prefix, []byte{'\n'})
	last := bytes.LastIndexByte(prefix, '\n')
	col = offset - last // last == -1 for line 1 → col = offset+1
	return line, col
}

// Text renders findings in perfscan's human format:
//
//	file.cpp:12:9: message (PX1001 L1, fix available)
func Text(w io.Writer, findings []Finding) {
	for _, f := range findings {
		pos := fmt.Sprintf("%s:#%d", f.File, f.Offset)
		if f.Line > 0 {
			pos = fmt.Sprintf("%s:%d:%d", f.File, f.Line, f.Col)
		}
		meta := f.ID
		if f.Level != "" {
			meta += " " + f.Level
		}
		if f.Fixes > 0 {
			meta += ", fix available"
		}
		fmt.Fprintf(w, "%s: %s (%s)\n", pos, f.Message, meta)
	}
}

// JSON renders findings as a JSON array (perfscan -json analog).
func JSON(w io.Writer, findings []Finding) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if findings == nil {
		findings = []Finding{}
	}
	return enc.Encode(findings)
}

// SARIF renders findings as a minimal SARIF 2.1.0 log
// (GitHub Code Scanning compatible).
func SARIF(w io.Writer, findings []Finding) error {
	type sarifRule struct {
		ID   string `json:"id"`
		Name string `json:"name,omitempty"`
	}
	type sarifMessage struct {
		Text string `json:"text"`
	}
	type sarifRegion struct {
		StartLine   int `json:"startLine,omitempty"`
		StartColumn int `json:"startColumn,omitempty"`
	}
	type sarifArtifactLocation struct {
		URI string `json:"uri"`
	}
	type sarifPhysicalLocation struct {
		ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
		Region           *sarifRegion          `json:"region,omitempty"`
	}
	type sarifLocation struct {
		PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
	}
	type sarifResult struct {
		RuleID    string          `json:"ruleId"`
		Message   sarifMessage    `json:"message"`
		Locations []sarifLocation `json:"locations"`
	}
	type sarifDriver struct {
		Name    string      `json:"name"`
		Rules   []sarifRule `json:"rules"`
		Version string      `json:"version,omitempty"`
	}
	type sarifTool struct {
		Driver sarifDriver `json:"driver"`
	}
	type sarifRun struct {
		Tool    sarifTool     `json:"tool"`
		Results []sarifResult `json:"results"`
	}
	type sarifLog struct {
		Schema  string     `json:"$schema"`
		Version string     `json:"version"`
		Runs    []sarifRun `json:"runs"`
	}

	seen := map[string]bool{}
	rules := []sarifRule{}
	results := make([]sarifResult, 0, len(findings))
	for _, f := range findings {
		if !seen[f.ID] {
			seen[f.ID] = true
			rules = append(rules, sarifRule{ID: f.ID, Name: f.TidyName})
		}
		var region *sarifRegion
		if f.Line > 0 {
			region = &sarifRegion{StartLine: f.Line, StartColumn: f.Col}
		}
		results = append(results, sarifResult{
			RuleID:  f.ID,
			Message: sarifMessage{Text: f.Message},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: f.File},
					Region:           region,
				},
			}},
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(sarifLog{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool:    sarifTool{Driver: sarifDriver{Name: "perfscanxx", Rules: rules}},
			Results: results,
		}},
	})
}

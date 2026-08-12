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
	"path/filepath"
	"sort"
	"strconv"

	"github.com/jxsl13/perfscan/perfscanxx/internal/catalog"
	"github.com/jxsl13/perfscan/perfscanxx/internal/fixes"
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
		// clang-tidy writes the diagnostic FilePath RELATIVE to its
		// BuildDirectory (the -p build dir); MainSourceFile is absolute.
		// Resolve to an absolute path for reading, and display it relative
		// to the current working directory (perfscan-style).
		abs := resolvePath(d.DiagnosticMessage.FilePath, d.BuildDirectory, ef.MainSourceFile)
		f := Finding{
			ID:       d.DiagnosticName,
			TidyName: d.DiagnosticName,
			Message:  d.DiagnosticMessage.Message,
			File:     displayPath(abs),
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
		f.Line, f.Col = lineCol(abs, f.Offset)
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

// ResolvePath is the exported form of resolvePath, used by the diff package to
// map a Replacement's FilePath to an absolute path the same way findings are.
func ResolvePath(filePath, buildDir, mainSrc string) string {
	return resolvePath(filePath, buildDir, mainSrc)
}

// DisplayPath is the exported form of displayPath: render an absolute path
// relative to the cwd for readable output.
func DisplayPath(abs string) string { return displayPath(abs) }

// resolvePath makes a diagnostic's (possibly BuildDirectory-relative)
// FilePath absolute: an already-absolute path is returned as-is; otherwise
// it is joined onto the BuildDirectory, falling back to the directory of the
// absolute MainSourceFile when no BuildDirectory was exported.
func resolvePath(filePath, buildDir, mainSrc string) string {
	if filePath == "" {
		return mainSrc
	}
	if filepath.IsAbs(filePath) {
		return filepath.Clean(filePath)
	}
	if buildDir != "" {
		return filepath.Clean(filepath.Join(buildDir, filePath))
	}
	if mainSrc != "" {
		return filepath.Clean(filepath.Join(filepath.Dir(mainSrc), filePath))
	}
	return filePath
}

// displayPath renders an absolute path relative to the current working
// directory when that is shorter and stays within the tree, else returns the
// path unchanged — so findings read like "examples/sample.cpp", not a long
// absolute path.
func displayPath(abs string) string {
	if abs == "" || !filepath.IsAbs(abs) {
		return abs
	}
	cwd, err := os.Getwd()
	if err != nil {
		return abs
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil || len(rel) == 0 || rel == "." || (len(rel) >= 2 && rel[0] == '.' && rel[1] == '.') {
		return abs
	}
	return rel
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
		// Concatenation + strconv instead of fmt.Sprintf: this runs once per
		// finding and avoids the format-string parse and its allocations.
		pos := f.File + ":#" + strconv.Itoa(f.Offset)
		if f.Line > 0 {
			pos = f.File + ":" + strconv.Itoa(f.Line) + ":" + strconv.Itoa(f.Col)
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
	type sarifText struct {
		Text string `json:"text"`
	}
	type sarifRule struct {
		ID               string     `json:"id"`
		Name             string     `json:"name,omitempty"`
		ShortDescription *sarifText `json:"shortDescription,omitempty"`
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
		RuleIndex int             `json:"ruleIndex"`
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

	ruleIndex := map[string]int{}
	rules := []sarifRule{}
	results := make([]sarifResult, 0, len(findings))
	for _, f := range findings {
		if _, ok := ruleIndex[f.ID]; !ok {
			ruleIndex[f.ID] = len(rules)
			r := sarifRule{ID: f.ID, Name: f.TidyName}
			// Enrich with the catalog's one-line summary so GitHub Code
			// Scanning shows what each PX rule means, not just its id.
			// Pass-through diagnostics (clang-diagnostic-*) aren't in the
			// catalog and simply carry no shortDescription.
			if e, ok := catalog.ByID(f.ID); ok && e.Title != "" {
				r.ShortDescription = &sarifText{Text: e.Title}
			}
			rules = append(rules, r)
		}
		var region *sarifRegion
		if f.Line > 0 {
			region = &sarifRegion{StartLine: f.Line, StartColumn: f.Col}
		}
		results = append(results, sarifResult{
			RuleID:    f.ID,
			RuleIndex: ruleIndex[f.ID],
			Message:   sarifMessage{Text: f.Message},
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

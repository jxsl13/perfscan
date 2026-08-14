// Package report renders clang-tidy diagnostics in perfscan's output
// formats: human text, JSON and SARIF 2.1.0 — with catalog metadata (PX id,
// fix level) attached and -level gating applied.
package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/jxsl13/perfscan/perfscanxx/internal/catalog"
	"github.com/jxsl13/perfscan/perfscanxx/internal/fixes"
)

// ToolVersion is stamped into the SARIF tool.driver.version so GitHub Code
// Scanning can track and dedup results across perfscanxx versions. main sets it
// from its ldflags-injected version; it defaults to "dev" for tests and
// go-run builds.
var ToolVersion = "dev"

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
// fixItCount is the total number of fix-it edits a diagnostic carries — the main
// message's replacements PLUS any in its notes. clang-tidy --fix applies all of
// them, so counting only the main message would under-report "fix available" (and
// the -json/-list fix count) for a check that emits its fix-it in a note.
func fixItCount(d fixes.Diagnostic) int {
	n := len(d.DiagnosticMessage.Replacements)
	for _, note := range d.Notes {
		n += len(note.Replacements)
	}
	return n
}

func FromExport(ef *fixes.ExportFile, maxLevel catalog.Level) []Finding {
	var out []Finding
	// Deduplicate diagnostics that repeat across translation units. When
	// clang-tidy runs over several TUs, a diagnostic anchored in a SHARED HEADER
	// is written to --export-fixes once per TU that includes that header — so a
	// header-heavy C++ project (e.g. fmt, where format.h is compiled into several
	// .cc TUs) would otherwise report the same finding N times and inflate the
	// count. clang-tidy's own console output collapses these; we match that by
	// keying on the diagnostic's identity (resolved file + byte offset + check
	// name). Two DIFFERENT checks at one offset have different names and are kept.
	seen := map[string]bool{}
	for _, d := range ef.Diagnostics {
		// clang-tidy writes the diagnostic FilePath RELATIVE to its
		// BuildDirectory (the -p build dir); MainSourceFile is absolute.
		// Resolve to an absolute path for reading, and display it relative
		// to the current working directory (perfscan-style).
		abs := resolvePath(d.DiagnosticMessage.FilePath, d.BuildDirectory, ef.MainSourceFile)
		key := abs + "\x00" + strconv.Itoa(d.DiagnosticMessage.FileOffset) + "\x00" + d.DiagnosticName
		if seen[key] {
			continue
		}
		seen[key] = true
		f := Finding{
			ID:       d.DiagnosticName,
			TidyName: d.DiagnosticName,
			Message:  d.DiagnosticMessage.Message,
			File:     displayPath(abs),
			Offset:   d.DiagnosticMessage.FileOffset,
			Fixes:    fixItCount(d),
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
	// Total order: file, then byte offset, then check ID, then message. The ID
	// and message tiebreakers matter when two DIFFERENT checks fire at the same
	// file+offset — their relative order in --export-fixes follows clang-tidy's
	// TU processing order, which varies run-to-run (parallel runs, TU ordering)
	// and across clang-tidy versions. Without the tiebreakers those co-located
	// findings could reorder between otherwise-identical runs, making CI/report
	// diffs noisy; a fully-specified order keeps the output stable.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Offset != out[j].Offset {
			return out[i].Offset < out[j].Offset
		}
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Message < out[j].Message
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
			// Flag a fix whose upstream fix-it is unsafe to apply blindly
			// (PX3004/PX3007/PX3015/PX3027) right where the user decides whether
			// to -fix — the default text output. The full rationale stays in
			// -explain (and -json/-sarif carry it too); here a compact marker is
			// enough to stop a blind -fix. Only caveated checks get it.
			if e, ok := catalog.ByID(f.ID); ok && e.Caveat != "" {
				meta += " (⚠ caveat — see -explain " + f.ID + ")"
			}
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

// lineIndependentFingerprint hashes a finding's line-independent identity — its
// check id, file and message, the same triple the baseline ratchet keys on —
// into a compact stable hex value for SARIF partialFingerprints. GitHub Code
// Scanning uses it to track a result across commits, so a finding is not closed
// and reopened when unrelated lines shift above it (the line/column in the
// physical location moves, but the fingerprint does not).
func lineIndependentFingerprint(f Finding) string {
	h := fnv.New64a()
	io.WriteString(h, f.ID)
	h.Write([]byte{0})
	io.WriteString(h, f.File)
	h.Write([]byte{0})
	io.WriteString(h, f.Message)
	return strconv.FormatUint(h.Sum64(), 16)
}

// SARIF renders findings as a minimal SARIF 2.1.0 log
// (GitHub Code Scanning compatible).
func SARIF(w io.Writer, findings []Finding) error {
	type sarifText struct {
		Text string `json:"text"`
	}
	type sarifConfig struct {
		Level string `json:"level"`
	}
	type sarifRuleProps struct {
		Tags []string `json:"tags,omitempty"`
	}
	type sarifRule struct {
		ID                   string          `json:"id"`
		Name                 string          `json:"name,omitempty"`
		ShortDescription     *sarifText      `json:"shortDescription,omitempty"`
		FullDescription      *sarifText      `json:"fullDescription,omitempty"`
		HelpURI              string          `json:"helpUri,omitempty"`
		DefaultConfiguration *sarifConfig    `json:"defaultConfiguration,omitempty"`
		Properties           *sarifRuleProps `json:"properties,omitempty"`
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
		Level     string          `json:"level,omitempty"`
		Message   sarifMessage    `json:"message"`
		Locations []sarifLocation `json:"locations"`
		// PartialFingerprints give GitHub Code Scanning a LINE-INDEPENDENT
		// identity for the result, so a finding is tracked across commits and
		// not closed+reopened when unrelated lines shift above it. The key is
		// (id, file, message) — the same line-independent identity the baseline
		// ratchet uses — hashed for a compact stable value.
		PartialFingerprints map[string]string `json:"partialFingerprints,omitempty"`
	}

	// sarifLevel maps a catalog level to a SARIF result level for GitHub Code
	// Scanning triage. L3 is the aggressive/niche tier the catalog documents as
	// "must never surface by default", so it is a lower-priority "note"; the
	// actionable L1/L2 findings are "warning" (nothing here is an "error").
	sarifLevel := func(level string) string {
		if level == "L3" {
			return "note"
		}
		return "warning"
	}
	type sarifDriver struct {
		Name           string      `json:"name"`
		InformationURI string      `json:"informationUri"`
		Rules          []sarifRule `json:"rules"`
		Version        string      `json:"version,omitempty"`
	}
	type sarifTool struct {
		Driver sarifDriver `json:"driver"`
	}
	// GitHub Code Scanning uses run.automationDetails.id to categorize an
	// analysis: the portion before the trailing "/" is the category. Without
	// it, a repo that uploads perfscanxx's SARIF alongside another tool's (or
	// a second perfscanxx run, e.g. a matrix build) on the same commit can have
	// the two runs treated as the same analysis, so the later upload CLOBBERS
	// the earlier one's results. A stable "perfscanxx/" category keeps our
	// findings a distinct, non-overwriting analysis.
	type sarifAutomationDetails struct {
		ID string `json:"id"`
	}
	type sarifRun struct {
		Tool              sarifTool               `json:"tool"`
		AutomationDetails *sarifAutomationDetails `json:"automationDetails,omitempty"`
		Results           []sarifResult           `json:"results"`
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
			r := sarifRule{
				ID:                   f.ID,
				Name:                 f.TidyName,
				DefaultConfiguration: &sarifConfig{Level: sarifLevel(f.Level)},
			}
			// Enrich with the catalog's one-line summary so GitHub Code
			// Scanning shows what each PX rule means, not just its id.
			// Pass-through diagnostics (clang-diagnostic-*) aren't in the
			// catalog and simply carry no shortDescription.
			if e, ok := catalog.ByID(f.ID); ok {
				if e.Title != "" {
					r.ShortDescription = &sarifText{Text: e.Title}
				}
				// Surface the safety caveat (some fix-its are unsafe to apply
				// blindly: PX3004/PX3007/PX3015/PX3027) in fullDescription, which
				// GitHub Code Scanning renders in the rule details — so a reviewer
				// triaging a finding sees the warning that -explain and -json show,
				// not just the one-line title. Only caveated rules get one.
				if e.Caveat != "" {
					r.FullDescription = &sarifText{Text: e.Title + " Caveat: " + e.Caveat}
				}
				// Link each rule to its upstream clang-tidy page so GitHub Code
				// Scanning offers a "learn more" per rule (custom checks have none).
				if url, ok := catalog.DocURL(e); ok {
					r.HelpURI = url
				}
				// Tag every catalogued rule "performance" plus its category, so
				// GitHub Code Scanning's rule-tag filter can group/narrow
				// perfscanxx findings (all perf) by category (alloc, containers, …).
				tags := []string{"performance"}
				if e.Category != "" {
					tags = append(tags, e.Category)
				}
				r.Properties = &sarifRuleProps{Tags: tags}
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
			Level:     sarifLevel(f.Level),
			Message:   sarifMessage{Text: f.Message},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: f.File},
					Region:           region,
				},
			}},
			PartialFingerprints: map[string]string{"perfscanxxIdentity/v1": lineIndependentFingerprint(f)},
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(sarifLog{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool:              sarifTool{Driver: sarifDriver{Name: "perfscanxx", InformationURI: "https://github.com/jxsl13/perfscan/perfscanxx", Version: ToolVersion, Rules: rules}},
			AutomationDetails: &sarifAutomationDetails{ID: "perfscanxx/"},
			Results:           results,
		}},
	})
}

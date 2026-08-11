// Package runner loads packages, executes perfscan checks, filters ignore
// directives, applies gated fixes and renders output.
package runner

import (
	"encoding/json"
	"fmt"
	"go/format"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// Options configures a run.
type Options struct {
	// Patterns are package patterns (./..., specific dirs/files).
	Patterns []string
	// Checks is a comma-separated selector: "all", explicit IDs
	// ("PS2005"), prefixes with a trailing * ("PS2*"), and exclusions
	// with a leading - ("-PS3003"). Default "all".
	Checks string
	// MaxLevel reports only checks whose fix level is <= MaxLevel.
	MaxLevel lint.Level
	// Tests includes _test.go files.
	Tests bool
	// Fix applies suggested fixes of auto-fixable checks whose level is
	// <= FixLevel.
	Fix bool
	// FixLevel gates which fixes -fix applies (default L1: only
	// idiomatic, bit-identical rewrites).
	FixLevel lint.Level
	// JSON emits findings as JSON instead of text.
	JSON bool
	// SARIF emits findings as SARIF 2.1.0 (for GitHub Code Scanning).
	SARIF bool
	// ConfigPath overrides config discovery.
	ConfigPath string
	// ExitZero forces exit code 0 even with findings.
	ExitZero bool
	// Baseline is a baseline file to filter against: findings covered by
	// it are suppressed, so only regressions remain (the "ratchet").
	Baseline string
	// WriteBaseline writes the current findings to Baseline instead of
	// reporting them, and exits 0.
	WriteBaseline bool

	Stdout io.Writer
	Stderr io.Writer
}

// Finding is one reported diagnostic.
type Finding struct {
	Check   *lint.Check
	Pos     token.Position
	End     token.Position
	Message string
	Fixes   []analysis.SuggestedFix
	fset    *token.FileSet
}

// Run executes perfscan and returns the process exit code.
func Run(checks []*lint.Check, opts Options) int {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if len(opts.Patterns) == 0 {
		opts.Patterns = []string{"./..."}
	}
	if opts.MaxLevel == 0 {
		opts.MaxLevel = lint.LevelAggressive
	}
	if opts.FixLevel == 0 {
		opts.FixLevel = lint.LevelIdiomatic
	}

	cfg, cfgPath := loadConfig(opts)
	config.Set(cfg.Compile())

	enabled, explicit, err := selectChecks(checks, opts.Checks, opts.MaxLevel)
	if err != nil {
		fmt.Fprintln(opts.Stderr, "perfscan:", err)
		return 2
	}
	// Domain checks are OPT-IN: a FULLY starved one (every vocabulary
	// field empty) is skipped silently under "all"/wildcard selection —
	// a partially fed check runs on whatever signals it has. Explicitly
	// naming a check with missing vocabulary keeps it and warns: the
	// user asked for a check that may not fire, which is worth one loud
	// line.
	kept := make([]*lint.Check, 0, len(enabled))
	for _, c := range enabled {
		missing := missingVocab(c, cfg)
		fullyStarved := c.NeedsConfig && len(missing) == len(c.Vocab)
		if fullyStarved && !explicit[c.ID] {
			continue
		}
		kept = append(kept, c)
		if len(missing) > 0 && explicit[c.ID] {
			src := "no perfscan.yaml found"
			if cfgPath != "" {
				src = "config " + cfgPath
			}
			fmt.Fprintf(opts.Stderr, "perfscan: WARNING: %s was selected explicitly but vocabulary %s is missing (%s)\n",
				c.ID, strings.Join(missing, ", "), src)
		}
	}
	enabled = kept

	pkgs, err := load(opts)
	if err != nil {
		fmt.Fprintln(opts.Stderr, "perfscan:", err)
		return 2
	}
	loadErrors := false
	for _, p := range pkgs {
		for _, e := range p.Errors {
			fmt.Fprintln(opts.Stderr, e)
			loadErrors = true
		}
	}
	if loadErrors {
		return 2
	}

	findings := make([]Finding, 0, len(pkgs))
	for _, pkg := range pkgs {
		for _, c := range enabled {
			findings = append(findings, runCheck(c, pkg)...)
		}
	}

	findings = filterIgnored(findings)
	slices.SortFunc(findings, func(a, b Finding) int {
		if c := strings.Compare(a.Pos.Filename, b.Pos.Filename); c != 0 {
			return c
		}
		if c := a.Pos.Line - b.Pos.Line; c != 0 {
			return c
		}
		if c := a.Pos.Column - b.Pos.Column; c != 0 {
			return c
		}
		return strings.Compare(a.Check.ID, b.Check.ID)
	})
	findings = dedup(findings)

	if opts.WriteBaseline {
		if opts.Baseline == "" {
			fmt.Fprintln(opts.Stderr, "perfscan: -write-baseline requires -baseline <file>")
			return 2
		}
		if err := writeBaseline(opts.Baseline, findings); err != nil {
			fmt.Fprintln(opts.Stderr, "perfscan: baseline:", err)
			return 2
		}
		fmt.Fprintf(opts.Stderr, "perfscan: wrote baseline with %d finding(s) to %s\n", len(findings), opts.Baseline)
		return 0
	}
	if opts.Baseline != "" {
		filtered, suppressed, err := applyBaseline(opts.Baseline, findings)
		if err != nil {
			fmt.Fprintln(opts.Stderr, "perfscan: baseline:", err)
			return 2
		}
		findings = filtered
		if suppressed > 0 {
			fmt.Fprintf(opts.Stderr, "perfscan: %d baselined finding(s) suppressed (%s)\n", suppressed, opts.Baseline)
		}
	}

	if opts.Fix {
		applied, failed := applyFixes(findings, opts)
		fmt.Fprintf(opts.Stderr, "perfscan: applied %d fix(es), %d failed\n", applied, failed)
	}

	switch {
	case opts.SARIF:
		emitSARIF(opts.Stdout, findings)
	case opts.JSON:
		emitJSON(opts.Stdout, findings)
	default:
		for _, f := range findings {
			fmt.Fprintf(opts.Stdout, "%s:%d:%d: %s (%s %s)\n",
				relPath(f.Pos.Filename), f.Pos.Line, f.Pos.Column, f.Message, f.Check.ID, f.Check.Level)
		}
	}

	if len(findings) > 0 && !opts.ExitZero {
		return 1
	}
	return 0
}

func relPath(p string) string {
	wd, err := os.Getwd()
	if err != nil {
		return p
	}
	if r, err := filepath.Rel(wd, p); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return p
}

func loadConfig(opts Options) (config.Config, string) {
	if opts.ConfigPath != "" {
		c, err := config.Load(opts.ConfigPath)
		if err != nil {
			fmt.Fprintln(opts.Stderr, "perfscan: config:", err)
			return config.Config{}, ""
		}
		return c, opts.ConfigPath
	}
	wd, _ := os.Getwd()
	return config.Discover(wd)
}

// missingVocab returns the vocabulary fields a domain check needs that the
// config does not supply. Empty for non-domain checks and fully-fed domain
// checks.
func missingVocab(c *lint.Check, cfg config.Config) []string {
	if !c.NeedsConfig {
		return nil
	}
	fields := map[string]int{
		"elementAccessors":       len(cfg.ElementAccessors),
		"fastPathHelpers":        len(cfg.FastPathHelpers),
		"elementCountMethods":    len(cfg.ElementCountMethods),
		"shapeMethods":           len(cfg.ShapeMethods),
		"indexDecomposeFuncs":    len(cfg.IndexDecomposeFuncs),
		"allocatorFuncs":         len(cfg.AllocatorFuncs),
		"perElementVisitors":     len(cfg.PerElementVisitors),
		"bulkCopyHelpers":        len(cfg.BulkCopyHelpers),
		"vectorizedSiblingFuncs": len(cfg.VectorizedSiblingFuncs),
		"fanOutHelpers":          len(cfg.FanOutHelpers),
		"dtypeMethods":           len(cfg.DtypeMethods),
	}
	missing := make([]string, 0, len(c.Vocab))
	for _, v := range c.Vocab {
		if n, ok := fields[v]; ok && n == 0 {
			missing = append(missing, v)
		}
	}
	return missing
}

// selectChecks resolves the -checks selector. The explicit set contains
// IDs the user named exactly (not via "all" or a wildcard) — those opt a
// vocabulary-starved domain check in, with a warning.
func selectChecks(all []*lint.Check, sel string, maxLevel lint.Level) ([]*lint.Check, map[string]bool, error) {
	if sel == "" {
		sel = "all"
	}
	include := map[string]bool{}
	exclude := map[string]bool{}
	explicit := map[string]bool{}
	for _, tok := range strings.Split(sel, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		neg := strings.HasPrefix(tok, "-")
		pat := strings.TrimPrefix(tok, "-")
		matched := false
		for _, c := range all {
			if matchCheck(c.ID, pat) {
				matched = true
				if neg {
					exclude[c.ID] = true
				} else {
					include[c.ID] = true
					if pat == c.ID {
						explicit[c.ID] = true
					}
				}
			}
		}
		if !matched && pat != "all" {
			return nil, nil, fmt.Errorf("no check matches %q (see perfscan -list)", tok)
		}
	}
	out := make([]*lint.Check, 0, len(all))
	for _, c := range all {
		if len(include) > 0 && !include[c.ID] {
			continue
		}
		if exclude[c.ID] || c.Level > maxLevel {
			continue
		}
		out = append(out, c)
	}
	return out, explicit, nil
}

func matchCheck(id, pat string) bool {
	if pat == "all" {
		return true
	}
	if prefix, ok := strings.CutSuffix(pat, "*"); ok {
		return strings.HasPrefix(id, prefix)
	}
	return id == pat
}

func load(opts Options) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedTypesSizes | packages.NeedImports | packages.NeedDeps,
		Tests: opts.Tests,
	}
	return packages.Load(cfg, opts.Patterns...)
}

func runCheck(c *lint.Check, pkg *packages.Package) []Finding {
	var out []Finding
	pass := &analysis.Pass{
		Analyzer:   c.Analyzer,
		Fset:       pkg.Fset,
		Files:      pkg.Syntax,
		OtherFiles: pkg.OtherFiles,
		Pkg:        pkg.Types,
		TypesInfo:  pkg.TypesInfo,
		TypesSizes: pkg.TypesSizes,
		ResultOf:   map[*analysis.Analyzer]any{},
		Report: func(d analysis.Diagnostic) {
			end := d.End
			if !end.IsValid() {
				end = d.Pos
			}
			out = append(out, Finding{
				Check:   c,
				Pos:     pkg.Fset.Position(d.Pos),
				End:     pkg.Fset.Position(end),
				Message: d.Message,
				Fixes:   d.SuggestedFixes,
				fset:    pkg.Fset,
			})
		},
	}
	if _, err := c.Analyzer.Run(pass); err != nil {
		fmt.Fprintf(os.Stderr, "perfscan: %s on %s: %v\n", c.ID, pkg.PkgPath, err)
	}
	return out
}

func dedup(in []Finding) []Finding {
	seen := make(map[string]bool, len(in))
	out := make([]Finding, 0, len(in))
	for _, f := range in {
		key := f.Pos.Filename + ":" + strconv.Itoa(f.Pos.Line) + ":" + strconv.Itoa(f.Pos.Column) + ":" + f.Check.ID
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	return out
}

// filterIgnored drops findings suppressed by //perfscan:ignore directives on
// the finding's line or the line directly above. The directive names one or
// more check IDs (comma/space separated); a bare directive suppresses every
// check on that line.
func filterIgnored(findings []Finding) []Finding {
	lines := map[string][]string{}
	fileLines := func(path string) []string {
		if l, ok := lines[path]; ok {
			return l
		}
		b, err := os.ReadFile(path)
		if err != nil {
			lines[path] = nil
			return nil
		}
		l := strings.Split(string(b), "\n")
		lines[path] = l
		return l
	}
	covered := func(f Finding, line int) bool {
		l := fileLines(f.Pos.Filename)
		if line < 1 || line > len(l) {
			return false
		}
		text := l[line-1]
		_, after, found := strings.Cut(text, "//perfscan:ignore")
		if !found {
			return false
		}
		rest := strings.TrimSpace(after)
		if rest == "" {
			return true
		}
		for _, tok := range strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
			if tok == f.Check.ID {
				return true
			}
			if !strings.HasPrefix(tok, "PS") {
				// reason text reached; IDs come first
				break
			}
		}
		return false
	}
	out := make([]Finding, 0, len(findings))
	for _, f := range findings {
		if covered(f, f.Pos.Line) || covered(f, f.Pos.Line-1) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// applyFixes applies the suggested fixes of auto-fixable checks whose level
// is within opts.FixLevel, then gofmt-formats touched files.
func applyFixes(findings []Finding, opts Options) (applied, failed int) {
	type edit struct {
		start, end int
		text       []byte
	}
	//perfscan:ignore PS2104 findings cluster in few files; len(findings) would over-reserve
	perFile := map[string][]edit{}
	for _, f := range findings {
		if !f.Check.AutoFix || f.Check.Level > opts.FixLevel || len(f.Fixes) == 0 {
			continue
		}
		fix := f.Fixes[0]
		ok := true
		edits := make([]edit, 0, len(fix.TextEdits))
		for _, te := range fix.TextEdits {
			file := f.fset.File(te.Pos)
			if file == nil {
				ok = false
				break
			}
			edits = append(edits, edit{
				start: file.Offset(te.Pos),
				end:   file.Offset(te.End),
				text:  te.NewText,
			})
		}
		if !ok {
			failed++
			continue
		}
		perFile[f.Pos.Filename] = append(perFile[f.Pos.Filename], edits...)
		applied++
	}

	for path, edits := range perFile {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "perfscan: fix %s: %v\n", path, err)
			failed++
			continue
		}
		slices.SortFunc(edits, func(a, b edit) int { return b.start - a.start })
		overlap := false
		for i := 1; i < len(edits); i++ {
			if edits[i].end > edits[i-1].start {
				overlap = true
				break
			}
		}
		if overlap {
			fmt.Fprintf(opts.Stderr, "perfscan: fix %s: overlapping edits, skipping file\n", path)
			failed++
			continue
		}
		for _, e := range edits {
			src = append(src[:e.start], append(append([]byte{}, e.text...), src[e.end:]...)...)
		}
		if formatted, err := format.Source(src); err == nil {
			src = formatted
		}
		if err := os.WriteFile(path, src, 0o644); err != nil {
			fmt.Fprintf(opts.Stderr, "perfscan: fix %s: %v\n", path, err)
			failed++
		}
	}
	return applied, failed
}

type jsonEdit struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	EndLine int    `json:"endLine"`
	EndCol  int    `json:"endCol"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	NewText string `json:"newText"`
}

type jsonFix struct {
	Message string     `json:"message"`
	Edits   []jsonEdit `json:"edits"`
}

type jsonFinding struct {
	ID       string    `json:"id"`
	Category string    `json:"category"`
	Level    string    `json:"level"`
	AutoFix  bool      `json:"autoFix"`
	File     string    `json:"file"`
	Line     int       `json:"line"`
	Col      int       `json:"col"`
	EndLine  int       `json:"endLine"`
	EndCol   int       `json:"endCol"`
	Message  string    `json:"message"`
	Fixes    []jsonFix `json:"fixes,omitempty"`
}

func emitJSON(w io.Writer, findings []Finding) {
	out := make([]jsonFinding, 0, len(findings))
	for _, f := range findings {
		jf := jsonFinding{
			ID:       f.Check.ID,
			Category: f.Check.Category,
			Level:    f.Check.Level.String(),
			AutoFix:  f.Check.AutoFix,
			File:     relPath(f.Pos.Filename),
			Line:     f.Pos.Line,
			Col:      f.Pos.Column,
			EndLine:  f.End.Line,
			EndCol:   f.End.Column,
			Message:  f.Message,
		}
		for _, fix := range f.Fixes {
			jx := jsonFix{Message: fix.Message}
			for _, te := range fix.TextEdits {
				file := f.fset.File(te.Pos)
				if file == nil {
					continue
				}
				p := f.fset.Position(te.Pos)
				e := f.fset.Position(te.End)
				jx.Edits = append(jx.Edits, jsonEdit{
					File:    relPath(p.Filename),
					Line:    p.Line,
					Col:     p.Column,
					EndLine: e.Line,
					EndCol:  e.Column,
					Start:   file.Offset(te.Pos),
					End:     file.Offset(te.End),
					NewText: string(te.NewText),
				})
			}
			jf.Fixes = append(jf.Fixes, jx)
		}
		out = append(out, jf)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

// Package runner loads packages, executes perfscan checks, filters ignore
// directives, applies gated fixes and renders output.
package runner

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	"github.com/jxsl13/perfscan/perfscan/config"
	"github.com/jxsl13/perfscan/perfscan/lint"
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
	// Fix applies the suggested fixes of every reported auto-fixable
	// check — MaxLevel gates both reporting and fixing.
	Fix bool
	// Diff prints a unified diff of what Fix would change, without
	// modifying any file, and exits 1 when at least one file would
	// change. Mutually exclusive with Fix.
	Diff bool
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
	// IncludeGenerated reports (and fixes) findings in generated Go files
	// (those with the conventional "// Code generated ... DO NOT EDIT."
	// marker). By default they are skipped.
	IncludeGenerated bool
	// Exclude drops findings whose slash-normalized file path contains any
	// of these substrings (e.g. "vendor/", "testdata/", ".pb.go"). Because
	// -fix works from the findings list, excluding a finding also excludes
	// its fix.
	Exclude []string

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
	if opts.Diff && opts.Fix {
		fmt.Fprintln(opts.Stderr, "perfscan: -diff and -fix are mutually exclusive")
		return 2
	}
	cacheWd()

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
	before := len(findings)
	findings = filterGenerated(findings, opts.IncludeGenerated)
	if dropped := before - len(findings); !opts.IncludeGenerated && dropped > 0 {
		fmt.Fprintf(opts.Stderr, "perfscan: %d finding(s) in generated files skipped (use -include-generated to report them)\n", dropped)
	}
	if len(opts.Exclude) > 0 {
		before := len(findings)
		findings = filterExcluded(findings, opts.Exclude)
		if dropped := before - len(findings); dropped > 0 {
			fmt.Fprintf(opts.Stderr, "perfscan: %d finding(s) excluded by -exclude\n", dropped)
		}
	}
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

	if opts.Diff {
		// Dry-run: the diff IS the output — findings text is suppressed
		// (stdout must stay a valid patch); the summary goes to stderr.
		return diffFixes(findings, opts)
	}

	if opts.Fix {
		applied, overlapping, failed := applyFixes(findings, opts)
		msg := fmt.Sprintf("perfscan: applied %d fix(es)", applied)
		if overlapping > 0 {
			// Benign: these overlapped a fix already applied to the same span
			// (two checks that rewrite one node), so the code is fixed either
			// way — reported distinctly from a genuine failure.
			msg += fmt.Sprintf(", %d skipped (overlaps an applied fix)", overlapping)
		}
		if failed > 0 {
			msg += fmt.Sprintf(", %d failed", failed)
		}
		fmt.Fprintln(opts.Stderr, msg)
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

// cachedWd holds the working directory for relPath, which runs once per
// finding when formatting output — caching avoids a Getwd syscall per
// finding. Run refreshes it up front (the process may chdir between runs,
// e.g. in tests, but never during one); relPath falls back to fetching it
// lazily when called outside Run.
var (
	cachedWd    string
	cachedWdErr error
	wdCached    bool
)

// cacheWd captures the current working directory for relPath.
func cacheWd() {
	cachedWd, cachedWdErr = os.Getwd()
	wdCached = true
}

func relPath(p string) string {
	if !wdCached {
		cacheWd()
	}
	if cachedWdErr != nil {
		return p
	}
	if r, err := filepath.Rel(cachedWd, p); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return p
}

func loadConfig(opts Options) (config.Config, string) {
	var c config.Config
	var path string
	if opts.ConfigPath != "" {
		var err error
		c, err = config.Load(opts.ConfigPath)
		if err != nil {
			fmt.Fprintln(opts.Stderr, "perfscan: config:", err)
			return config.Config{}, ""
		}
		path = opts.ConfigPath
	} else {
		wd, _ := os.Getwd()
		c, path = config.Discover(wd)
	}
	// Warn on unrecognized top-level keys: yaml.Unmarshal silently drops them,
	// so a typo'd vocabulary key would otherwise leave its domain check starved
	// and silent — the one failure mode that costs whole investigations.
	for _, k := range config.UnknownKeys(path) {
		fmt.Fprintf(opts.Stderr, "perfscan: WARNING: config %s: unrecognized key %q (typo? the domain check keying on it stays silent)\n", path, k)
	}
	return c, path
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
	for tok := range strings.SplitSeq(sel, ",") {
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

// filterGenerated drops findings located in generated Go files (those with
// the conventional `// Code generated ... DO NOT EDIT.` marker before the
// package clause), unless include is true. Detection uses go/ast.IsGenerated,
// the authoritative check. A file that cannot be parsed is treated as NOT
// generated (reported), never silently dropped.
func filterGenerated(findings []Finding, include bool) []Finding {
	if include {
		return findings
	}
	generated := map[string]bool{}
	isGenerated := func(path string) bool {
		if g, ok := generated[path]; ok {
			return g
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.PackageClauseOnly|parser.ParseComments)
		g := err == nil && ast.IsGenerated(file)
		generated[path] = g
		return g
	}
	out := make([]Finding, 0, len(findings))
	// Iterate by index: a Finding is large enough that ranging by value would
	// copy it every iteration (perfscan's own PS3103 — dogfooded).
	for i := range findings {
		if isGenerated(findings[i].Pos.Filename) {
			continue
		}
		out = append(out, findings[i])
	}
	return out
}

// pathExcluded reports whether p's slash-normalized path contains any of the
// exclude substrings. An empty exclude list never matches.
func pathExcluded(p string, excludes []string) bool {
	if len(excludes) == 0 {
		return false
	}
	slashed := filepath.ToSlash(p)
	for _, e := range excludes {
		if strings.Contains(slashed, e) {
			return true
		}
	}
	return false
}

// filterExcluded drops findings whose file path matches any -exclude
// substring (see pathExcluded). Because -fix works from the findings list,
// this filter covers report, JSON, SARIF, -diff, baseline, and -fix alike.
func filterExcluded(findings []Finding, excludes []string) []Finding {
	if len(excludes) == 0 {
		return findings
	}
	out := make([]Finding, 0, len(findings))
	// Iterate by index: a Finding is large enough that ranging by value would
	// copy it every iteration (perfscan's own PS3103 — dogfooded).
	for i := range findings {
		if pathExcluded(findings[i].Pos.Filename, excludes) {
			continue
		}
		out = append(out, findings[i])
	}
	return out
}

// patchedFile pairs a file's on-disk bytes with the bytes -fix would leave
// behind for it.
type patchedFile struct {
	orig, fixed []byte
}

// patchedFiles computes, per file, the bytes -fix would write: it groups
// the suggested fixes of the reported auto-fixable checks (the enabled set
// is already MaxLevel-gated), merges the sorted TextEdits, and
// gofmt-formats the result — everything applyFixes does except the final
// write. applyFixes writes the results; the -diff path renders them.
func patchedFiles(findings []Finding, opts Options) (files map[string]patchedFile, applied, overlapping, failed int) {
	type edit struct {
		start, end int
		text       []byte
	}
	// A fileFix is one finding's edits within a single file, resolved
	// atomically — all its edits apply or none do.
	type fileFix struct {
		edits []edit
	}
	//perfscan:ignore PS2104 findings cluster in few files; len(findings) would over-reserve
	perFile := map[string][]fileFix{}
	for _, f := range findings {
		if !f.Check.AutoFix || len(f.Fixes) == 0 {
			continue
		}
		fix := f.Fixes[0]
		ok := true
		// Group each edit under the REAL token.File it belongs to, keyed by
		// that file's name with RAW offsets. Keying by the finding's
		// Pos.Filename would honor //line directives (and cgo preambles),
		// which remap the reported path to an unrelated file — sending these
		// raw offsets into the wrong file and silently corrupting it.
		//perfscan:ignore PS2104 a fix's edits nearly always share one file; len(TextEdits) would over-reserve
		byFile := map[string][]edit{}
		for _, te := range fix.TextEdits {
			file := f.fset.File(te.Pos)
			if file == nil {
				ok = false
				break
			}
			byFile[file.Name()] = append(byFile[file.Name()], edit{
				start: file.Offset(te.Pos),
				end:   file.Offset(te.End),
				text:  te.NewText,
			})
		}
		if !ok {
			failed++
			continue
		}
		// One fileFix per (finding, file): the finding's edits in that file are
		// applied atomically — all or none — so overlap resolution can drop a
		// whole conflicting fix without splicing half of it.
		for name, es := range byFile {
			perFile[name] = append(perFile[name], fileFix{edits: es})
		}
	}

	// fixMinStart is a fix's earliest edit offset (its sort key); conflicts
	// reports whether any edit in es overlaps any already-accepted edit
	// (half-open ranges, so a boundary-touching insertion does not conflict).
	fixMinStart := func(f fileFix) int {
		m := f.edits[0].start
		for _, e := range f.edits[1:] {
			if e.start < m {
				m = e.start
			}
		}
		return m
	}
	conflicts := func(es, accepted []edit) bool {
		for _, e := range es {
			for _, a := range accepted {
				if e.start < a.end && a.start < e.end {
					return true
				}
			}
		}
		return false
	}
	// selfOverlaps reports whether a single fix's own edits overlap each other.
	// A well-formed SuggestedFix never does (the go/analysis contract), but the
	// greedy loop applies an accepted fix's edits verbatim, so a malformed one
	// would corrupt the file instead of being skipped as the old flat overlap
	// check did — reject it whole rather than splice overlapping edits.
	selfOverlaps := func(es []edit) bool {
		for i := range es {
			for j := i + 1; j < len(es); j++ {
				if es[i].start < es[j].end && es[j].start < es[i].end {
					return true
				}
			}
		}
		return false
	}

	files = make(map[string]patchedFile, len(perFile))
	for path, fixes := range perFile {
		// Never write outside the working tree. On a cgo package the analyzer
		// sees the cgo-PROCESSED translation unit, whose token.File.Name() is the
		// go-build cache path; its offsets are in range for THAT file, so the
		// out-of-range guard below misses. Writing there poisons a build artifact
		// and, worse, reports a false "applied" while the user's real source is
		// untouched. GOMODCACHE/GOROOT are likewise off-limits (read-only deps).
		if outsideWorkTree(path) {
			fmt.Fprintf(opts.Stderr, "perfscan: fix targets a generated/cached source outside the module (%s); cgo/generated packages cannot be auto-fixed — apply manually\n", path)
			failed += len(fixes)
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "perfscan: fix %s: %v\n", path, err)
			failed += len(fixes)
			continue
		}
		// A "// Code generated ... DO NOT EDIT." file must never be rewritten,
		// wherever it lives (cgo TUs carry this marker; so do stringer/protoc
		// outputs a user may have in-tree). Same reasoning as the path guard.
		if isGeneratedSource(src) {
			fmt.Fprintf(opts.Stderr, "perfscan: fix %s: generated file (DO NOT EDIT); not applied\n", path)
			failed += len(fixes)
			continue
		}
		orig := slices.Clone(src)

		// Reject the whole file when any edit's offsets do not address the
		// on-disk bytes — e.g. a cgo-processed TU whose parsed bytes differ
		// from the source. Applying such offsets would panic or corrupt.
		outOfRange := false
		for i := range fixes {
			for _, e := range fixes[i].edits {
				if e.start < 0 || e.start > e.end || e.end > len(src) {
					outOfRange = true
				}
			}
		}
		if outOfRange {
			fmt.Fprintf(opts.Stderr, "perfscan: fix %s: edit offsets out of range (generated or cgo source?), skipping file\n", path)
			failed += len(fixes)
			continue
		}

		// Resolve overlaps at the FIX level: two checks can target the same
		// span (e.g. PS2103 and PS2122 both rewrite one fmt.Sprintf into a + b,
		// via different but equivalent edits). Rather than skip the whole file,
		// apply a maximal set of non-overlapping fixes. A fix that collides with
		// an already-accepted one is counted OVERLAPPING (benign — the code is
		// still correctly rewritten by the fix that won), distinct from a
		// genuine failure; a self-overlapping (malformed) fix is a real failure
		// since applying it would corrupt the file. Deterministic: fixes are
		// ordered by their first edit, then by input order (stable).
		slices.SortStableFunc(fixes, func(a, b fileFix) int { return fixMinStart(a) - fixMinStart(b) })
		var accepted []edit
		for i := range fixes {
			if selfOverlaps(fixes[i].edits) {
				failed++
				continue
			}
			if conflicts(fixes[i].edits, accepted) {
				overlapping++
				continue
			}
			accepted = append(accepted, fixes[i].edits...)
			applied++
		}
		// Every fix for this file was rejected (all conflicting or malformed):
		// leave it untouched rather than rewriting it with only a gofmt pass.
		if len(accepted) == 0 {
			continue
		}
		// Apply accepted edits back-to-front so earlier offsets stay valid.
		slices.SortFunc(accepted, func(a, b edit) int { return b.start - a.start })
		for _, e := range accepted {
			src = append(src[:e.start], append(append([]byte{}, e.text...), src[e.end:]...)...)
		}
		if formatted, err := format.Source(src); err == nil {
			src = formatted
		}
		files[path] = patchedFile{orig: orig, fixed: src}
	}
	return files, applied, overlapping, failed
}

// applyFixes applies the suggested fixes of the reported auto-fixable
// checks (the enabled set is already MaxLevel-gated), then gofmt-formats
// touched files.
func applyFixes(findings []Finding, opts Options) (applied, overlapping, failed int) {
	files, applied, overlapping, failed := patchedFiles(findings, opts)
	for path, pf := range files {
		if err := os.WriteFile(path, pf.fixed, 0o644); err != nil {
			fmt.Fprintf(opts.Stderr, "perfscan: fix %s: %v\n", path, err)
			failed++
		}
	}
	return applied, overlapping, failed
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

// Package runner loads packages, executes perfscan checks, filters ignore
// directives, applies gated fixes and renders output.
package runner

import (
	"bytes"
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
	"golang.org/x/tools/go/ast/astutil"
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
		// Drop any import the applied fixes just orphaned. Independent checks
		// can EACH remove the last-but-one reference to a shared package —
		// PS3002 rewriting sort.Slice and PS3104 rewriting sort.Strings in one
		// file both drop a "sort" reference, but neither alone sees the import
		// as orphaned, so without this the file is left with an "imported and
		// not used" error. Best-effort and centralized here so it covers every
		// check pair, not just the sort family.
		src = dedupeImports(src)
		src = addReferencedStdlibImports(src, orig)
		src = pruneOrphanedImports(src)
		if formatted, err := format.Source(src); err == nil {
			src = formatted
		}
		files[path] = patchedFile{orig: orig, fixed: src}
	}
	return files, applied, overlapping, failed
}

// pruneOrphanedImports removes import specs that the just-applied fixes left
// unused, so cross-check rewrites that each drop the last reference to a
// shared package cannot leave an "imported and not used" compile error. It is
// best-effort: a parse problem, a cgo file (import "C"), or a //line-directive
// file (generated/preprocessed source whose import block must not be
// rewritten) returns the input unchanged. Blank (_) and dot (.) imports are
// never pruned — a blank import is deliberately unreferenced. Only
// standard-library imports are considered (see the loop): for those the
// package name equals the last path segment so usage detection is reliable,
// whereas a versioned third-party path (k8s.io/klog/v2, package "klog") would
// be misjudged. The source compiled BEFORE the fixes, so any stdlib import
// unused AFTER them was orphaned by them, never pre-existing.
// dedupeImports collapses exact-duplicate import specs the applied fixes may
// have introduced. Two DIFFERENT checks can each add the SAME import — PS3104
// rewriting sort.Ints and PS3105 rewriting sort.Sort(sort.IntSlice) both add
// "slices" — yielding two `import "slices"` declarations, which is a "redeclared
// in this block" compile error. It removes the extra copies, keeping one,
// matching on (alias, path) so two intentionally-different aliases of one path
// are preserved. Blank (_) imports are left alone (duplicate blank imports are
// legal and deliberate). Same best-effort cgo / //line / parse guards as
// pruneOrphanedImports.
func dedupeImports(src []byte) []byte {
	if bytes.Contains(src, []byte("//line ")) || bytes.Contains(src, []byte("/*line ")) {
		return src
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return src
	}
	for _, imp := range f.Imports {
		if imp.Path.Value == `"C"` {
			return src
		}
	}
	type spec struct{ name, path string }
	count := map[spec]int{}
	var dups []spec
	for _, imp := range f.Imports {
		name := ""
		if imp.Name != nil {
			name = imp.Name.Name
			if name == "_" || name == "." {
				continue // blank/dot duplicates are legal / not our concern
			}
		}
		path, uerr := strconv.Unquote(imp.Path.Value)
		if uerr != nil {
			return src
		}
		s := spec{name, path}
		count[s]++
		if count[s] == 2 {
			dups = append(dups, s)
		}
	}
	if len(dups) == 0 {
		return src
	}
	for _, s := range dups {
		// Remove every copy, then add exactly one back — robust whether
		// DeleteNamedImport removes a single match or all of them per call.
		for astutil.DeleteNamedImport(fset, f, s.name, s.path) {
		}
		astutil.AddNamedImport(fset, f, s.name, s.path)
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return src
	}
	return buf.Bytes()
}

// fixableStdlibImports maps package NAME -> import PATH for exactly the
// standard-library packages perfscan's import-adding fixes introduce. It is
// the allowlist gate of addReferencedStdlibImports: only these, and never a
// third-party path, may be added to a fixed file.
var fixableStdlibImports = map[string]string{
	"slices":  "slices",
	"cmp":     "cmp",
	"io":      "io",
	"strconv": "strconv",
	"bytes":   "bytes",
	"fmt":     "fmt",
	"sort":    "sort",
	"strings": "strings",
	"errors":  "errors",
	"utf8":    "unicode/utf8",
	"hex":     "encoding/hex", // PS2107 rewrites fmt.Sprintf("%x", b) -> hex.EncodeToString(b)
}

// addReferencedStdlibImports adds any stdlib import the just-applied fixes
// reference but no surviving fix declares. Import-adding checks (PS3104,
// PS2110, PS3002, PS2129, ...) attach the `import "slices"`-style edit to
// only the FIRST fixable finding in a file (to avoid overlapping
// import-block edits), but the runner filters findings (//perfscan:ignore,
// -exclude, baseline) AFTER the checks run and BEFORE fixes apply: when the
// finding carrying the import-add is filtered out while a sibling finding of
// the same check survives and is rewritten, the rewrite references a package
// that was never imported — "undefined: slices", a broken build. This pass
// restores the invariant: every allowlisted package name the FIXED file uses
// as a qualifier but neither imports nor declares gets its stdlib import
// added. It runs after dedupeImports and before pruneOrphanedImports (an
// import added here is by construction used, so the prune never removes it).
//
// Safety: allowlist-gated (fixableStdlibImports, stdlib only); a qualifier
// whose name is declared ANYWHERE in the file (func/type/var/const, params,
// results, receivers, :=, range vars, labels, fields) is skipped so a user
// local shadowing a package name is never clobbered; a qualifier the ORIGINAL
// (pre-fix, compiling) file already used without importing is skipped too —
// there it must resolve to a cross-file package-level identifier or dot
// import, and adding the import would redeclare it. Same best-effort cgo /
// //line / parse guards as dedupeImports and pruneOrphanedImports.
func addReferencedStdlibImports(src, orig []byte) []byte {
	if bytes.Contains(src, []byte("//line ")) || bytes.Contains(src, []byte("/*line ")) {
		return src
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return src
	}
	missing := missingFixableImports(f)
	if len(missing) == 0 {
		return src
	}
	for _, imp := range f.Imports {
		if imp.Path.Value == `"C"` {
			return src // cgo: leave the preprocessed import block untouched
		}
	}
	// Pre-fix guard: if the ORIGINAL file (which compiled) already used the
	// qualifier without importing it, the name resolves to something else
	// (cross-file package-level identifier, dot import) — do not add.
	if of, err := parser.ParseFile(token.NewFileSet(), "", orig, parser.ParseComments); err == nil {
		preexisting := missingFixableImports(of)
		missing = slices.DeleteFunc(missing, func(path string) bool {
			return slices.Contains(preexisting, path)
		})
	}
	if len(missing) == 0 {
		return src
	}
	for _, path := range missing {
		astutil.AddImport(fset, f, path)
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return src
	}
	return buf.Bytes()
}

// missingFixableImports returns the import paths (in first-use order) of
// allowlisted stdlib packages that f uses as a selector qualifier but neither
// imports under that name nor declares as any identifier in the file.
func missingFixableImports(f *ast.File) []string {
	// Local names the import block already provides.
	imported := make(map[string]bool, len(f.Imports))
	for _, imp := range f.Imports {
		path, uerr := strconv.Unquote(imp.Path.Value)
		if uerr != nil {
			continue
		}
		name := path
		if i := strings.LastIndexByte(path, '/'); i >= 0 {
			name = path[i+1:]
		}
		if imp.Name != nil {
			if imp.Name.Name == "_" || imp.Name.Name == "." {
				continue
			}
			name = imp.Name.Name
		}
		imported[name] = true
	}
	// Every identifier NAME declared anywhere in the file: if a user local
	// (or type, func, field, label, ...) shares a package's name, adding the
	// import could clobber it — be conservative and never add.
	declared := map[string]bool{}
	declare := func(id *ast.Ident) {
		if id != nil && id.Name != "_" {
			declared[id.Name] = true
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			declare(n.Name)
		case *ast.Field: // receivers, params, results, struct fields, type params
			for _, id := range n.Names {
				declare(id)
			}
		case *ast.ValueSpec: // var / const names
			for _, id := range n.Names {
				declare(id)
			}
		case *ast.TypeSpec:
			declare(n.Name)
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				for _, lhs := range n.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						declare(id)
					}
				}
			}
		case *ast.RangeStmt:
			if id, ok := n.Key.(*ast.Ident); ok {
				declare(id)
			}
			if id, ok := n.Value.(*ast.Ident); ok {
				declare(id)
			}
		case *ast.LabeledStmt:
			declare(n.Label)
		}
		return true
	})
	var missing []string
	seen := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		path, ok := fixableStdlibImports[id.Name]
		if !ok || imported[id.Name] || declared[id.Name] || seen[id.Name] {
			return true
		}
		seen[id.Name] = true
		missing = append(missing, path)
		return true
	})
	return missing
}

// isVersionSegment reports whether s is a Go module major-version path segment
// (v2, v3, …). Such a segment is never the package's actual name, so the
// path-based import-usage heuristic cannot be trusted for these imports.
func isVersionSegment(s string) bool {
	if len(s) < 2 || s[0] != 'v' {
		return false
	}
	for _, r := range s[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func pruneOrphanedImports(src []byte) []byte {
	if bytes.Contains(src, []byte("//line ")) || bytes.Contains(src, []byte("/*line ")) {
		return src
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return src
	}
	for _, imp := range f.Imports {
		if imp.Path.Value == `"C"` {
			return src // cgo: leave the preprocessed import block untouched
		}
	}
	// Collect the orphans first: DeleteNamedImport mutates f.Imports, so
	// deleting inside `range f.Imports` would nil-deref. UsesImport is computed
	// on the original file before any deletion (deleting one import never
	// changes whether another is used).
	type orphan struct{ name, path string }
	var orphans []orphan
	for _, imp := range f.Imports {
		var name string
		if imp.Name != nil {
			name = imp.Name.Name
			if name == "_" || name == "." {
				continue // side-effect or dot import: never unused-prune
			}
		}
		path, uerr := strconv.Unquote(imp.Path.Value)
		if uerr != nil {
			continue
		}
		// A trailing version segment (…/v2) means the package name is NOT the
		// last path segment: math/rand/v2 is package "rand", k8s.io/klog/v2 is
		// "klog". astutil.UsesImport infers the name from the last segment ("v2")
		// and would wrongly report the import unused, pruning a LIVE import — the
		// "undefined: rand" break observed on gonum's stat/distmv after a
		// sort.Ints -> slices.Sort rewrite orphaned "sort" next to "math/rand/v2".
		// Leaving a (rarely) genuinely-orphaned versioned import is far safer than
		// deleting a live one; this runs BEFORE the stdlib-only gate below so it
		// also covers stdlib /vN paths, which that gate (dot-in-path) misses.
		if isVersionSegment(path[strings.LastIndexByte(path, '/')+1:]) {
			continue
		}
		// Only STANDARD-LIBRARY imports (no dot in the path). For those the
		// package name always equals the last path segment, so astutil.UsesImport
		// — which infers the name from the path — is reliable. A third-party path
		// like k8s.io/klog/v2 has package name "klog" != last segment "v2", and
		// UsesImport would wrongly report it unused and prune a live import
		// (observed on kubernetes). The cross-check orphan bug this guards against
		// is a stdlib one (sort), so restricting to stdlib loses nothing.
		if strings.Contains(path, ".") {
			continue
		}
		if astutil.UsesImport(f, path) {
			continue
		}
		orphans = append(orphans, orphan{name, path})
	}
	if len(orphans) == 0 {
		return src
	}
	for _, o := range orphans {
		astutil.DeleteNamedImport(fset, f, o.name, o.path)
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return src
	}
	return buf.Bytes()
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

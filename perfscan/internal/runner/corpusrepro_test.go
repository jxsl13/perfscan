package runner

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestFixRealWorldSortAndWriteImportComposition pins a composition observed in
// the wild during corpus -fix validation (rs/zerolog console.go): a single file
// where PS3104 (sort.Strings -> slices.Sort) and PS2129 (fmt.Fprint(w,s) ->
// io.WriteString(w,s)) both fire. Each check both ADDS a stdlib import (slices,
// io) AND orphans one (sort, fmt) — so the runner must, in one pass, add slices,
// add io, prune the now-unused sort and fmt, and leave a still-used import
// (bytes) intact, producing an import block that compiles. This is the two-add +
// two-prune case, distinct from the orphan-only (TestFixPrunesCrossCheckOrphan-
// Import) and duplicate-add (TestFixDedupesCrossCheckImportAdd) pins.
//
// On the real file this rewrote cleanly and zerolog's tests passed; this locks
// that outcome as a standing regression against the shared import machinery.
func TestFixRealWorldSortAndWriteImportComposition(t *testing.T) {
	const src = `package p

import (
	"bytes"
	"fmt"
	"sort"
)

func render(buf *bytes.Buffer, fields []string, s string) {
	sort.Strings(fields)
	fmt.Fprint(buf, s)
}
`
	got := string(runFixMode(t, src))

	// Both idioms rewritten.
	for _, want := range []string{"slices.Sort(fields)", "io.WriteString(buf, s)"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in the fixed file:\n%s", want, got)
		}
	}
	// The two added imports are present, the still-live one kept, the two
	// orphaned ones pruned.
	for _, want := range []string{`"bytes"`, `"io"`, `"slices"`} {
		if !strings.Contains(got, want) {
			t.Errorf("expected import %q to be present:\n%s", want, got)
		}
	}
	for _, gone := range []string{`"fmt"`, `"sort"`} {
		if strings.Contains(got, gone) {
			t.Errorf("import %s should have been pruned as an orphan:\n%s", gone, got)
		}
	}
	// A corrupted/co-located import edit would still leave text that fails to
	// parse — assert the result is valid Go.
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

// TestFixGinFprintNoImportChurn pins the ZERO-import-churn variant of PS2129
// observed during corpus -fix validation on gin-gonic/gin (logger.go): a single
// `fmt.Fprint(out, s)` -> `io.WriteString(out, s)` in a file that ALREADY
// imports "io" and STILL uses "fmt" elsewhere. The runner must rewrite the call
// but touch the import block NOT AT ALL — no duplicate "io" add, no premature
// "fmt" prune. This is the counterpart to the add+prune pins above (zerolog
// TestFixRealWorldSortAndWriteImportComposition, cobra partial-retention): here
// the correct edit is import-neutral, and a naive "PS2129 always adds io / drops
// fmt" would either duplicate io or orphan a still-live fmt and break the build.
//
// On the real gin logger.go this rewrote cleanly; gin built and its whole test
// suite passed except one pre-existing environmental port-bind test (TestRun-
// Empty, which fails identically without the fix). Lock the outcome here.
func TestFixGinFprintNoImportChurn(t *testing.T) {
	const src = `package p

import (
	"fmt"
	"io"
)

func log(out io.Writer, s string) {
	fmt.Fprintf(out, "[GIN] %s\n", s) // keeps fmt live
	fmt.Fprint(out, s)                // <- rewritten to io.WriteString
}
`
	got := string(runFixMode(t, src))

	if !strings.Contains(got, "io.WriteString(out, s)") {
		t.Errorf("expected fmt.Fprint(out, s) -> io.WriteString(out, s):\n%s", got)
	}
	if strings.Contains(got, "fmt.Fprint(out, s)") {
		t.Errorf("original fmt.Fprint(out, s) should be gone:\n%s", got)
	}
	// fmt is still used by the surviving Fprintf and io was already imported:
	// BOTH imports must remain, each exactly once (no duplicate io add).
	for _, want := range []string{`"fmt"`, `"io"`} {
		if n := strings.Count(got, want); n != 1 {
			t.Errorf("import %s should appear exactly once (no churn), got %d:\n%s", want, n, got)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

// TestFixPromClientSortAndWriteRuneComposition pins a composition observed
// during corpus -fix validation on prometheus/client_golang (internal/metric.go):
// one file where PS3104 (sort.Strings -> slices.Sort) fires alongside PS5102
// (bytes.Buffer.WriteRune('x') -> WriteByte('x') for single-byte ASCII runes).
// PS3104 must ADD "slices" and PRUNE the now-orphaned "sort", while PS5102's two
// import-NEUTRAL rune->byte rewrites happen in the same pass without disturbing
// that import edit. This locks the interaction of an add+prune import check with
// an import-neutral one — PS5102's first runner-level composition pin.
//
// Bit-identical: WriteRune(r) for r < utf8.RuneSelf writes exactly the one byte
// WriteByte(byte(r)) writes; slices.Sort is bit-identical to sort.Strings for
// strings. On the real client_golang this rewrote cleanly across 8 files (17
// fixes); it built and its entire test suite passed. Lock the metric.go shape.
func TestFixPromClientSortAndWriteRuneComposition(t *testing.T) {
	const src = `package p

import (
	"bytes"
	"sort"
)

func render(buf *bytes.Buffer, names []string) {
	sort.Strings(names)
	for _, n := range names {
		buf.WriteString(n)
		buf.WriteRune(';')
		buf.WriteRune('=')
	}
}
`
	got := string(runFixMode(t, src))

	// Both idioms rewritten.
	for _, want := range []string{"slices.Sort(names)", "buf.WriteByte(';')", "buf.WriteByte('=')"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in the fixed file:\n%s", want, got)
		}
	}
	// PS3104 added slices and orphaned sort; bytes stays (still used).
	for _, want := range []string{`"bytes"`, `"slices"`} {
		if !strings.Contains(got, want) {
			t.Errorf("expected import %q to be present:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"sort"`) {
		t.Errorf("import \"sort\" should have been pruned as an orphan:\n%s", got)
	}
	// No stray WriteRune left, and the result compiles.
	if strings.Contains(got, "WriteRune") {
		t.Errorf("all single-byte WriteRune calls should be rewritten:\n%s", got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

// TestFixConsulGroupingMapPrealloc pins the PS2104 grouping-map shape observed
// during corpus -fix validation on hashicorp/consul (stream/event_publisher.go:
// `groupedEvents := make(map[topicSubject][]Event, len(events))`). The map is
// filled by ranging `events` but keyed by a DERIVED value (the subject), so it
// holds AT MOST len(events) entries and usually FEWER. PS2104 still hints it to
// len(events): a map capacity hint is a pure pre-allocation reservation that
// NEVER changes behavior or iteration — an upper-bound hint is bit-identical by
// construction, so grouping maps (entries <= len(src)) are a safe, intended
// target, not a miss. Locks that PS2104 fires here and sizes to len(src).
//
// On the real consul this was 1 of 72 bit-identical fixes across 34 files; the
// tree built, vetted, and the state + stream package tests passed.
func TestFixConsulGroupingMapPrealloc(t *testing.T) {
	const src = `package p

type ev struct{ subject string }

func group(events []ev) map[string][]ev {
	grouped := map[string][]ev{}
	for _, e := range events {
		grouped[e.subject] = append(grouped[e.subject], e)
	}
	return grouped
}
`
	got := string(runFixMode(t, src))

	if !strings.Contains(got, "make(map[string][]ev, len(events))") {
		t.Errorf("expected grouping map hinted to len(events):\n%s", got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

// TestFixStdlibSprintfBuilderCompose pins a two-check composition surfaced by a
// robustness sweep of the Go standard library (a `str := ""` accumulator built
// with `str += fmt.Sprintf("%s %s; ", a, b)` in a loop, as in encoding/gob):
// PS2128 converts the string accumulator to a strings.Builder AND PS2103
// rewrites the inner Sprintf %s-splice to concatenation — ON THE SAME statement
// — yielding `str.WriteString(a + " " + b + "; ")`, with "strings" added and
// "fmt" pruned. Both transforms are independently proven byte-identical
// (TestEquiv_PS2128BuilderAccumulator, TestEquiv_PS2103SprintfSpliceToConcat);
// this locks that they COMPOSE correctly on one node. On the real stdlib the
// full report AND -diff ran crash-free across ~17 package trees, and every -diff
// proposal was bit-identical.
func TestFixStdlibSprintfBuilderCompose(t *testing.T) {
	const src = `package p

import "fmt"

func render(fields []struct{ Name, Type string }) string {
	str := ""
	for _, f := range fields {
		str += fmt.Sprintf("%s %s; ", f.Name, f.Type)
	}
	return str
}
`
	got := string(runFixMode(t, src))

	for _, want := range []string{
		"var str strings.Builder",
		`str.WriteString(f.Name + " " + f.Type + "; ")`,
		"return str.String()",
		`"strings"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in the composed fix:\n%s", want, got)
		}
	}
	if strings.Contains(got, "fmt.Sprintf") || strings.Contains(got, `"fmt"`) {
		t.Errorf("fmt (and its import) should be gone once the only Sprintf is rewritten:\n%s", got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

// TestFixMinioUtf8AndSlicesImportCompose pins a two-import-family composition
// observed during corpus -fix validation on minio (internal/s3select/sql +
// bucket packages): PS2125 rewrites len([]rune(s)) -> utf8.RuneCountInString(s)
// (adding "unicode/utf8") in the same file where PS3104 rewrites sort.Strings ->
// slices.Sort (adding "slices" and orphaning "sort"). The runner must, in one
// pass, add BOTH new imports, prune the now-unused "sort", and produce a file
// that parses. This is distinct from the io/slices/fmt import pins above — it
// exercises the unicode/utf8 add alongside a slices add + sort prune.
//
// On the real minio this was part of 42 bit-identical fixes across 23 files; the
// tree built, vetted, and every changed package's tests passed. Lock the mix.
func TestFixMinioUtf8AndSlicesImportCompose(t *testing.T) {
	const src = `package p

import "sort"

func f(names []string, s string) (int, []string) {
	sort.Strings(names)
	n := len([]rune(s))
	return n, names
}
`
	got := string(runFixMode(t, src))

	for _, want := range []string{
		"slices.Sort(names)",
		"utf8.RuneCountInString(s)",
		`"slices"`,
		`"unicode/utf8"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in the composed fix:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"sort"`) {
		t.Errorf("sort import should be pruned as an orphan:\n%s", got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

// TestFixCaddyThreeCheckImportChurn pins a rich composition observed during
// corpus -fix validation on caddyserver/caddy (a strings.Builder helper that
// sorts, writes single-byte runes, and hex-encodes a key): THREE checks fire in
// one file with a two-add / two-prune import result — PS3104 (sort.Strings ->
// slices.Sort: add slices, orphan sort), PS2107 %x (fmt.Sprintf("%x", b) ->
// hex.EncodeToString: add encoding/hex, orphan fmt), and PS5102 (WriteRune ->
// WriteByte, import-neutral) — while "strings" (still used by the Builder) stays.
// The runner must, in one pass, add both new imports, prune both orphans, keep
// the live one, and produce a file that parses. On the real caddy this was part
// of 18 bit-identical fixes across 12 files; the tree built, vetted, and all
// changed packages' tests passed.
func TestFixCaddyThreeCheckImportChurn(t *testing.T) {
	const src = `package p

import (
	"fmt"
	"sort"
	"strings"
)

func render(names []string, key []byte) string {
	sort.Strings(names)
	var sb strings.Builder
	sb.WriteRune('"')
	sb.WriteRune('-')
	return sb.String() + fmt.Sprintf("%x", key)
}
`
	got := string(runFixMode(t, src))

	for _, want := range []string{
		"slices.Sort(names)",
		`sb.WriteByte('"')`,
		"sb.WriteByte('-')",
		"hex.EncodeToString(key)",
		`"slices"`,
		`"encoding/hex"`,
		`"strings"`, // still used by strings.Builder — must survive
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in the composed fix:\n%s", want, got)
		}
	}
	for _, gone := range []string{`"sort"`, `"fmt"`} {
		if strings.Contains(got, gone) {
			t.Errorf("import %s should have been pruned as an orphan:\n%s", gone, got)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

// TestFixPrometheusVerbArmsCompose pins a composition observed during corpus
// -fix validation on prometheus/prometheus, where the recent PS2107 verb arms
// (%g -> strconv.FormatFloat, %q -> strconv.Quote) fired on real code
// (fmt.Sprintf("%g", bucket) in a histogram bucket formatter, fmt.Sprintf("%q",
// item.Val) in the promql parser). Both arms need "strconv" — the runner must
// add it exactly ONCE — while PS3104 (sort.Strings -> slices.Sort) adds "slices"
// and orphans "sort", PS5102 (WriteRune -> WriteByte) is import-neutral, and
// "strings" (Builder) stays. FP-stress note: the recent advisory family
// (PS2131-2134) fired ZERO times across all of prometheus AND the Go stdlib.
//
// On the real prometheus this was part of 22 bit-identical fixes across 11 files;
// the tree built and all changed packages' tests passed.
func TestFixPrometheusVerbArmsCompose(t *testing.T) {
	const src = `package p

import (
	"fmt"
	"sort"
	"strings"
)

type item struct{ Val string }

func render(names []string, buckets []float64, it item) (string, []string) {
	sort.Strings(names)
	var sb strings.Builder
	sb.WriteRune('[')
	var out []string
	for _, b := range buckets {
		out = append(out, fmt.Sprintf("%g", b))
	}
	return fmt.Sprintf("%q", it.Val), append(out, sb.String())
}
`
	got := string(runFixMode(t, src))

	for _, want := range []string{
		"slices.Sort(names)",
		"sb.WriteByte('[')",
		"strconv.FormatFloat(b, 'g', -1, 64)", // %g arm, in an append arg
		"strconv.Quote(it.Val)",               // %q arm, field-selector arg
		`"slices"`,
		`"strconv"`,
		`"strings"`, // Builder still uses it
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in the composed fix:\n%s", want, got)
		}
	}
	// strconv added exactly once despite two arms needing it.
	if n := strings.Count(got, `"strconv"`); n != 1 {
		t.Errorf(`"strconv" import should appear exactly once, got %d:\n%s`, n, got)
	}
	for _, gone := range []string{`"sort"`, `"fmt"`} {
		if strings.Contains(got, gone) {
			t.Errorf("import %s should have been pruned as an orphan:\n%s", gone, got)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

// TestFixHoistFamilyComposesInOneFunc pins the interaction of the three
// hoist-family AutoFixes — PS2127 (regexp.MustCompile), PS2132
// (strings.NewReplacer), PS2134 (template.Must(New().Parse)) — when they all
// fire inside the SAME function. Each inserts a fresh package-level var at the
// same insertion point (immediately before the enclosing function, ahead of its
// doc comment) and rewrites its own call site. The runner must apply all three
// same-position insertions without collision, keep every import still used by
// the hoisted vars (regexp, strings, text/template) intact, leave the call sites
// referring to the new vars, and produce a file that parses. Each check seeds its
// var name from the function's source line, so the three names share that line
// number but differ by prefix (psRegexpL / psReplacerL / psTemplateL) — this pins
// that the distinct prefixes prevent a name clash. Verified end-to-end (compiles,
// bit-identical output) before pinning here.
func TestFixHoistFamilyComposesInOneFunc(t *testing.T) {
	const src = `package p

import (
	"io"
	"regexp"
	"strings"
	"text/template"
)

const tmplText = "Hi {{.}}"

// process is the doc comment: the hoisted vars must land ABOVE this line.
func process(w io.Writer, s string) string {
	if regexp.MustCompile("^a+$").MatchString(s) {
		s = strings.NewReplacer("a", "b").Replace(s)
	}
	t := template.Must(template.New("t").Parse(tmplText))
	_ = t.Execute(w, s)
	return s
}
`
	got := string(runFixMode(t, src))

	// All three call sites rewritten to the hoisted vars.
	for _, want := range []string{
		"psRegexpL", "psReplacerL", "psTemplateL",
		".MatchString(s)", ".Replace(s)", "t.Execute(w, s)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in the composed fix:\n%s", want, got)
		}
	}
	// The original inline forms are gone from the function body.
	for _, gone := range []string{
		`regexp.MustCompile("^a+$").MatchString`,
		`strings.NewReplacer("a", "b").Replace`,
	} {
		if strings.Contains(got, gone) {
			t.Errorf("inline form %q should have been hoisted away:\n%s", gone, got)
		}
	}
	// Every import is still used by a hoisted var (or the params) — none pruned.
	for _, want := range []string{`"io"`, `"regexp"`, `"strings"`, `"text/template"`} {
		if !strings.Contains(got, want) {
			t.Errorf("import %s must remain (still used by a hoisted var):\n%s", want, got)
		}
	}
	// The doc comment must not be wedged between a hoisted var and the func: the
	// comment stays immediately above `func process`.
	if !strings.Contains(got, "// process is the doc comment: the hoisted vars must land ABOVE this line.\nfunc process(") {
		t.Errorf("doc comment must stay directly above func process:\n%s", got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

// TestFixBboltRegexpHoistRawStringPattern pins a PS2127 hoist observed in the
// wild during corpus -fix validation on etcd-io/bbolt (internal/btesting/
// btesting.go, truncDuration): an inline regexp.MustCompile of a RAW-STRING
// (backtick) pattern containing backslash escapes, chained to ReplaceAllString.
// The existing ps2127 golden uses an interpreted-string pattern ("^a+$"); this
// locks the raw-string case, where the hoisted var must reproduce the pattern
// literal byte-for-byte (backticks + `\d` intact — an interpreted-string
// rewrite would mangle the escapes). On the real file this rewrote cleanly and
// bbolt built + its changed-package tests passed; lock that outcome here.
func TestFixBboltRegexpHoistRawStringPattern(t *testing.T) {
	const src = "package p\n" +
		"\n" +
		"import (\n" +
		"\t\"regexp\"\n" +
		"\t\"time\"\n" +
		")\n" +
		"\n" +
		"func truncDuration(d time.Duration) string {\n" +
		"\treturn regexp.MustCompile(`^(\\d+)(\\.\\d+)`).ReplaceAllString(d.String(), \"$1\")\n" +
		"}\n"
	got := string(runFixMode(t, src))

	// The pattern literal must survive verbatim as a raw string in the hoisted var.
	if !strings.Contains(got, "= regexp.MustCompile(`^(\\d+)(\\.\\d+)`)") {
		t.Errorf("raw-string pattern must be hoisted byte-for-byte (backticks + escapes intact):\n%s", got)
	}
	// The call site now uses the hoisted var and keeps the read-only method.
	if !strings.Contains(got, ".ReplaceAllString(d.String(), \"$1\")") {
		t.Errorf("call site must keep ReplaceAllString on the hoisted var:\n%s", got)
	}
	if strings.Contains(got, "regexp.MustCompile(`^(\\d+)(\\.\\d+)`).ReplaceAllString") {
		t.Errorf("inline compile-and-use should have been hoisted away:\n%s", got)
	}
	// Both imports remain (regexp via the hoisted var, time via the param).
	for _, want := range []string{`"regexp"`, `"time"`} {
		if !strings.Contains(got, want) {
			t.Errorf("import %s must remain:\n%s", want, got)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

// TestFixBadgerMultiKeySortComposition pins a composition observed in the wild
// during corpus -fix validation on dgraph-io/badger (levels.go): one file where
// PS3002 rewrites a MULTI-KEY sort.Slice (a nested Level-then-ID comparator over
// NON-FLOAT fields) to slices.SortFunc with cmp.Compare, and PS3104 rewrites a
// sort.Strings to slices.Sort. Both checks ADD "slices" (must dedupe to one),
// PS3002 also ADDS "cmp", and "sort" — whose only two uses are both rewritten —
// must be pruned as an orphan. This exercises the multi-key cmp.Compare rewrite
// (bit-identical only for non-float keys) together with the shared import
// machinery on a realistic two-sort function. On the real file this rewrote
// cleanly and badger built + its changed-package tests passed.
func TestFixBadgerMultiKeySortComposition(t *testing.T) {
	const src = `package p

import "sort"

type TableInfo struct {
	Level int
	ID    uint64
}

func order(result []TableInfo, splits []string) []string {
	sort.Slice(result, func(i, j int) bool {
		if result[i].Level != result[j].Level {
			return result[i].Level < result[j].Level
		}
		return result[i].ID < result[j].ID
	})
	sort.Strings(splits)
	return splits
}
`
	got := string(runFixMode(t, src))

	for _, want := range []string{
		"slices.SortFunc(result, func(a, b TableInfo) int {",
		"cmp.Compare(a.Level, b.Level)",
		"cmp.Compare(a.ID, b.ID)",
		"slices.Sort(splits)",
		`"cmp"`,
		`"slices"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in the composed fix:\n%s", want, got)
		}
	}
	// "slices" added by BOTH checks must appear exactly once.
	if n := strings.Count(got, `"slices"`); n != 1 {
		t.Errorf(`"slices" import should appear exactly once, got %d:\n%s`, n, got)
	}
	// "sort" had only the two now-rewritten uses — it must be pruned.
	if strings.Contains(got, `"sort"`) {
		t.Errorf(`"sort" should have been pruned as an orphan:\n%s`, got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

// TestFixGoreleaserTemplateHoist pins a PS2134 hoist observed in the wild during
// corpus -fix validation on goreleaser/goreleaser (internal/pipe/release/body.go,
// describeBody): a bound local `bodyTemplate := template.Must(template.New(
// "release").Parse(bodyTemplateText))` whose template text is a package-level
// const IDENTIFIER (a multi-line raw string), used read-only via a single
// .Execute. The fix hoists the whole template.Must(...) to a package var — the
// const-ident text is preserved verbatim (NOT inlined) and the call site becomes
// an alias assignment. On the real repo this rewrote cleanly and the release
// package's body-template tests passed (byte-identical output).
func TestFixGoreleaserTemplateHoist(t *testing.T) {
	const src = "package p\n" +
		"\n" +
		"import (\n" +
		"\t\"bytes\"\n" +
		"\t\"text/template\"\n" +
		")\n" +
		"\n" +
		"const bodyTemplateText = `{{ with .Header }}{{ . }}{{ end }}{{ .ReleaseNotes }}`\n" +
		"\n" +
		"func describeBody(header, notes string) (bytes.Buffer, error) {\n" +
		"\tvar out bytes.Buffer\n" +
		"\tbodyTemplate := template.Must(template.New(\"release\").Parse(bodyTemplateText))\n" +
		"\terr := bodyTemplate.Execute(&out, struct{ Header, ReleaseNotes string }{header, notes})\n" +
		"\treturn out, err\n" +
		"}\n"
	got := string(runFixMode(t, src))

	// Hoisted to a package var; the const-ident text is preserved (not inlined).
	if !strings.Contains(got, "= template.Must(template.New(\"release\").Parse(bodyTemplateText))") {
		t.Errorf("template.Must should be hoisted with the const-ident text preserved verbatim:\n%s", got)
	}
	if !strings.Contains(got, "psTemplateL") {
		t.Errorf("expected a psTemplateL<line> package var:\n%s", got)
	}
	// The call site keeps the Execute on the (now aliased) local.
	if !strings.Contains(got, "bodyTemplate.Execute(&out,") {
		t.Errorf("call site must keep bodyTemplate.Execute:\n%s", got)
	}
	// The inline create-and-parse is gone from the function body.
	if strings.Contains(got, "bodyTemplate := template.Must(template.New(\"release\").Parse(bodyTemplateText))") {
		t.Errorf("inline template.Must should have been hoisted away:\n%s", got)
	}
	// Both imports remain (template via the hoisted var, bytes via the local).
	for _, want := range []string{`"bytes"`, `"text/template"`} {
		if !strings.Contains(got, want) {
			t.Errorf("import %s must remain:\n%s", want, got)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

// TestFixBluemondayLenSplitToCount pins a PS2121 rewrite observed in the wild
// during corpus -fix validation on microcosm-cc/bluemonday (css/handlers.go):
// `len(strings.Split(i, "/")) == 2` inside an if-condition becomes
// `(strings.Count(i, "/") + 1) == 2`. Two things this locks on a real comparison
// context: the separator "/" is a provably NON-EMPTY constant (the guard PS2121
// requires, since Split(s,"") and Count(s,"")+1 disagree), and the `Count(...) + 1`
// replacement is PARENTHESIZED so it binds correctly as the left operand of ==
// (without the parens, `strings.Count(i,"/") + 1 == 2` would parse as
// `Count + (1 == 2)` and not compile). On the real file this rewrote cleanly and
// bluemonday built + its tests passed.
func TestFixBluemondayLenSplitToCount(t *testing.T) {
	const src = `package p

import "strings"

func hasTwoParts(i string) bool {
	if len(strings.Split(i, "/")) == 2 {
		return true
	}
	return false
}
`
	got := string(runFixMode(t, src))

	if !strings.Contains(got, "if (strings.Count(i, \"/\") + 1) == 2 {") {
		t.Errorf("expected the parenthesized Count(...)+1 rewrite as the == operand:\n%s", got)
	}
	if strings.Contains(got, "len(strings.Split(i, \"/\"))") {
		t.Errorf("the len(Split()) form should have been rewritten:\n%s", got)
	}
	if !strings.Contains(got, `"strings"`) {
		t.Errorf("strings import must remain (Count still uses it):\n%s", got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

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

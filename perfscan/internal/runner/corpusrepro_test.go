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

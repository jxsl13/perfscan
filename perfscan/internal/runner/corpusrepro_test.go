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

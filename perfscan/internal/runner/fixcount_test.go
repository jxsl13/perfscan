package runner

import (
	"bytes"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// Regression: patchedFiles must count a fix as "applied" only once its target
// file is actually written. A file skipped in the write loop (unreadable /
// overlapping edits / offsets out of range for the on-disk bytes, e.g. a
// cgo-processed TU) marks its fixes FAILED. Previously applied was incremented
// per finding up front, so "applied N" overstated the count whenever the file
// was later skipped.
func TestFixCountExcludesSkippedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	content := []byte("package x\n") // 10 bytes on disk
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	// Declare the token.File LARGER than the on-disk bytes so we can mint edit
	// positions past the real end — modeling a parsed TU whose bytes differ
	// from the source (the out-of-range skip path).
	fset := token.NewFileSet()
	tf := fset.AddFile(path, -1, 100)

	mkFinding := func(startOff, endOff int) Finding {
		return Finding{
			Check: &lint.Check{ID: "PS9999", AutoFix: true},
			fset:  fset,
			Fixes: []analysis.SuggestedFix{{
				TextEdits: []analysis.TextEdit{{
					Pos:     tf.Pos(startOff),
					End:     tf.Pos(endOff),
					NewText: []byte("Y"),
				}},
			}},
		}
	}

	opts := Options{Stderr: io.Discard}

	// Edit offsets 50..60 exceed the 10-byte file → the file is skipped, so the
	// fix is a genuine FAILURE (not applied, not a benign overlap).
	if _, applied, overlap, failed := patchedFiles([]Finding{mkFinding(50, 60)}, &opts); applied != 0 || overlap != 0 || failed != 1 {
		t.Errorf("out-of-range fix: applied=%d overlap=%d failed=%d, want 0/0/1", applied, overlap, failed)
	}

	// Edit offsets 0..8 are within the file → the fix is applied.
	if _, applied, overlap, failed := patchedFiles([]Finding{mkFinding(0, 8)}, &opts); applied != 1 || overlap != 0 || failed != 0 {
		t.Errorf("in-range fix: applied=%d overlap=%d failed=%d, want 1/0/0", applied, overlap, failed)
	}

	// Two fixes onto the same skipped file: BOTH count as failed, neither
	// applied (they share the file's fate).
	if _, applied, overlap, failed := patchedFiles([]Finding{mkFinding(50, 60), mkFinding(70, 80)}, &opts); applied != 0 || overlap != 0 || failed != 2 {
		t.Errorf("two out-of-range fixes: applied=%d overlap=%d failed=%d, want 0/0/2", applied, overlap, failed)
	}
}

// Regression: two checks can emit overlapping fixes for the same span (e.g.
// PS2103 and PS2122 both rewrite one fmt.Sprintf, via different edits).
// patchedFiles applies a maximal NON-overlapping set — dropping a whole
// conflicting fix — instead of skipping the entire file and losing every fix
// in it.
func TestFixConflictResolution(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.go")
	content := []byte("package p\nvar x = 100000\n") // 25 bytes; "100000" at [18,24)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	tf := fset.AddFile(path, -1, len(content))

	mk := func(startOff, endOff int, text string) Finding {
		return Finding{
			Check: &lint.Check{ID: "PS9999", AutoFix: true},
			fset:  fset,
			Fixes: []analysis.SuggestedFix{{
				TextEdits: []analysis.TextEdit{{
					Pos:     tf.Pos(startOff),
					End:     tf.Pos(endOff),
					NewText: []byte(text),
				}},
			}},
		}
	}
	opts := Options{Stderr: io.Discard}

	// A replaces [18,24] ("100000" -> "1"); B replaces [20,22] (inside A) —
	// they overlap. A starts first, so A wins and B is counted OVERLAPPING (a
	// benign drop, not a failure): 1 applied, 1 overlapping, 0 failed, and the
	// file IS written with A's rewrite (not skipped whole).
	files, applied, overlap, failed := patchedFiles([]Finding{mk(18, 24, "1"), mk(20, 22, "9")}, &opts)
	if applied != 1 || overlap != 1 || failed != 0 {
		t.Fatalf("conflicting fixes: applied=%d overlap=%d failed=%d, want 1/1/0", applied, overlap, failed)
	}
	pf, ok := files[path]
	if !ok || !bytes.Contains(pf.fixed, []byte("var x = 1\n")) {
		t.Fatalf("winning fix not applied; fixed=%q", pf.fixed)
	}

	// Non-overlapping fixes both apply: C replaces "x" [14,15], D replaces
	// "100000" [18,24] — disjoint, so 2 applied, 0 overlapping, 0 failed.
	if _, applied, overlap, failed := patchedFiles([]Finding{mk(14, 15, "y"), mk(18, 24, "1")}, &opts); applied != 2 || overlap != 0 || failed != 0 {
		t.Errorf("disjoint fixes: applied=%d overlap=%d failed=%d, want 2/0/0", applied, overlap, failed)
	}
}

// Regression: a single (malformed) fix whose OWN edits overlap must be rejected
// whole, not spliced — the greedy loop applies an accepted fix's edits
// verbatim, so applying overlapping ones would corrupt the file. The file is
// left unmodified.
func TestSelfOverlappingFixRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "q.go")
	content := []byte("package q\nvar x = 100000\n") // "100000" at [18,24)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	tf := fset.AddFile(path, -1, len(content))

	f := Finding{
		Check: &lint.Check{ID: "PS9999", AutoFix: true},
		fset:  fset,
		Fixes: []analysis.SuggestedFix{{
			TextEdits: []analysis.TextEdit{
				{Pos: tf.Pos(18), End: tf.Pos(24), NewText: []byte("A")}, // [18,24)
				{Pos: tf.Pos(20), End: tf.Pos(22), NewText: []byte("B")}, // [20,22) — overlaps
			},
		}},
	}
	// A self-overlapping (malformed) fix is a genuine FAILURE — not a benign
	// overlap of a different fix — because applying it would corrupt the file.
	files, applied, overlap, failed := patchedFiles([]Finding{f}, &Options{Stderr: io.Discard})
	if applied != 0 || overlap != 0 || failed != 1 {
		t.Fatalf("self-overlapping fix: applied=%d overlap=%d failed=%d, want 0/0/1", applied, overlap, failed)
	}
	// With no fix applied, the file must not be written at all (not even a
	// gofmt-only rewrite) — so it is absent from the patched set.
	if _, ok := files[path]; ok {
		t.Fatalf("file must be left untouched when its only fix is rejected, but it was patched")
	}
}

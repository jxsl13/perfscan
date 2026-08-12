package runner

import (
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
	// fix is FAILED, not applied.
	if _, applied, failed := patchedFiles([]Finding{mkFinding(50, 60)}, opts); applied != 0 || failed != 1 {
		t.Errorf("out-of-range fix: applied=%d failed=%d, want applied=0 failed=1", applied, failed)
	}

	// Edit offsets 0..8 are within the file → the fix is applied.
	if _, applied, failed := patchedFiles([]Finding{mkFinding(0, 8)}, opts); applied != 1 || failed != 0 {
		t.Errorf("in-range fix: applied=%d failed=%d, want applied=1 failed=0", applied, failed)
	}

	// Two fixes onto the same skipped file: BOTH count as failed, neither
	// applied (they share the file's fate).
	if _, applied, failed := patchedFiles([]Finding{mkFinding(50, 60), mkFinding(70, 80)}, opts); applied != 0 || failed != 2 {
		t.Errorf("two out-of-range fixes: applied=%d failed=%d, want applied=0 failed=2", applied, failed)
	}
}

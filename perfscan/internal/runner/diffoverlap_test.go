package runner

import (
	"bytes"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// diffFixes must report an overlapping (benign) fix drop distinctly from a
// genuine render failure, and still show the winning fix's diff — mirroring the
// -fix summary's "skipped (overlaps an applied fix)".
func TestDiffOverlappingFixReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.go")
	content := []byte("package p\nvar x = 100000\n") // "100000" at [18,24)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	tf := fset.AddFile(path, -1, len(content))
	mk := func(s, e int, text string) Finding {
		return Finding{
			Check: &lint.Check{ID: "PS9999", AutoFix: true},
			fset:  fset,
			Fixes: []analysis.SuggestedFix{{TextEdits: []analysis.TextEdit{{
				Pos: tf.Pos(s), End: tf.Pos(e), NewText: []byte(text),
			}}}},
		}
	}

	var out, errBuf bytes.Buffer
	// A [18,24] and B [20,22] overlap; A wins, B is a benign overlap drop.
	code := diffFixes([]Finding{mk(18, 24, "1"), mk(20, 22, "9")}, &Options{Stdout: &out, Stderr: &errBuf})
	if code != 1 {
		t.Errorf("diffFixes with a pending change: code=%d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), "overlapping fix(es) not shown") {
		t.Errorf("stderr should note the overlapping drop distinctly; got:\n%s", errBuf.String())
	}
	if strings.Contains(errBuf.String(), "could not be rendered") {
		t.Errorf("an overlap is benign, not a render failure; stderr:\n%s", errBuf.String())
	}
	if !strings.Contains(out.String(), "var x = 1") {
		t.Errorf("the winning fix's diff was not rendered; stdout:\n%s", out.String())
	}
}

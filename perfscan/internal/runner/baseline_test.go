package runner

import (
	"go/token"
	"path/filepath"
	"testing"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

func fakeFinding(file, id, msg string, line int) Finding {
	return Finding{
		Check:   &lint.Check{ID: id},
		Pos:     token.Position{Filename: file, Line: line, Column: 1},
		Message: msg,
	}
}

func TestBaselineRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	old := []Finding{
		fakeFinding("a.go", "PS2101", "out is appended", 10),
		fakeFinding("a.go", "PS2101", "out is appended", 42), // same key, count 2
		fakeFinding("b.go", "PS3003", "map probed", 7),
	}
	if err := writeBaseline(path, old); err != nil {
		t.Fatal(err)
	}

	// Same findings at shifted lines: all suppressed.
	shifted := []Finding{
		fakeFinding("a.go", "PS2101", "out is appended", 12),
		fakeFinding("a.go", "PS2101", "out is appended", 44),
		fakeFinding("b.go", "PS3003", "map probed", 9),
	}
	surviving, suppressed, err := applyBaseline(path, shifted)
	if err != nil {
		t.Fatal(err)
	}
	if suppressed != 3 || len(surviving) != 0 {
		t.Fatalf("want 3 suppressed, 0 surviving; got %d, %d", suppressed, len(surviving))
	}

	// A regression (third occurrence of a key baselined at count 2, plus a
	// brand-new finding) survives.
	regressed := append(shifted,
		fakeFinding("a.go", "PS2101", "out is appended", 99),
		fakeFinding("c.go", "PS2005", "regexp in loop", 5),
	)
	surviving, suppressed, err = applyBaseline(path, regressed)
	if err != nil {
		t.Fatal(err)
	}
	if suppressed != 3 || len(surviving) != 2 {
		t.Fatalf("want 3 suppressed, 2 surviving; got %d, %d", suppressed, len(surviving))
	}
	if surviving[0].Pos.Line != 99 || surviving[1].Check.ID != "PS2005" {
		t.Fatalf("unexpected survivors: %+v", surviving)
	}
}

func TestBaselineMissingFile(t *testing.T) {
	_, _, err := applyBaseline(filepath.Join(t.TempDir(), "nope.json"), nil)
	if err == nil {
		t.Fatal("want error for missing baseline file")
	}
}

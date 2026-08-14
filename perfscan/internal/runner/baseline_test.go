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

// TestBaselineRoundTripsSpecialCharMessage pins that a finding message carrying
// YAML-hostile characters survives writeBaseline -> applyBaseline intact, so its
// line-independent key (relPath + ID + message) still matches and the finding
// stays suppressed. TestBaselineRoundTrip uses only plain messages, so the
// escaping/round-trip of a message with colons, quotes, apostrophes, a percent
// sign, or a leading dash was untested. A one-byte change across the round-trip
// would resurface the baselined finding as a false regression.
func TestBaselineRoundTripsSpecialCharMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")
	msg := `- 'variable' "x": copied, per #note: costs 100% more`
	if err := writeBaseline(path, []Finding{fakeFinding("a.go", "PS2101", msg, 10)}); err != nil {
		t.Fatal(err)
	}
	// The same finding at a shifted line: line-independent, so it must be
	// suppressed — but only if the special-char message round-tripped exactly.
	surviving, suppressed, err := applyBaseline(path, []Finding{fakeFinding("a.go", "PS2101", msg, 99)})
	if err != nil {
		t.Fatal(err)
	}
	if suppressed != 1 || len(surviving) != 0 {
		t.Errorf("a baselined finding with a special-char message must stay suppressed (message must round-trip exactly); surviving=%v suppressed=%d", surviving, suppressed)
	}
}

func TestBaselineMissingFile(t *testing.T) {
	_, _, err := applyBaseline(filepath.Join(t.TempDir(), "nope.json"), nil)
	if err == nil {
		t.Fatal("want error for missing baseline file")
	}
}

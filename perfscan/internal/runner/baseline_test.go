package runner

import (
	"bytes"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscan/checks"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// TestBaselineRunFlowSeedFilterAndWriteGuard drives the baseline flow through
// Run() (not just writeBaseline/applyBaseline): -write-baseline SEEDS the file
// and short-circuits (exit 0); a later -baseline run FILTERS — suppressing
// everything on an identical corpus and, crucially, leaving the baseline file
// byte-for-byte UNCHANGED (a filter run must never rewrite it, or the ratchet
// would silently absorb regressions); and -write-baseline WITHOUT -baseline is a
// usage error (exit 2). Only writeBaseline/applyBaseline were unit tested; this
// covers the Run() wiring.
func TestBaselineRunFlowSeedFilterAndWriteGuard(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(corpusGoMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(corpusMain), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()
	blPath := filepath.Join(dir, "baseline.yaml")

	run := func(o Options) (string, int) {
		var out, errBuf bytes.Buffer
		o.Patterns = []string{"./..."}
		o.MaxLevel = lint.LevelAggressive
		o.Stdout, o.Stderr = &out, &errBuf
		code := Run(checks.All(), o)
		return out.String() + errBuf.String(), code
	}

	// Seed: -write-baseline writes the file and exits 0.
	out, code := run(Options{Baseline: blPath, WriteBaseline: true})
	if code != 0 {
		t.Fatalf("seed run: exit %d, want 0\n%s", code, out)
	}
	seeded, err := os.ReadFile(blPath)
	if err != nil {
		t.Fatalf("seed run should have written %s: %v", blPath, err)
	}
	if len(seeded) == 0 {
		t.Fatal("seeded baseline is empty; the corpus should produce findings")
	}

	// Filter: -baseline (no -write) suppresses everything (identical corpus),
	// exits 0, and must NOT modify the baseline file.
	out, code = run(Options{Baseline: blPath})
	if code != 0 {
		t.Fatalf("filter run: exit %d, want 0 (all baselined)\n%s", code, out)
	}
	after, err := os.ReadFile(blPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(seeded) {
		t.Errorf("a filter run must NOT rewrite the baseline:\nbefore:\n%s\nafter:\n%s", seeded, after)
	}

	// Guard: -write-baseline without -baseline is a usage error (exit 2).
	out, code = run(Options{WriteBaseline: true})
	if code != 2 || !strings.Contains(out, "-write-baseline requires -baseline") {
		t.Errorf("-write-baseline without -baseline: exit %d, want 2 with the requires-baseline message\n%s", code, out)
	}
}

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

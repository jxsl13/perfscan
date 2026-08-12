package runner

import (
	"os"
	"path/filepath"
	"testing"

	"go/token"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// TestFilterIgnored covers the //perfscan:ignore suppression directive: a bare
// directive silences every check on its line, a directive naming IDs silences
// only those, a directive on the line directly ABOVE a finding also applies, and
// reason text after the IDs does not accidentally suppress unrelated checks. A
// bug here means users either can't silence a false positive or, worse, silently
// lose real findings.
func TestFilterIgnored(t *testing.T) {
	// Each source line carries (or doesn't) a directive; findings below anchor to
	// these 1-based line numbers. Plain separator lines keep each directive from
	// bleeding onto a neighbor via the line-above rule.
	const src = "" +
		"a := 1 // plain, no directive\n" + // 1
		"b := 2 //perfscan:ignore\n" + // 2  bare -> suppresses any check
		"sep := 0\n" + // 3  separator
		"c := 4 //perfscan:ignore PS2101\n" + // 4  targeted -> only PS2101
		"sep = 0\n" + // 5  separator
		"d := 6 //perfscan:ignore PS9999 because reasons\n" + // 6  targets PS9999 + reason text
		"sep = 0\n" + // 7  separator
		"//perfscan:ignore PS2101\n" + // 8  directive on the line above line 9
		"e := 9\n" // 9  suppressed via the line above

	dir := t.TempDir()
	path := filepath.Join(dir, "f.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	mk := func(id string, line int) Finding {
		return Finding{
			Check: &lint.Check{ID: id},
			Pos:   token.Position{Filename: path, Line: line},
		}
	}

	cases := []struct {
		name    string
		f       Finding
		wantOut bool // true => survives (reported); false => suppressed
	}{
		{"no directive survives", mk("PS2101", 1), true},
		{"bare directive suppresses any id", mk("PS2101", 2), false},
		{"bare directive suppresses other id too", mk("PS5104", 2), false},
		{"targeted directive suppresses named id", mk("PS2101", 4), false},
		{"targeted directive spares other id", mk("PS5104", 4), true},
		{"directive for a different id spares this one", mk("PS2101", 6), true},
		{"directive targeting this id suppresses it", mk("PS9999", 6), false},
		{"reason text after id is not treated as an id", mk("PS0000", 6), true},
		{"directive on the line above suppresses", mk("PS2101", 9), false},
	}

	// Feed every case through at once so the survivors set is exactly the ones
	// we expect (also exercises the shared per-file line cache).
	in := make([]Finding, len(cases))
	var wantIDsAtLines [][2]any
	for i, c := range cases {
		in[i] = c.f
		if c.wantOut {
			wantIDsAtLines = append(wantIDsAtLines, [2]any{c.f.Check.ID, c.f.Pos.Line})
		}
	}

	got := filterIgnored(in)

	// Assert per-case membership by (id,line) identity.
	survived := func(id string, line int) bool {
		for _, f := range got {
			if f.Check.ID == id && f.Pos.Line == line {
				return true
			}
		}
		return false
	}
	for _, c := range cases {
		if s := survived(c.f.Check.ID, c.f.Pos.Line); s != c.wantOut {
			t.Errorf("%s: %s@L%d survived=%v, want %v",
				c.name, c.f.Check.ID, c.f.Pos.Line, s, c.wantOut)
		}
	}
	if len(got) != len(wantIDsAtLines) {
		t.Errorf("survivor count = %d, want %d", len(got), len(wantIDsAtLines))
	}
}

// TestFilterIgnoredMissingFileReportsAll ensures a finding whose file cannot be
// read is never silently dropped (an unreadable file must not swallow findings).
func TestFilterIgnoredMissingFileReportsAll(t *testing.T) {
	f := Finding{
		Check: &lint.Check{ID: "PS2101"},
		Pos:   token.Position{Filename: filepath.Join(t.TempDir(), "nope.go"), Line: 1},
	}
	got := filterIgnored([]Finding{f})
	if len(got) != 1 {
		t.Fatalf("unreadable file: got %d findings, want 1 (must not suppress)", len(got))
	}
}

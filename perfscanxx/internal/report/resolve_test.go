package report

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolvePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-style absolute-path fixtures; resolvePath uses path/filepath and is exercised with native paths at runtime")
	}
	abs := filepath.FromSlash("/abs/main.cpp")
	cases := []struct{ fp, build, main, want string }{
		{filepath.FromSlash("/x/a.cpp"), "/build", "/m/main.cpp", filepath.FromSlash("/x/a.cpp")},  // absolute wins
		{"a.cpp", filepath.FromSlash("/build"), "/m/main.cpp", filepath.FromSlash("/build/a.cpp")}, // join build dir
		{"a.cpp", "", filepath.FromSlash("/m/main.cpp"), filepath.FromSlash("/m/a.cpp")},           // fall back to main src dir
		{"", "", abs, abs}, // empty file path -> main source
	}
	for _, c := range cases {
		if got := resolvePath(c.fp, c.build, c.main); got != c.want {
			t.Errorf("resolvePath(%q,%q,%q)=%q want %q", c.fp, c.build, c.main, got, c.want)
		}
	}
}

func TestLineColFromOffset(t *testing.T) {
	// "ab\ncde\nf": a0 b1 nl2 c3 d4 e5 nl6 f7 — offset 4 ('d') is line 2 col 2.
	old := ReadFile
	ReadFile = func(string) ([]byte, error) { return []byte("ab\ncde\nf"), nil }
	defer func() { ReadFile = old }()
	if l, c := lineCol("x", 4); l != 2 || c != 2 {
		t.Errorf("lineCol offset4 = %d:%d want 2:2", l, c)
	}
	if l, c := lineCol("x", 0); l != 1 || c != 1 {
		t.Errorf("lineCol offset0 = %d:%d want 1:1", l, c)
	}
}

// TestLineColIsByteBasedWithMultibyte pins that lineCol computes the column as a
// BYTE offset from the line start, matching clang-tidy's --export-fixes FileOffset
// (a byte offset) and its byte-based column convention. A multi-byte UTF-8 rune
// before the diagnostic on the same line must advance the column by its BYTE
// length, not by one rune — a rune-based "fix" would drift the reported column on
// every non-ASCII line and disagree with clang-tidy's own line:col.
func TestLineColIsByteBasedWithMultibyte(t *testing.T) {
	old := ReadFile
	defer func() { ReadFile = old }()

	// "日X\nY": 日 is 3 bytes (offsets 0-2), X at 3, \n at 4, Y at 5.
	ReadFile = func(string) ([]byte, error) { return []byte("日X\nY"), nil }
	// X is on line 1 at byte column 4 (日 occupies byte columns 1-3). Rune-based
	// counting would wrongly report column 2.
	if l, c := lineCol("x", 3); l != 1 || c != 4 {
		t.Errorf("lineCol('日X', offset 3='X') = %d:%d, want 1:4 (byte column, not 1:2 rune)", l, c)
	}
	// Y is the first byte of line 2 → column 1, unaffected by the multibyte line 1.
	if l, c := lineCol("x", 5); l != 2 || c != 1 {
		t.Errorf("lineCol offset 5 ('Y' on line 2) = %d:%d, want 2:1", l, c)
	}
}

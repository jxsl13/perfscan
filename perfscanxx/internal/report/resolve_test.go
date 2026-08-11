package report

import (
	"path/filepath"
	"testing"
)

func TestResolvePath(t *testing.T) {
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

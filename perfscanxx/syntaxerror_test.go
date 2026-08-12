package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSyntaxErrorTUIsSafe pins that a translation unit which does not parse
// never crashes perfscanxx and is never modified under -fix: clang-tidy declines
// fix-its on a non-compiling TU, so the correct behavior is to warn about the
// partial parse and leave the file untouched. Skipped when clang-tidy is absent.
func TestSyntaxErrorTUIsSafe(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found; skipping syntax-error safety test")
	}
	dir := t.TempDir()
	cpp := filepath.Join(dir, "broken.cpp")
	const broken = "void f( { int x = 0; }\n" // malformed parameter list
	if err := os.WriteFile(cpp, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"clang++ -std=c++17 -c broken.cpp"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	// Must not panic (the test fails automatically if it does).
	_, stderr, _ := runCLI("-tidy", bin, "-fix", "-p", dir, cpp)

	if got, _ := os.ReadFile(cpp); string(got) != broken {
		t.Errorf("broken.cpp was modified by -fix:\n%s", got)
	}
	if !strings.Contains(stderr, "did not fully parse") {
		t.Errorf("expected a partial-parse warning; stderr:\n%s", stderr)
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoTranslationUnitsMatched pins the empty-input edge: when a run resolves
// to zero translation units (here a compdb whose only entry references a file
// that does not exist on disk, which expandInputs skips), perfscanxx exits 2
// with a clear message instead of silently doing nothing or crashing. Hermetic
// — this fails before clang-tidy is ever invoked, so no clang-tidy is needed.
func TestNoTranslationUnitsMatched(t *testing.T) {
	dir := t.TempDir()
	ghost := filepath.Join(dir, "ghost.cpp") // referenced by the compdb, never created
	cc := `[{"directory":"` + dir + `","file":"` + ghost + `","command":"clang++ -std=c++17 -c ghost.cpp"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI("-p", dir, dir)
	if code != 2 {
		t.Errorf("no matching TUs: exit %d, want 2; stderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "no C++ translation units matched") {
		t.Errorf("expected a clear 'no C++ translation units matched' message; stderr:\n%s", stderr)
	}
}

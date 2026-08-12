package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoTranslationUnitsMatched pins the empty-input edge: when a run resolves
// to zero translation units, perfscanxx exits 2 with a clear message instead of
// silently doing nothing or crashing. Hermetic — this fails before clang-tidy
// is ever invoked, so no clang-tidy is needed.
//
// It also pins diagnoseNoMatch's two specific explanations, so a stale or
// mismatched build directory (a common trap when a checkout is copied/moved or
// a build dir is reused across trees) is named rather than surfaced as a bare
// "no units matched".
func TestNoTranslationUnitsMatched(t *testing.T) {
	// Case 1: the database lists a TU that does not exist on disk (a stale
	// build dir whose paths point at a tree that moved). Message must say so.
	t.Run("none_on_disk", func(t *testing.T) {
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
			t.Errorf("expected the 'no C++ translation units matched' message; stderr:\n%s", stderr)
		}
		if !strings.Contains(stderr, "none exist on disk") {
			t.Errorf("expected the stale-build-dir explanation (none exist on disk); stderr:\n%s", stderr)
		}
	})

	// Case 2: the database's TU DOES exist on disk, but the requested pattern
	// points at a different subtree. Message must say the TUs are rooted
	// elsewhere rather than implying there are none.
	t.Run("wrong_subtree", func(t *testing.T) {
		dir := t.TempDir()
		srcDir := filepath.Join(dir, "src")
		otherDir := filepath.Join(dir, "other")
		for _, d := range []string{srcDir, otherDir} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		real := filepath.Join(srcDir, "a.cpp")
		if err := os.WriteFile(real, []byte("int a;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cc := `[{"directory":"` + dir + `","file":"` + real + `","command":"clang++ -std=c++17 -c a.cpp"}]`
		if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
			t.Fatal(err)
		}
		// -p finds the DB in dir; the pattern points at other/ (no TUs there).
		_, stderr, code := runCLI("-p", dir, otherDir)
		if code != 2 {
			t.Errorf("wrong subtree: exit %d, want 2; stderr:\n%s", code, stderr)
		}
		if !strings.Contains(stderr, "on-disk translation unit") || !strings.Contains(stderr, "rooted near") {
			t.Errorf("expected the wrong-subtree explanation (on-disk TUs rooted near ...); stderr:\n%s", stderr)
		}
	})
}

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// exportOneFinding builds a synthetic --export-fixes YAML with a single
// performance-for-range-copy (PX1001) diagnostic at offset 5 of src, whose
// fix-it replaces "auto x" (len 6) with "const auto& x".
func exportOneFinding(src string) string {
	return "MainSourceFile: '" + src + "'\n" +
		"Diagnostics:\n" +
		"  - DiagnosticName: performance-for-range-copy\n" +
		"    DiagnosticMessage:\n" +
		"      Message: 'loop variable is copied'\n" +
		"      FilePath: '" + src + "'\n" +
		"      FileOffset: 5\n" +
		"      Replacements:\n" +
		"        - FilePath: '" + src + "'\n" +
		"          Offset: 5\n" +
		"          Length: 6\n" +
		"          ReplacementText: 'const auto& x'\n"
}

// TestExitCodeContract pins the reporting-mode exit-code contract CI pipelines
// depend on: a run that REPORTS findings without -fix must exit 1 (the gate
// trips), applying -fix must exit 0 (the gate passes once the code is fixed),
// and a clean run with no findings must exit 0. These transitions were only
// exercised indirectly before (through the -diff and baseline tests, never the
// plain report path), so a regression that flipped, say, findings-without-fix to
// exit 0 would silently disarm every CI gate using perfscanxx. Uses the stubTidy
// harness (no real clang-tidy) and a concrete file argument (no compile DB).
func TestExitCodeContract(t *testing.T) {
	const orig = "for (auto x : items) {}\n"
	const fixed = "for (const auto& x : items) {}\n"

	newSrc := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		src := filepath.Join(dir, "main.cpp")
		if err := os.WriteFile(src, []byte(orig), 0o644); err != nil {
			t.Fatal(err)
		}
		return src
	}

	t.Run("findings without -fix -> 1", func(t *testing.T) {
		src := newSrc(t)
		defer stubTidy(t, exportOneFinding(src), nil, nil)()
		_, errOut, code := runCLI(src)
		if code != 1 {
			t.Errorf("reporting findings without -fix: exit = %d, want 1\nstderr: %s", code, errOut)
		}
		if b, _ := os.ReadFile(src); string(b) != orig {
			t.Errorf("reporting mode must not modify the file, got:\n%s", b)
		}
	})

	t.Run("-fix applied -> 0", func(t *testing.T) {
		src := newSrc(t)
		var sawFix bool
		defer stubTidy(t, exportOneFinding(src), map[string]string{src: fixed}, &sawFix)()
		_, errOut, code := runCLI("-fix", src)
		if code != 0 {
			t.Errorf("-fix applied: exit = %d, want 0\nstderr: %s", code, errOut)
		}
		if !sawFix {
			t.Error("-fix must invoke clang-tidy --fix")
		}
		if b, _ := os.ReadFile(src); string(b) != fixed {
			t.Errorf("-fix must write the fixed content, got:\n%s", b)
		}
	})

	t.Run("clean (no findings) -> 0", func(t *testing.T) {
		src := newSrc(t)
		defer stubTidy(t, "", nil, nil)() // empty export = clang-tidy found nothing
		_, errOut, code := runCLI(src)
		if code != 0 {
			t.Errorf("clean run: exit = %d, want 0\nstderr: %s", code, errOut)
		}
	})
}

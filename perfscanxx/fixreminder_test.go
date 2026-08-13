package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFixLevelReviewReminder pins that -fix warns about behavior-changing fixes
// when applying at -level >= 2 (structured/aggressive), and stays quiet at
// -level 1 (idiomatic, behavior-preserving by design). Uses a synthetic
// --export-fixes with one fixable finding; clang-tidy is stubbed.
func TestFixLevelReviewReminder(t *testing.T) {
	run := func(level string) string {
		dir := t.TempDir()
		src := filepath.Join(dir, "main.cpp")
		if err := os.WriteFile(src, []byte("for (auto x : items) {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		export := "MainSourceFile: '" + src + "'\n" +
			"Diagnostics:\n" +
			"  - DiagnosticName: performance-for-range-copy\n" +
			"    DiagnosticMessage:\n" +
			"      Message: 'copied'\n" +
			"      FilePath: '" + src + "'\n" +
			"      FileOffset: 5\n" +
			"      Replacements:\n" +
			"        - FilePath: '" + src + "'\n" +
			"          Offset: 5\n" +
			"          Length: 6\n" +
			"          ReplacementText: 'const auto& x'\n"
		restore := stubTidy(t, export, map[string]string{src: "for (const auto& x : items) {}\n"}, nil)
		defer restore()
		_, stderr, _ := runCLI("-fix", "-level", level, src)
		return stderr
	}
	if s := run("2"); !strings.Contains(s, "can change behavior") {
		t.Errorf("-fix -level 2 should print the review reminder; stderr:\n%s", s)
	}
	if s := run("1"); strings.Contains(s, "can change behavior") {
		t.Errorf("-fix -level 1 must NOT print the review reminder; stderr:\n%s", s)
	}
}

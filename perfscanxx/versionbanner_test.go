package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/tidy"
)

// TestVersionReportsClangTidyBackend pins that `perfscanxx -version` prints not
// only its own version but the resolved clang-tidy backend (path + LLVM major) —
// the single biggest variable in perfscanxx's behavior, so a bug report or a "why
// did it find nothing" is self-diagnosing. Hermetic: clang-tidy is injected.
func TestVersionReportsClangTidyBackend(t *testing.T) {
	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/opt/homebrew/opt/llvm/bin/clang-tidy", nil }
	tidy.Executor = func(_ context.Context, argv []string, stdout, _ *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("Homebrew LLVM version 22.1.8\n")
			return 0, nil
		}
		return 0, nil
	}

	stdout, _, code := runCLI("-version")
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	for _, want := range []string{"perfscanxx", "clang-tidy:", "/opt/homebrew/opt/llvm/bin/clang-tidy", "LLVM 22"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("-version output missing %q:\n%s", want, stdout)
		}
	}
}

// TestVersionWhenClangTidyMissing pins that -version still succeeds and says so
// (with an install hint) when no clang-tidy is on PATH — the banner must never
// fail just because the backend is absent.
func TestVersionWhenClangTidyMissing(t *testing.T) {
	origLook := tidy.LookPath
	defer func() { tidy.LookPath = origLook }()
	tidy.LookPath = func(string) (string, error) { return "", context.Canceled } // any error => not found

	stdout, _, code := runCLI("-version")
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if !strings.Contains(stdout, "perfscanxx") {
		t.Errorf("-version must still print the tool version:\n%s", stdout)
	}
	if !strings.Contains(stdout, "not found in PATH") {
		t.Errorf("-version should report clang-tidy is absent with an install hint:\n%s", stdout)
	}
}

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/tidy"
)

// writeCompileDB drops a minimal one-TU compile_commands.json + source into dir
// and returns dir, so the CLI has something to enumerate.
func writeCompileDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cpp := filepath.Join(dir, "t.cpp")
	if err := os.WriteFile(cpp, []byte("int main(){return 0;}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"clang++ -std=c++17 -c t.cpp"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func argvHasExperimental(argv []string) bool {
	for _, a := range argv {
		if a == "--experimental-custom-checks" {
			return true
		}
	}
	return false
}

// TestExperimentalRejectionDegradesToBuiltins pins the graceful-degradation path
// for a clang-tidy whose --version we CANNOT parse (so the numeric >= 20 gate does
// not pre-empt it) but which is actually too old for --experimental-custom-checks:
// it rejects that flag, aborts WITHOUT analyzing, exits non-zero and writes no
// fixes. Left unhandled the empty payload reads as "clean". perfscanxx must instead
// detect the rejection, drop the custom checks, and re-run the built-ins — so a
// mixed selection (one custom + one built-in) still produces a real analysis.
func TestExperimentalRejectionDegradesToBuiltins(t *testing.T) {
	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }

	var analysisArgvs [][]string
	tidy.Executor = func(_ context.Context, argv []string, stdout, stderr *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			// Unparseable: no "LLVM version N" -> MajorVersion ok=false, gate misses it.
			stdout.WriteString("acme clang-tidy build 7\n")
			return 0, nil
		}
		analysisArgvs = append(analysisArgvs, argv)
		if argvHasExperimental(argv) {
			// Old clang-tidy: reject the flag, abort, write no export file.
			stderr.WriteString("error: clang-tidy: Unknown command line argument '--experimental-custom-checks'.\n")
			return 1, nil
		}
		// The degraded re-run: succeeds, no diagnostics.
		for _, a := range argv {
			if strings.HasPrefix(a, "--export-fixes=") {
				_ = os.WriteFile(strings.TrimPrefix(a, "--export-fixes="), []byte(""), 0o644)
			}
		}
		return 0, nil
	}

	dir := writeCompileDB(t)
	// One custom (PX2101) + one built-in (PX1001): after dropping custom, PX1001
	// remains, so the re-run has something to do.
	_, stderr, code := runCLI("-checks", "PX2101,PX1001", "-p", dir, dir)

	if code != 0 {
		t.Fatalf("exit %d, want 0 (degraded run succeeds); stderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "re-running with the built-in checks only") {
		t.Errorf("expected the degrade warning; stderr:\n%s", stderr)
	}
	if len(analysisArgvs) != 2 {
		t.Fatalf("expected 2 analysis runs (rejected + degraded retry), got %d:\n%v", len(analysisArgvs), analysisArgvs)
	}
	if !argvHasExperimental(analysisArgvs[0]) {
		t.Errorf("first analysis run should carry --experimental-custom-checks:\n%v", analysisArgvs[0])
	}
	if argvHasExperimental(analysisArgvs[1]) {
		t.Errorf("degraded re-run must NOT carry --experimental-custom-checks:\n%v", analysisArgvs[1])
	}
}

// TestExperimentalRejectionCustomOnlyErrors pins the boundary of the degrade path:
// when ONLY query-based custom checks were selected, dropping them leaves nothing
// to run, so perfscanxx must fail loudly with an actionable message rather than
// silently report a clean run.
func TestExperimentalRejectionCustomOnlyErrors(t *testing.T) {
	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }

	tidy.Executor = func(_ context.Context, argv []string, stdout, stderr *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("acme clang-tidy build 7\n") // unparseable
			return 0, nil
		}
		stderr.WriteString("error: clang-tidy: Unknown command line argument '--experimental-custom-checks'.\n")
		return 1, nil // reject, write no export
	}

	dir := writeCompileDB(t)
	_, stderr, code := runCLI("-checks", "PX2101", "-p", dir, dir)
	if code != 2 {
		t.Fatalf("exit %d, want 2 (nothing left to run); stderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "does not support --experimental-custom-checks") {
		t.Errorf("expected the custom-only failure message; stderr:\n%s", stderr)
	}
}

// TestFailedAnalysisIsNotReportedClean pins the loud guard independent of the
// experimental path: when clang-tidy exits non-zero AND writes no results (a bad
// -p, an unreadable file, a fatal toolchain error), perfscanxx must NOT fall
// through to the report path — an empty payload there reads as "no findings",
// misreporting a FAILED run as a clean one. It must fail with exit 2 instead.
func TestFailedAnalysisIsNotReportedClean(t *testing.T) {
	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }

	tidy.Executor = func(_ context.Context, argv []string, stdout, stderr *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("Homebrew LLVM version 22.1.8\n") // modern; not the experimental case
			return 0, nil
		}
		stderr.WriteString("error: some fatal toolchain failure\n")
		return 1, nil // fail, write no export
	}

	dir := writeCompileDB(t)
	// Built-in only (PX1001): no experimental flag, so this exercises the generic guard.
	_, stderr, code := runCLI("-checks", "PX1001", "-p", dir, dir)
	if code != 2 {
		t.Fatalf("exit %d, want 2 (failed analysis must not read as clean); stderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "did not run to completion") {
		t.Errorf("expected the failed-analysis message; stderr:\n%s", stderr)
	}
}

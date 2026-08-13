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

// TestOldClangTidySkipsCustomChecks pins graceful degradation: on a clang-tidy
// older than LLVM 20 (no --experimental-custom-checks), a default run — which
// selects the query-based custom checks (PX2101-PX2106) — must NOT pass that
// flag (which the old clang-tidy would reject and abort on), and must warn once
// while still running the built-in checks. Hermetic: clang-tidy is injected,
// reporting LLVM 18 for --version.
func TestOldClangTidySkipsCustomChecks(t *testing.T) {
	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }

	var runArgv []string
	tidy.Executor = func(_ context.Context, argv []string, stdout, _ *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("Ubuntu LLVM version 18.1.3\n")
			return 0, nil
		}
		runArgv = argv
		for _, a := range argv {
			if strings.HasPrefix(a, "--export-fixes=") {
				_ = os.WriteFile(strings.TrimPrefix(a, "--export-fixes="), []byte(""), 0o644)
			}
		}
		return 0, nil
	}

	dir := t.TempDir()
	cpp := filepath.Join(dir, "t.cpp")
	if err := os.WriteFile(cpp, []byte("int main(){return 0;}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"clang++ -std=c++17 -c t.cpp"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI("-p", dir, dir)
	if code != 0 {
		t.Errorf("exit %d, want 0 (built-ins still run); stderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "skipping the query-based custom checks") {
		t.Errorf("expected the graceful-skip warning; stderr:\n%s", stderr)
	}
	if len(runArgv) == 0 {
		t.Fatal("clang-tidy was never invoked for the analysis run")
	}
	for _, a := range runArgv {
		if a == "--experimental-custom-checks" {
			t.Errorf("--experimental-custom-checks was passed to an LLVM 18 clang-tidy (would abort); argv:\n%v", runArgv)
		}
	}
}

// TestFixErrorsRequiresFix pins that -fix-errors without -fix is rejected with a
// clear message (it has no meaning on its own).
func TestFixErrorsRequiresFix(t *testing.T) {
	dir := t.TempDir()
	cpp := filepath.Join(dir, "t.cpp")
	_ = os.WriteFile(cpp, []byte("int main(){return 0;}\n"), 0o644)
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"clang++ -std=c++17 -c t.cpp"}]`
	_ = os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644)
	_, stderr, code := runCLI("-fix-errors", "-p", dir, dir)
	if code != 2 || !strings.Contains(stderr, "-fix-errors has no effect without -fix") {
		t.Errorf("expected exit 2 + require-fix message; code=%d stderr=%q", code, stderr)
	}
}

// TestExperimentalFlagGatedByCustomSelection is the hermetic (clang-tidy-free)
// complement to TestOldClangTidySkipsCustomChecks and the empirical
// TestCustomChecksFireOnTargetPattern: on a NEW-enough clang-tidy, selecting a
// query-based custom check must pass --experimental-custom-checks (else the
// custom check never runs — the contract corpus runs like Catch2's PX2101/PX2107
// rely on), while a builtin-only selection must NOT pass it (it is only needed
// for custom checks and adds nothing for built-ins).
func TestExperimentalFlagGatedByCustomSelection(t *testing.T) {
	run := func(checks string) []string {
		origLook, origExec := tidy.LookPath, tidy.Executor
		defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
		tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
		var runArgv []string
		tidy.Executor = func(_ context.Context, argv []string, stdout, _ *bytes.Buffer) (int, error) {
			if len(argv) >= 2 && argv[1] == "--version" {
				stdout.WriteString("Homebrew LLVM version 22.1.8\n") // new enough
				return 0, nil
			}
			runArgv = argv
			for _, a := range argv {
				if strings.HasPrefix(a, "--export-fixes=") {
					_ = os.WriteFile(strings.TrimPrefix(a, "--export-fixes="), []byte(""), 0o644)
				}
			}
			return 0, nil
		}
		dir := t.TempDir()
		cpp := filepath.Join(dir, "t.cpp")
		if err := os.WriteFile(cpp, []byte("int main(){return 0;}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"clang++ -std=c++17 -c t.cpp"}]`
		if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, code := runCLI("-checks", checks, "-p", dir, dir); code != 0 {
			t.Fatalf("checks=%q: exit %d, want 0", checks, code)
		}
		return runArgv
	}

	hasExperimental := func(argv []string) bool {
		for _, a := range argv {
			if a == "--experimental-custom-checks" {
				return true
			}
		}
		return false
	}

	// A custom check (PX2101) selected -> the flag must be present.
	if argv := run("PX2101"); !hasExperimental(argv) {
		t.Errorf("custom check PX2101 selected but --experimental-custom-checks missing:\n%v", argv)
	}
	// A builtin-only selection (PX1001) -> the flag must be absent.
	if argv := run("PX1001"); hasExperimental(argv) {
		t.Errorf("builtin-only PX1001 selected but --experimental-custom-checks was passed:\n%v", argv)
	}
}

// TestBoundaryClangTidyKeepsCustomChecks pins the INCLUSIVE boundary of the
// experimental-custom-checks version gate. The gate drops custom checks only when
// major < MinExperimentalMajor (20), so LLVM 20 — the FIRST version that supports
// --experimental-custom-checks — must KEEP them. TestOldClangTidySkipsCustomChecks
// covers the drop below the boundary (LLVM 18) and TestExperimentalFlagGatedBy...
// covers well above it (LLVM 22), but exactly-20 was untested: a regression to a
// `<= MinExperimentalMajor` gate would wrongly disable custom checks on the very
// version they debut, and both existing tests (18 still drops, 22 still keeps)
// would stay green.
func TestBoundaryClangTidyKeepsCustomChecks(t *testing.T) {
	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }

	var runArgv []string
	tidy.Executor = func(_ context.Context, argv []string, stdout, _ *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("LLVM version 20.1.0\n") // exactly the boundary
			return 0, nil
		}
		runArgv = argv
		for _, a := range argv {
			if strings.HasPrefix(a, "--export-fixes=") {
				_ = os.WriteFile(strings.TrimPrefix(a, "--export-fixes="), []byte(""), 0o644)
			}
		}
		return 0, nil
	}

	dir := t.TempDir()
	cpp := filepath.Join(dir, "t.cpp")
	if err := os.WriteFile(cpp, []byte("int main(){return 0;}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"clang++ -std=c++17 -c t.cpp"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI("-checks", "PX2101", "-p", dir, dir)
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr:\n%s", code, stderr)
	}
	if strings.Contains(stderr, "skipping the query-based custom checks") {
		t.Errorf("LLVM 20 is the FIRST supported version — custom checks must NOT be skipped; stderr:\n%s", stderr)
	}
	found := false
	for _, a := range runArgv {
		if a == "--experimental-custom-checks" {
			found = true
		}
	}
	if !found {
		t.Errorf("LLVM 20 must keep custom checks and pass --experimental-custom-checks; argv:\n%v", runArgv)
	}
}

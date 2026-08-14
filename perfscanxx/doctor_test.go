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

// TestDoctorReadyEnvironment pins that -doctor reports every requirement met and
// exits 0 when clang-tidy (modern LLVM) and a compile database with on-disk TUs
// are present. Hermetic: clang-tidy is injected.
func TestDoctorReadyEnvironment(t *testing.T) {
	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
	tidy.Executor = func(_ context.Context, argv []string, stdout, _ *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("Homebrew LLVM version 22.1.8\n")
			return 0, nil
		}
		return 0, nil
	}

	dir := t.TempDir()
	cpp := filepath.Join(dir, "a.cpp")
	if err := os.WriteFile(cpp, []byte("int x;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"clang++ -std=c++17 -c a.cpp"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, code := runCLI("-tidy", "clang-tidy", "-doctor", "-p", dir)
	if code != 0 {
		t.Fatalf("ready env: exit=%d, want 0; output:\n%s", code, out)
	}
	for _, want := range []string{"clang-tidy: /usr/bin/clang-tidy (LLVM 22)", "custom checks: supported", "compile database:", "1 TU(s), 1 on disk", "ready to scan."} {
		if !strings.Contains(out, want) {
			t.Errorf("-doctor output missing %q:\n%s", want, out)
		}
	}
}

// TestDoctorMissingRequirements pins the failure path: no clang-tidy and no
// compile database each get a ✗ line with remediation, and -doctor exits 1.
func TestDoctorMissingRequirements(t *testing.T) {
	origLook := tidy.LookPath
	defer func() { tidy.LookPath = origLook }()
	tidy.LookPath = func(string) (string, error) { return "", os.ErrNotExist } // clang-tidy absent

	// Run from an empty dir so no compile database is discoverable.
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	out, _, code := runCLI("-doctor")
	if code != 1 {
		t.Fatalf("missing env: exit=%d, want 1; output:\n%s", code, out)
	}
	for _, want := range []string{"✗ clang-tidy: not found", "brew install llvm", "✗ compile database: not found", "not ready"} {
		if !strings.Contains(out, want) {
			t.Errorf("-doctor output missing %q:\n%s", want, out)
		}
	}
}

// TestDoctorPartialStaleDatabase pins the middle case: a compile database listing
// some TUs that are absent on disk (files deleted/moved, or not-yet-generated) is
// a ⚠ (warning) — the scan still works on the present ones — not a ✗, and -doctor
// still exits 0.
func TestDoctorPartialStaleDatabase(t *testing.T) {
	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
	tidy.Executor = func(_ context.Context, argv []string, stdout, _ *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("LLVM version 22.0.0\n")
			return 0, nil
		}
		return 0, nil
	}

	dir := t.TempDir()
	present := filepath.Join(dir, "a.cpp")
	if err := os.WriteFile(present, []byte("int x;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ghost := filepath.Join(dir, "gone.cpp") // in the DB, never on disk
	cc := `[{"directory":"` + dir + `","file":"` + present + `","command":"clang++ -std=c++17 -c a.cpp"},` +
		`{"directory":"` + dir + `","file":"` + ghost + `","command":"clang++ -std=c++17 -c gone.cpp"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, code := runCLI("-tidy", "clang-tidy", "-doctor", "-p", dir)
	if code != 0 {
		t.Fatalf("partial-stale DB: exit=%d, want 0 (still scannable); output:\n%s", code, out)
	}
	for _, want := range []string{"⚠ compile database:", "2 TU(s), 1 on disk — 1 missing", "ready to scan."} {
		if !strings.Contains(out, want) {
			t.Errorf("-doctor output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "✗ compile database") {
		t.Errorf("a partially-stale DB must be ⚠, not ✗:\n%s", out)
	}
}

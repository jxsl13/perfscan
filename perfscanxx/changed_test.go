package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/tidy"
)

// changedProject writes N .cpp TUs + a compile db in a temp dir and returns dir,
// the abs TU paths, and a cleanup-installing stub for clang-tidy that records the
// .cpp files each analysis invocation received.
func changedProject(t *testing.T, names ...string) (dir string, files []string, scanned func() []string) {
	t.Helper()
	dir = t.TempDir()
	var entries []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("int x;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
		entries = append(entries, `{"directory":"`+dir+`","file":"`+p+`","command":"clang++ -std=c++17 -c `+n+`"}`)
	}
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte("["+strings.Join(entries, ",")+"]"), 0o644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var got []string
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
	tidy.Executor = func(_ context.Context, argv []string, stdout, _ *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("LLVM version 22.0.0\n")
			return 0, nil
		}
		for _, a := range argv {
			if strings.HasSuffix(a, ".cpp") {
				mu.Lock()
				got = append(got, filepath.Base(a))
				mu.Unlock()
			}
			if strings.HasPrefix(a, "--export-fixes=") {
				_ = os.WriteFile(strings.TrimPrefix(a, "--export-fixes="), nil, 0o644)
			}
		}
		return 0, nil
	}
	return dir, files, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), got...) }
}

// TestChangedScansOnlyChangedTUs pins the core -changed behavior: only TUs that
// differ from the ref are analyzed; the rest are skipped.
func TestChangedScansOnlyChangedTUs(t *testing.T) {
	origLook, origExec, origGit := tidy.LookPath, tidy.Executor, gitOutput
	defer func() { tidy.LookPath, tidy.Executor, gitOutput = origLook, origExec, origGit }()

	dir, files, scanned := changedProject(t, "a.cpp", "b.cpp", "c.cpp")
	gitOutput = func(_ context.Context, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "rev-parse" {
			return dir + "\n", nil
		}
		return "a.cpp\nc.cpp\n", nil // a.cpp and c.cpp changed; b.cpp did not
	}
	_, errOut, code := runCLI(append([]string{"-changed", "origin/main", "-checks", "PX3008", "-p", dir}, files...)...)
	if code != 0 && code != 1 {
		t.Fatalf("exit=%d; stderr:\n%s", code, errOut)
	}
	got := scanned()
	want := map[string]bool{"a.cpp": true, "c.cpp": true}
	for _, f := range got {
		if !want[f] {
			t.Errorf("scanned an UNCHANGED TU %s; -changed should skip it. scanned=%v", f, got)
		}
	}
	if len(got) != 2 {
		t.Errorf("scanned %v, want exactly a.cpp + c.cpp", got)
	}
	if !strings.Contains(errOut, "scanning 2 translation unit(s) changed vs origin/main") {
		t.Errorf("missing the -changed summary; stderr:\n%s", errOut)
	}
}

// TestChangedHeaderOnlyWarnsAndScansNothing pins that a changed HEADER (not a TU)
// is reported with the "run a full scan" warning and, with no changed TU, the run
// exits 0 without invoking clang-tidy for analysis.
func TestChangedHeaderOnlyWarnsAndScansNothing(t *testing.T) {
	origLook, origExec, origGit := tidy.LookPath, tidy.Executor, gitOutput
	defer func() { tidy.LookPath, tidy.Executor, gitOutput = origLook, origExec, origGit }()

	dir, files, scanned := changedProject(t, "a.cpp", "b.cpp")
	gitOutput = func(_ context.Context, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "rev-parse" {
			return dir + "\n", nil
		}
		return "include/util.h\n", nil // only a header changed
	}
	_, errOut, code := runCLI(append([]string{"-changed", "HEAD~1", "-checks", "PX3008", "-p", dir}, files...)...)
	if code != 0 {
		t.Fatalf("header-only change: exit=%d, want 0 (nothing to scan); stderr:\n%s", code, errOut)
	}
	if !strings.Contains(errOut, "1 changed header(s)") {
		t.Errorf("expected the changed-header warning; stderr:\n%s", errOut)
	}
	if !strings.Contains(errOut, "no changed translation units") {
		t.Errorf("expected the nothing-to-scan note; stderr:\n%s", errOut)
	}
	if s := scanned(); len(s) != 0 {
		t.Errorf("clang-tidy analyzed %v, but only a header changed", s)
	}
}

// TestChangedGitFailureFallsBackToFullScan pins robustness: a git error must NOT
// break the run — it falls back to scanning all TUs (a CI hiccup can't turn the
// lint into a no-op).
func TestChangedGitFailureFallsBackToFullScan(t *testing.T) {
	origLook, origExec, origGit := tidy.LookPath, tidy.Executor, gitOutput
	defer func() { tidy.LookPath, tidy.Executor, gitOutput = origLook, origExec, origGit }()

	dir, files, scanned := changedProject(t, "a.cpp", "b.cpp")
	gitOutput = func(_ context.Context, args ...string) (string, error) {
		return "", context.Canceled // simulate any git failure
	}
	_, errOut, _ := runCLI(append([]string{"-changed", "origin/main", "-checks", "PX3008", "-p", dir}, files...)...)
	if !strings.Contains(errOut, "falling back") && !strings.Contains(errOut, "scanning all translation units instead") {
		t.Errorf("git failure should warn about the fallback; stderr:\n%s", errOut)
	}
	if len(scanned()) != 2 {
		t.Errorf("git failure should full-scan both TUs, scanned=%v", scanned())
	}
}

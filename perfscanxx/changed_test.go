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

// TestChangedBaselineNoFalseStale pins the -changed x -baseline interaction: on an
// incremental scan, baseline entries for the UNSCANNED TUs are trivially unmatched,
// but that is NOT staleness — the run just didn't look at them. The "stale baseline
// entries" nudge (which only makes sense on a full scan) must be SUPPRESSED under
// -changed, or every incremental CI run would cry wolf.
func TestChangedBaselineNoFalseStale(t *testing.T) {
	origLook, origExec, origGit := tidy.LookPath, tidy.Executor, gitOutput
	defer func() { tidy.LookPath, tidy.Executor, gitOutput = origLook, origExec, origGit }()

	dir := t.TempDir()
	var files, entries []string
	for _, n := range []string{"a.cpp", "b.cpp"} {
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
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
	// Emit one PX3008 finding for whichever .cpp each invocation analyzes.
	tidy.Executor = func(_ context.Context, argv []string, stdout, _ *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("LLVM version 22.0.0\n")
			return 0, nil
		}
		var cpp, export string
		for _, a := range argv {
			if strings.HasSuffix(a, ".cpp") {
				cpp = a
			}
			if strings.HasPrefix(a, "--export-fixes=") {
				export = strings.TrimPrefix(a, "--export-fixes=")
			}
		}
		if export != "" && cpp != "" {
			y := "MainSourceFile: '" + cpp + "'\nDiagnostics:\n" +
				"  - DiagnosticName: readability-container-size-empty\n" +
				"    DiagnosticMessage:\n      Message: 'm'\n      FilePath: '" + cpp + "'\n      FileOffset: 0\n      Replacements: []\n"
			_ = os.WriteFile(export, []byte(y), 0o644)
		}
		return 0, nil
	}

	bl := filepath.Join(dir, "bl.yaml")
	// Seed the baseline from a FULL scan (findings for a.cpp AND b.cpp).
	if _, _, code := runCLI(append([]string{"-tidy", "clang-tidy", "-checks", "PX3008", "-baseline", bl, "-p", dir}, files...)...); code != 0 {
		t.Fatalf("seeding baseline: exit=%d, want 0", code)
	}

	// Incremental run: only a.cpp changed -> b.cpp's baseline entry is unmatched,
	// but -changed must NOT report it as stale.
	gitOutput = func(_ context.Context, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "rev-parse" {
			return dir + "\n", nil
		}
		return "a.cpp\n", nil
	}
	_, errOut, _ := runCLI(append([]string{"-tidy", "clang-tidy", "-changed", "origin/main", "-checks", "PX3008", "-baseline", bl, "-p", dir}, files...)...)
	if strings.Contains(errOut, "stale baseline") {
		t.Errorf("-changed run falsely reported a stale baseline entry (b.cpp was just not scanned):\n%s", errOut)
	}
}

// TestChangedSARIFNotAuthoritative pins the -changed x -sarif x GitHub interaction:
// a -changed run is a PARTIAL scan, so its SARIF must set
// invocations[].executionSuccessful=false. Otherwise GitHub Code Scanning would
// treat the run as authoritative and CLOSE alerts in files the incremental scan
// never analyzed — silently resolving real issues. A full scan stays true.
func TestChangedSARIFNotAuthoritative(t *testing.T) {
	origLook, origExec, origGit := tidy.LookPath, tidy.Executor, gitOutput
	defer func() { tidy.LookPath, tidy.Executor, gitOutput = origLook, origExec, origGit }()

	dir, files, _ := changedProject(t, "a.cpp", "b.cpp")
	gitOutput = func(_ context.Context, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "rev-parse" {
			return dir + "\n", nil
		}
		return "a.cpp\n", nil
	}
	execSuccessful := func(sarif string) bool {
		// crude but sufficient: the field appears once in our single-run output.
		i := strings.Index(sarif, `"executionSuccessful"`)
		if i < 0 {
			t.Fatalf("SARIF missing executionSuccessful:\n%s", sarif)
		}
		return strings.Contains(sarif[i:i+40], "true")
	}

	// -changed -> partial -> executionSuccessful=false.
	out, _, _ := runCLI(append([]string{"-tidy", "clang-tidy", "-changed", "origin/main", "-checks", "PX3008", "-sarif", "-p", dir}, files...)...)
	if execSuccessful(out) {
		t.Errorf("-changed -sarif: executionSuccessful must be false (partial scan must not authoritatively close alerts):\n%s", out)
	}

	// Full scan (no -changed) -> executionSuccessful=true.
	full, _, _ := runCLI(append([]string{"-tidy", "clang-tidy", "-checks", "PX3008", "-sarif", "-p", dir}, files...)...)
	if !execSuccessful(full) {
		t.Errorf("full -sarif run: executionSuccessful should be true:\n%s", full)
	}
}

// TestChangedFixNarrowsToChangedTU pins that -changed -fix hands clang-tidy ONLY
// the changed translation units — so an incremental fix never rewrites a file the
// run didn't scan. Hermetic (stubbed git + clang-tidy): the earlier real-git-repo
// version corrupted the worktree perfscanxx itself lives in, so this verifies the
// same property safely by capturing which files the (fix) invocation received.
func TestChangedFixNarrowsToChangedTU(t *testing.T) {
	origLook, origExec, origGit := tidy.LookPath, tidy.Executor, gitOutput
	defer func() { tidy.LookPath, tidy.Executor, gitOutput = origLook, origExec, origGit }()

	dir, files, scanned := changedProject(t, "a.cpp", "b.cpp")
	gitOutput = func(_ context.Context, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "rev-parse" {
			return dir + "\n", nil
		}
		return "a.cpp\n", nil // only a.cpp changed
	}
	// Capture whether the analysis invocation carried --fix (the fix pass).
	var mu sync.Mutex
	sawFix := false
	base := tidy.Executor
	tidy.Executor = func(ctx context.Context, argv []string, out, errb *bytes.Buffer) (int, error) {
		for _, a := range argv {
			if a == "--fix" {
				mu.Lock()
				sawFix = true
				mu.Unlock()
			}
		}
		return base(ctx, argv, out, errb)
	}

	runCLI(append([]string{"-tidy", "clang-tidy", "-changed", "origin/main", "-fix", "-checks", "PX3008", "-p", dir}, files...)...)

	got := scanned()
	if len(got) != 1 || got[0] != "a.cpp" {
		t.Errorf("-changed -fix analyzed %v, want only a.cpp (must not touch unchanged TUs)", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if !sawFix {
		t.Error("expected the analysis pass to carry --fix under -fix")
	}
}

// TestChangedRejectsFlagLookingRef pins the guard for a common footgun: writing
// `-changed -fix` (ref omitted) makes Go's flag parser swallow -fix as the ref
// value. Since git refs never start with "-", perfscanxx rejects it up front with
// a clear message instead of running a full scan that silently drops the intended
// flag. Hermetic — the guard fires at flag validation, before any git/clang-tidy.
func TestChangedRejectsFlagLookingRef(t *testing.T) {
	_, errOut, code := runCLI("-changed", "-fix")
	if code != 2 {
		t.Fatalf("exit=%d, want 2; stderr:\n%s", code, errOut)
	}
	if !strings.Contains(errOut, "looks like a flag") {
		t.Errorf("expected the flag-looking-ref error; stderr:\n%s", errOut)
	}
}

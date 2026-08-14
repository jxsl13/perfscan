package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestFixAppliesPX3023 pins that PX3023 (modernize-shrink-to-fit) actually
// applies its clang-tidy fix-it end-to-end: the copy-and-swap capacity-shed
// idiom std::vector<T>(v).swap(v) is rewritten to v.shrink_to_fit(). PX3023 was
// added to the catalog with HasFix:true on the strength of a manual
// `clang-tidy --fix` run; this makes that a standing regression (the catalog's
// "HasFix true only if a fix-it applies" discipline).
//
// The fixture needs the C++ standard library <vector>, so on darwin it
// discovers the SDK sysroot via xcrun. If the toolchain cannot parse <vector>
// (a stripped CI image without libc++ headers) the test skips rather than
// fails, matching the repo's setup-difference idiom. Skipped when clang-tidy
// is absent.
func TestFixAppliesPX3023(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	dir := t.TempDir()
	cpp := filepath.Join(dir, "s.cpp")
	const src = "#include <vector>\n" +
		"void f(std::vector<int>& v) {\n" +
		"  std::vector<int>(v).swap(v);\n" +
		"}\n"
	if err := os.WriteFile(cpp, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	compile := "clang++ -std=c++17"
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("xcrun", "--show-sdk-path").Output()
		if err != nil {
			t.Skipf("xcrun --show-sdk-path failed (%v); cannot locate the C++ sysroot", err)
		}
		compile += " -isysroot " + strings.TrimSpace(string(out))
	}
	compile += " -c s.cpp"
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"` + compile + `"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, _ := runCLI("-tidy", bin, "-fix", "-checks", "PX3023", "-p", dir, cpp)

	got, err := os.ReadFile(cpp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "shrink_to_fit") {
		if strings.Contains(string(got), ".swap(v)") {
			t.Errorf("PX3023 left the copy-and-swap idiom in place alongside the fix:\n%s", got)
		}
		return // fix applied cleanly — success
	}

	// Not applied. Distinguish a genuine regression from an environment that
	// cannot parse <vector> (no libc++ headers): clang-tidy prints a fatal
	// "file not found" / "fatal error" for the missing header, and applies no
	// fix-it when the TU does not parse.
	if strings.Contains(stderr, "file not found") || strings.Contains(stderr, "fatal error") {
		t.Skipf("toolchain could not parse <vector>; skipping PX3023 apply check. stderr:\n%s", stderr)
	}
	t.Errorf("PX3023 -fix did not rewrite std::vector<int>(v).swap(v) to shrink_to_fit():\n%s", got)
}

// TestFixWithAdvisoryCustomCheckReportsButAppliesNothing pins the -fix behavior
// when the ONLY selected check is a query-based custom check (advisory,
// HasFix:false): custom checks carry no clang-tidy fix-it, so -fix must still
// REPORT the finding, leave the source byte-for-byte, print the "no fix-it" note,
// and exit 0 (fix mode completed — nothing to apply). This is the custom-check
// analog of the built-in fix tests and closes the untested advisory-under-fix
// path — a regression that dropped advisory findings under -fix, or that let a
// no-op fix run mutate/damage the file, would slip through otherwise. Uses
// PX2108 (custom-vector-bool).
func TestFixWithAdvisoryCustomCheckReportsButAppliesNothing(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	dir := t.TempDir()
	cpp := filepath.Join(dir, "v.cpp")
	const src = "#include <vector>\nstruct S { std::vector<bool> flags; };\n"
	if err := os.WriteFile(cpp, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	compile := "clang++ -std=c++17"
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("xcrun", "--show-sdk-path").Output()
		if err != nil {
			t.Skipf("xcrun --show-sdk-path failed (%v); cannot locate the C++ sysroot", err)
		}
		compile += " -isysroot " + strings.TrimSpace(string(out))
	}
	compile += " -c v.cpp"
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"` + compile + `"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI("-tidy", bin, "-fix", "-checks", "PX2108", "-p", dir, cpp)
	if strings.Contains(stderr, "file not found") || strings.Contains(stderr, "fatal error") {
		t.Skipf("toolchain could not parse <vector>; skipping. stderr:\n%s", stderr)
	}

	// The advisory finding must still be reported under -fix.
	if !strings.Contains(stdout, "PX2108") {
		t.Errorf("-fix with an advisory custom check must still REPORT the finding on stdout:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	// And the note that nothing was fixable must be printed.
	if !strings.Contains(stderr, "no reported finding carries a fix-it") {
		t.Errorf("-fix with only an advisory check must print the 'no fix-it' note on stderr:\n%s", stderr)
	}
	// The source must be byte-for-byte unchanged: a custom check has no fix-it,
	// so the no-op fix run must not touch the file.
	got, err := os.ReadFile(cpp)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != src {
		t.Errorf("-fix mutated the file for an advisory-only check; it must stay byte-identical:\n%s", got)
	}
	// Fix mode completed with nothing to apply: exit 0 per the -fix contract.
	if code != 0 {
		t.Errorf("exit code = %d, want 0 (-fix completed; nothing to apply)", code)
	}
}

// TestDiffWithAdvisoryCustomCheckShowsNoChange is the -diff analog of the -fix
// advisory pin: -diff previews what -fix would change and exits 1 iff anything
// WOULD change. With only an advisory (HasFix:false) custom check selected there
// is no fix-it, so -diff must produce no diff, print "no fixes to apply", leave
// the file byte-for-byte, and exit 0 — NOT 1. This locks the subtle contract that
// -diff's exit code keys on "would change", not on "found a finding": an advisory
// finding exists here yet the correct diff-mode exit is 0.
func TestDiffWithAdvisoryCustomCheckShowsNoChange(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	dir := t.TempDir()
	cpp := filepath.Join(dir, "v.cpp")
	const src = "#include <vector>\nstruct S { std::vector<bool> flags; };\n"
	if err := os.WriteFile(cpp, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	compile := "clang++ -std=c++17"
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("xcrun", "--show-sdk-path").Output()
		if err != nil {
			t.Skipf("xcrun --show-sdk-path failed (%v); cannot locate the C++ sysroot", err)
		}
		compile += " -isysroot " + strings.TrimSpace(string(out))
	}
	compile += " -c v.cpp"
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"` + compile + `"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI("-tidy", bin, "-diff", "-checks", "PX2108", "-p", dir, cpp)
	if strings.Contains(stderr, "file not found") || strings.Contains(stderr, "fatal error") {
		t.Skipf("toolchain could not parse <vector>; skipping. stderr:\n%s", stderr)
	}

	// No diff body was emitted (nothing would change).
	if !strings.Contains(stderr, "no fixes to apply") {
		t.Errorf("-diff with only an advisory check should report 'no fixes to apply':\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if strings.Contains(stdout, "@@") || strings.Contains(stdout, "---") {
		t.Errorf("-diff emitted a hunk for an advisory-only check; there is nothing to change:\n%s", stdout)
	}
	// File untouched — -diff never writes.
	got, err := os.ReadFile(cpp)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != src {
		t.Errorf("-diff mutated the file; it must never write:\n%s", got)
	}
	// Nothing would change -> exit 0, even though an advisory finding exists.
	if code != 0 {
		t.Errorf("exit code = %d, want 0 (-diff exits 1 only when something WOULD change)", code)
	}
}

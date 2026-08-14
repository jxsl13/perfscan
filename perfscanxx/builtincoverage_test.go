package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// builtinNonObviousSrc exhibits two anti-patterns whose coverage by an EXISTING
// built-in check is NOT obvious from the check's name/title, and which were each
// (re)confirmed as already-covered during a custom-check audit — so perfscanxx
// deliberately ships NO custom check for them:
//
//   - a STRUCTURED-BINDING range-for copy `for (auto [k, v] : m)` copies each
//     element — caught by performance-for-range-copy (PX1001), not just the plain
//     `for (auto x : v)` form.
//   - `emplace_back(T(args))` builds a redundant temporary that defeats emplace —
//     caught by modernize-use-emplace (PX2003), which handles this direction as
//     well as `push_back(T(args))`.
//
// The two functions below are crafted so each check has EXACTLY ONE trigger, so a
// missing tag means that specific non-obvious form regressed upstream.
const builtinNonObviousSrc = `#include <map>
#include <vector>
#include <string>
struct Big { std::string a, b, c, d; };
void structuredBindingCopy(const std::map<int, Big>& m) {
  for (auto [k, v] : m) {              // performance-for-range-copy: copies Big each iter
    (void)k; (void)v.a;
  }
}
void emplaceTemporary(std::vector<std::string>& v) {
  v.emplace_back(std::string("xxxx")); // modernize-use-emplace: redundant temporary
}
`

// TestBuiltinsCoverNonObviousForms pins that two existing built-in checks cover
// anti-patterns that are NOT obvious from their titles, so (1) a future audit does
// not add a redundant custom check for them (this was nearly done twice — the
// checks turned out already-covered), and (2) an upstream clang-tidy change that
// drops the coverage is caught here rather than silently narrowing perfscanxx.
// Skips when clang-tidy is unavailable (common in CI), like the other empirical tests.
func TestBuiltinsCoverNonObviousForms(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "t.cpp")
	if err := os.WriteFile(src, []byte(builtinNonObviousSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	args := []string{src, "--checks=-*,performance-for-range-copy,modernize-use-emplace", "--", "-std=c++17"}
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("xcrun", "--show-sdk-path").Output()
		if err != nil {
			t.Skip("xcrun --show-sdk-path failed; cannot locate the C++ sysroot")
		}
		args = append(args, "-isysroot", strings.TrimSpace(string(out)))
	}
	out, _ := exec.Command(bin, args...).CombinedOutput()
	output := string(out)
	if strings.Contains(output, "file not found") || strings.Contains(output, "fatal error:") {
		t.Skipf("toolchain could not parse the fixture headers; skipping:\n%s", output)
	}

	cases := []struct {
		what, tag string
	}{
		{"structured-binding range-for copy (PX1001)", "[performance-for-range-copy]"},
		{"emplace_back(T(...)) redundant temporary (PX2003)", "[modernize-use-emplace]"},
	}
	for _, c := range cases {
		if !strings.Contains(output, c.tag) {
			t.Errorf("%s produced NO diagnostic — the non-obvious coverage may have regressed upstream, or the fixture drifted:\n%s", c.what, output)
		}
	}
}

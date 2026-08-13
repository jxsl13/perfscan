package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/catalog"
)

// advisoryTriggerSrc exhibits every advisory (report-only, HasFix:false)
// BUILT-IN check's anti-pattern in one translation unit:
//
//	PX2002 inefficient-string-concatenation, PX3020 rvalue-reference-param-not-moved,
//	PX3021 no-int-to-ptr, PX3022 enum-size, PX3024 implicit-conversion-in-loop,
//	PX3025 no-automatic-move.
//
// Unlike the custom checks, these are upstream clang-tidy checks with no
// fix-it, so they are driven by --checks (not --experimental-custom-checks).
const advisoryTriggerSrc = `#include <string>
#include <vector>
#include <cstdint>

// performance-inefficient-string-concatenation: reallocates every iteration.
std::string advConcatLoop(const std::vector<std::string>& parts) {
  std::string result;
  for (const auto& p : parts) {
    result = result + p;
  }
  return result;
}

// cppcoreguidelines-rvalue-reference-param-not-moved: s is never std::move'd.
void advTakesRvalue(std::string&& s) {
  std::string local = s;
  (void)local;
}

// performance-no-int-to-ptr: an integer round-tripped through a pointer.
int* advIntToPtr(std::uintptr_t x) {
  return reinterpret_cast<int*>(x);
}

// performance-enum-size: underlying type far wider than the value set needs.
enum class AdvWideEnum : long long { A = 1, B = 2 };
AdvWideEnum advWideEnumUser() { return AdvWideEnum::A; }

// performance-implicit-conversion-in-loop: int element -> long temporary/iter.
long advSumConverted(const std::vector<int>& v) {
  long total = 0;
  for (const long& x : v) {
    total += x;
  }
  return total;
}

// performance-no-automatic-move: a const value parameter that is returned
// can't be moved -> forced copy.
std::string advConstParamReturn(const std::string s) {
  return s;
}
`

// TestAdvisoryChecksFireOnTargetPattern verifies each advisory (HasFix:false,
// non-custom) built-in check actually PRODUCES a diagnostic on code exhibiting
// its anti-pattern. It is the advisory-built-in analog of
// TestCustomChecksFireOnTargetPattern (custom queries) and
// TestHasFixFixtureCompleteness / the HasFix-apply tests (fixable built-ins):
// together they guarantee EVERY catalog entry is behaviorally exercised, so a
// typo'd TidyName, or a clang-tidy release that renames/drops a check, is
// caught rather than silently surfacing nothing. Self-maintaining: a new
// advisory built-in with no trigger here fails the test.
func TestAdvisoryChecksFireOnTargetPattern(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}

	var advisory []catalog.Entry
	for _, e := range catalog.All() {
		if !e.Custom && !e.HasFix {
			advisory = append(advisory, e)
		}
	}
	if len(advisory) == 0 {
		t.Fatal("no advisory built-in checks in catalog to validate")
	}

	names := make([]string, 0, len(advisory))
	for _, e := range advisory {
		names = append(names, e.TidyName)
	}
	checks := "-*," + strings.Join(names, ",")

	dir := t.TempDir()
	src := filepath.Join(dir, "t.cpp")
	if err := os.WriteFile(src, []byte(advisoryTriggerSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	args := []string{src, "--checks=" + checks, "--", "-std=c++17"}
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("xcrun", "--show-sdk-path").Output(); err == nil {
			args = append(args, "-isysroot", strings.TrimSpace(string(out)))
		} else {
			t.Skip("xcrun --show-sdk-path failed; cannot locate the C++ sysroot")
		}
	}
	out, _ := exec.Command(bin, args...).CombinedOutput()
	output := string(out)

	// If the standard headers cannot be parsed, no check can fire — skip rather
	// than fail (stripped CI image without libc++).
	if strings.Contains(output, "file not found") || strings.Contains(output, "fatal error:") {
		t.Skipf("toolchain could not parse the fixture headers; skipping:\n%s", output)
	}

	// Each advisory built-in must have fired, attributed as [<tidy-name>].
	for _, e := range advisory {
		tag := "[" + e.TidyName + "]"
		if !strings.Contains(output, tag) {
			t.Errorf("%s (%s) produced NO diagnostic on code exhibiting its anti-pattern — the catalog TidyName may be wrong or the check was renamed/dropped upstream:\n%s", e.ID, e.TidyName, output)
		}
	}
}

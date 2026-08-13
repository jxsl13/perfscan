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

// customTriggerSrc exhibits ALL query-based custom-check anti-patterns in one
// main-file translation unit (the checks carry isExpansionInMainFile guards):
//
//	PX2101 reserve-before-loop, PX2102 pessimizing-move, PX2103 catch-by-value,
//	PX2104 regex-in-loop, PX2105 dynamic-cast-in-loop, PX2106 stringstream-in-loop,
//	PX2107 pow-const-exponent.
const customTriggerSrc = `#include <vector>
#include <regex>
#include <sstream>
#include <exception>
#include <cmath>
struct B { virtual ~B(){} }; struct D : B {};
std::vector<int> pessimizing() {
  std::vector<int> v;
  return std::move(v); // PX2102
}
double g(double x) { return std::pow(x, 2); } // PX2107 (constant exponent)
float gf(float x) { return powf(x, 2.0f); }    // PX2107 (powf variant)
void f(B* p) {
  std::vector<int> grow;
  for (int i = 0; i < 10; ++i) {
    grow.push_back(i);          // PX2101
    std::regex re("x");         // PX2104
    (void)dynamic_cast<D*>(p);  // PX2105
    std::ostringstream ss;      // PX2106
    (void)re; (void)ss;
  }
  try {
  } catch (std::exception e) { (void)e; } // PX2103
}
`

// TestCustomChecksFireOnTargetPattern verifies each query-based custom check
// actually PRODUCES a diagnostic on code that exhibits its anti-pattern — the
// gap TestCustomQueriesAcceptedByClangTidy (which runs on a trivial `int main`
// TU) cannot close: a matcher can be syntactically accepted yet semantically
// match nothing (a wrong-but-valid matcher would fire on no real code). This is
// the custom-check analog of TestHasFixChecksActuallyApply for built-ins.
func TestCustomChecksFireOnTargetPattern(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}

	var custom []catalog.Entry
	for _, e := range catalog.All() {
		if e.Custom {
			custom = append(custom, e)
		}
	}
	if len(custom) == 0 {
		t.Fatal("no custom checks in catalog to validate")
	}
	cfg := catalog.ClangTidyConfig(custom)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".clang-tidy")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "t.cpp")
	if err := os.WriteFile(src, []byte(customTriggerSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	args := []string{src, "--experimental-custom-checks", "--config-file=" + cfgPath, "--", "-std=c++17"}
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("xcrun", "--show-sdk-path").Output(); err == nil {
			args = append(args, "-isysroot", strings.TrimSpace(string(out)))
		} else {
			t.Skip("xcrun --show-sdk-path failed; cannot locate the C++ sysroot")
		}
	}
	out, _ := exec.Command(bin, args...).CombinedOutput()
	output := string(out)

	if strings.Contains(output, "Unknown command line argument") && strings.Contains(output, "experimental-custom-checks") {
		t.Skip("clang-tidy is too old for --experimental-custom-checks; skipping")
	}
	if strings.Contains(output, "[clang-tidy-config]") {
		t.Fatalf("clang-tidy rejected a custom-check query:\n%s", output)
	}
	// If the standard headers cannot be parsed, no check can fire — skip rather
	// than fail (stripped CI image without libc++).
	if strings.Contains(output, "file not found") || strings.Contains(output, "fatal error:") {
		t.Skipf("toolchain could not parse the fixture headers; skipping:\n%s", output)
	}

	// Each custom check must have fired, attributed as [custom-<name>].
	for _, e := range custom {
		tag := "[" + e.TidyName + "]"
		if !strings.Contains(output, tag) {
			t.Errorf("%s (%s) produced NO diagnostic on code exhibiting its anti-pattern — the query matches nothing:\n%s", e.ID, e.TidyName, output)
		}
	}
}

// px2101LoopKindsSrc grows a vector via push_back in a RANGE-FOR and in a WHILE
// loop — NOT the counted `for` that TestCustomChecksFireOnTargetPattern uses.
// PX2101's matcher was once forStmt()-only and MISSED range-for (a false
// negative fixed by broadening to anyOf(forStmt, cxxForRangeStmt, whileStmt,
// doStmt)); leveldb corpus validation showed it firing across loop kinds. This
// pins that breadth so a regression to a narrower matcher is caught.
const px2101LoopKindsSrc = `#include <vector>
void rangeFor(const std::vector<int>& in) {
  std::vector<int> out;
  for (int x : in) {        // cxxForRangeStmt
    out.push_back(x);       // PX2101
  }
  (void)out;
}
void whileLoop(int n) {
  std::vector<int> out;
  int i = 0;
  while (i < n) {           // whileStmt
    out.push_back(i);       // PX2101
    ++i;
  }
  (void)out;
}
`

// TestPX2101FiresAcrossLoopKinds pins that the reserve-before-loop custom check
// fires on a range-for AND a while loop, not just a counted for. It expects at
// least two PX2101 diagnostics (one per loop). Guards the loop-kind breadth of
// the query matcher (anyOf(forStmt, cxxForRangeStmt, whileStmt, doStmt)).
func TestPX2101FiresAcrossLoopKinds(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	e, ok := catalog.ByID("PX2101")
	if !ok || !e.Custom {
		t.Fatal("PX2101 missing or not a custom check")
	}
	cfg := catalog.ClangTidyConfig([]catalog.Entry{e})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".clang-tidy")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "loops.cpp")
	if err := os.WriteFile(src, []byte(px2101LoopKindsSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	args := []string{src, "--experimental-custom-checks", "--config-file=" + cfgPath, "--", "-std=c++17"}
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("xcrun", "--show-sdk-path").Output()
		if err != nil {
			t.Skip("xcrun --show-sdk-path failed; cannot locate the C++ sysroot")
		}
		args = append(args, "-isysroot", strings.TrimSpace(string(out)))
	}
	out, _ := exec.Command(bin, args...).CombinedOutput()
	output := string(out)

	if strings.Contains(output, "Unknown command line argument") && strings.Contains(output, "experimental-custom-checks") {
		t.Skip("clang-tidy is too old for --experimental-custom-checks; skipping")
	}
	if strings.Contains(output, "[clang-tidy-config]") {
		t.Fatalf("clang-tidy rejected the PX2101 query:\n%s", output)
	}
	if strings.Contains(output, "file not found") || strings.Contains(output, "fatal error:") {
		t.Skipf("toolchain could not parse the fixture headers; skipping:\n%s", output)
	}

	tag := "[" + e.TidyName + "]"
	if n := strings.Count(output, tag); n < 2 {
		t.Errorf("PX2101 fired %d time(s) on a range-for + while fixture, want >= 2 (loop-kind breadth regressed?):\n%s", n, output)
	}
}

// px2107TrivialSrc exercises PX2107's exponent scoping: the NON-actionable
// integer exponents 0 (pow(x,0)==1) and 1 (pow(x,1)==x) must NOT fire — for
// those "multiply directly" is wrong advice — while an actionable constant
// exponent (2) must. Motivated by corpus validation on abseil, where the broad
// matcher would have flagged trivial exponents.
const px2107TrivialSrc = `#include <cmath>
double zero(double x)  { return std::pow(x, 0); } // NOT flagged
double one(double x)   { return std::pow(x, 1); } // NOT flagged
double two(double x)   { return std::pow(x, 2); } // flagged
`

// TestPX2107ExcludesTrivialExponents pins that PX2107 fires on pow(x, 2) but not
// on pow(x, 0) or pow(x, 1), through the real --experimental-custom-checks
// engine — exactly one diagnostic on the three-call fixture.
func TestPX2107ExcludesTrivialExponents(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	e, ok := catalog.ByID("PX2107")
	if !ok || !e.Custom {
		t.Fatal("PX2107 missing or not a custom check")
	}
	cfg := catalog.ClangTidyConfig([]catalog.Entry{e})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".clang-tidy")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "pow.cpp")
	if err := os.WriteFile(src, []byte(px2107TrivialSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	args := []string{src, "--experimental-custom-checks", "--config-file=" + cfgPath, "--", "-std=c++17"}
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("xcrun", "--show-sdk-path").Output()
		if err != nil {
			t.Skip("xcrun --show-sdk-path failed; cannot locate the C++ sysroot")
		}
		args = append(args, "-isysroot", strings.TrimSpace(string(out)))
	}
	out, _ := exec.Command(bin, args...).CombinedOutput()
	output := string(out)

	if strings.Contains(output, "Unknown command line argument") && strings.Contains(output, "experimental-custom-checks") {
		t.Skip("clang-tidy is too old for --experimental-custom-checks; skipping")
	}
	if strings.Contains(output, "[clang-tidy-config]") {
		t.Fatalf("clang-tidy rejected the PX2107 query:\n%s", output)
	}
	if strings.Contains(output, "file not found") || strings.Contains(output, "fatal error:") {
		t.Skipf("toolchain could not parse the fixture headers; skipping:\n%s", output)
	}

	tag := "[" + e.TidyName + "]"
	if n := strings.Count(output, tag); n != 1 {
		t.Errorf("PX2107 fired %d time(s), want exactly 1 (only pow(x,2); pow(x,0)/pow(x,1) excluded):\n%s", n, output)
	}
}

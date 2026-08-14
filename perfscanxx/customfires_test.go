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

// px2101ReserveSrc documents PX2101's KNOWN LIMITATION: the AST matcher cannot see
// whether the grown vector was already reserve()'d in a preceding sibling
// statement (that needs data-flow), so it fires on BOTH an un-reserved loop AND an
// already-reserved one. The Title/Message are worded so the already-reserved case
// reads as a "confirm you reserved" nudge rather than a false claim. This test
// pins that behavior so a future data-flow refinement that DOES exclude reserved
// loops updates it consciously.
const px2101ReserveSrc = `#include <vector>
void noReserve(std::vector<int>& v, int n) {
  for (int i = 0; i < n; ++i) v.push_back(i);   // fires: genuine
}
void withReserve(std::vector<int>& v, int n) {
  v.reserve(n);
  for (int i = 0; i < n; ++i) v.push_back(i);    // ALSO fires: matcher can't see the reserve
}
`

// TestPX2101FiresRegardlessOfPriorReserve pins the documented limitation: two
// findings, one per loop, whether or not a reserve precedes the loop.
func TestPX2101FiresRegardlessOfPriorReserve(t *testing.T) {
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
	src := filepath.Join(dir, "reserve.cpp")
	if err := os.WriteFile(src, []byte(px2101ReserveSrc), 0o644); err != nil {
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
		t.Skipf("toolchain could not parse the fixture; skipping:\n%s", output)
	}

	tag := "[" + e.TidyName + "]"
	if n := strings.Count(output, tag); n != 2 {
		t.Errorf("PX2101 fired %d time(s), want 2 (both loops; the matcher can't exclude a prior reserve):\n%s", n, output)
	}
}

// px2102ParamSrc distinguishes PX2102's real target (a named LOCAL, where
// std::move blocks NRVO) from a by-value PARAMETER, where NRVO does not apply at
// all — copy elision is barred for function parameters, and `return param;`
// already implicit-moves — so `return std::move(param)` is redundant-but-harmless,
// NOT a pessimization. Parameters have local storage, so the earlier
// hasLocalStorage()-only matcher flagged them (a false positive with a message
// about NRVO that does not apply). The query now adds unless(parmVarDecl()).
const px2102ParamSrc = `#include <string>
std::string localMove() {
  std::string s = "x";
  return std::move(s);   // PX2102: real NRVO pessimization on a local
}
std::string paramMove(std::string p) {
  return std::move(p);   // by-value parameter: must NOT fire
}
`

// TestPX2102DoesNotFireOnParameterMove pins that PX2102 fires on the local move
// but NOT on the by-value-parameter move — the false positive that
// unless(parmVarDecl()) removed.
func TestPX2102DoesNotFireOnParameterMove(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	e, ok := catalog.ByID("PX2102")
	if !ok || !e.Custom {
		t.Fatal("PX2102 missing or not a custom check")
	}
	cfg := catalog.ClangTidyConfig([]catalog.Entry{e})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".clang-tidy")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "moves.cpp")
	if err := os.WriteFile(src, []byte(px2102ParamSrc), 0o644); err != nil {
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
		t.Fatalf("clang-tidy rejected the PX2102 query:\n%s", output)
	}
	if strings.Contains(output, "file not found") || strings.Contains(output, "fatal error:") {
		t.Skipf("toolchain could not parse the fixture; skipping:\n%s", output)
	}

	tag := "[" + e.TidyName + "]"
	if n := strings.Count(output, tag); n != 1 {
		t.Errorf("PX2102 fired %d time(s), want exactly 1 (the local move only):\n%s", n, output)
	}
	if !strings.Contains(output, "moves.cpp:4:") {
		t.Errorf("PX2102 must fire on the LOCAL move at line 4:\n%s", output)
	}
	if strings.Contains(output, "moves.cpp:7:") {
		t.Errorf("PX2102 must NOT fire on the by-value PARAMETER move at line 7 (NRVO does not apply to parameters):\n%s", output)
	}
}

// px2103RefSrc pins PX2103's by-value scope: catching an exception BY VALUE
// copies (and can slice), so it fires; but catch by CONST REFERENCE — the
// idiomatic, recommended form — as well as plain reference and pointer, take no
// copy and must NOT fire. The query keys on hasCanonicalType(recordType()): a
// reference or pointer catch has a canonical type of reference/pointer, not
// record, so it is excluded. This is the highest-stakes negative in the custom
// set: a regression that dropped the reference exclusion would flag EVERY
// well-written catch block.
const px2103RefSrc = `#include <stdexcept>
void byValue()    { try {} catch (std::runtime_error e) { (void)e; } }        // PX2103: copies
void byConstRef() { try {} catch (const std::runtime_error& e) { (void)e; } } // idiomatic: NOT flagged
void byRef()      { try {} catch (std::runtime_error& e) { (void)e; } }        // NOT flagged
void byPointer()  { try {} catch (std::runtime_error* e) { (void)e; } }        // NOT flagged
`

// TestPX2103DoesNotFireOnCatchByReference pins that PX2103 fires only on the
// by-value catch (line 2) and stays silent on const-ref / ref / pointer catches.
func TestPX2103DoesNotFireOnCatchByReference(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	e, ok := catalog.ByID("PX2103")
	if !ok || !e.Custom {
		t.Fatal("PX2103 missing or not a custom check")
	}
	cfg := catalog.ClangTidyConfig([]catalog.Entry{e})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".clang-tidy")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "catch.cpp")
	if err := os.WriteFile(src, []byte(px2103RefSrc), 0o644); err != nil {
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
		t.Fatalf("clang-tidy rejected the PX2103 query:\n%s", output)
	}
	if strings.Contains(output, "file not found") || strings.Contains(output, "fatal error:") {
		t.Skipf("toolchain could not parse the fixture; skipping:\n%s", output)
	}

	tag := "[" + e.TidyName + "]"
	if n := strings.Count(output, tag); n != 1 {
		t.Errorf("PX2103 fired %d time(s), want exactly 1 (the by-value catch only):\n%s", n, output)
	}
	if !strings.Contains(output, "catch.cpp:2:") {
		t.Errorf("PX2103 must fire on the by-VALUE catch at line 2:\n%s", output)
	}
	for _, ln := range []string{"catch.cpp:3:", "catch.cpp:4:", "catch.cpp:5:"} {
		if strings.Contains(output, ln) {
			t.Errorf("PX2103 must NOT fire on a by-reference/pointer catch (%s):\n%s", ln, output)
		}
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

// loopKindBreadthSrc exhibits the three "-in-loop" custom checks (regex,
// dynamic-cast, stringstream) inside a RANGE-FOR and a WHILE loop — NOT the
// counted for that TestCustomChecksFireOnTargetPattern uses. All three share
// PX2101's anyOf(forStmt, cxxForRangeStmt, whileStmt, doStmt) ancestor matcher;
// this pins that they fire across loop kinds, so a regression to a narrower
// matcher is caught. Motivated by corpus validation on yaml-cpp, where PX2106
// (stringstream-in-loop) fired on a real site.
const loopKindBreadthSrc = `#include <vector>
#include <regex>
#include <sstream>
struct B { virtual ~B(){} }; struct D : B {};
void rangeFor(const std::vector<int>& xs, B* p) {
  for (int x : xs) {                 // cxxForRangeStmt
    std::regex re("x");              // PX2104
    (void)dynamic_cast<D*>(p);       // PX2105
    std::ostringstream ss;           // PX2106
    (void)x; (void)re; (void)ss;
  }
}
void whileLoop(int n, B* p) {
  int i = 0;
  while (i < n) {                    // whileStmt
    std::regex re("y");              // PX2104
    (void)dynamic_cast<D*>(p);       // PX2105
    std::ostringstream ss;           // PX2106
    (void)re; (void)ss; ++i;
  }
}
`

// TestInLoopChecksFireAcrossLoopKinds pins that PX2104/PX2105/PX2106 each fire on
// a range-for AND a while loop (not just a counted for) — expecting at least 2
// diagnostics per check (one per loop). Guards the loop-kind breadth of the
// shared ancestor matcher for the "-in-loop" custom-check family.
func TestInLoopChecksFireAcrossLoopKinds(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	var sel []catalog.Entry
	for _, id := range []string{"PX2104", "PX2105", "PX2106"} {
		e, ok := catalog.ByID(id)
		if !ok || !e.Custom {
			t.Fatalf("%s missing or not a custom check", id)
		}
		sel = append(sel, e)
	}
	cfg := catalog.ClangTidyConfig(sel)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".clang-tidy")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "loops.cpp")
	if err := os.WriteFile(src, []byte(loopKindBreadthSrc), 0o644); err != nil {
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
		t.Fatalf("clang-tidy rejected a custom-check query:\n%s", output)
	}
	if strings.Contains(output, "file not found") || strings.Contains(output, "fatal error:") {
		t.Skipf("toolchain could not parse the fixture headers; skipping:\n%s", output)
	}

	for _, e := range sel {
		tag := "[" + e.TidyName + "]"
		if n := strings.Count(output, tag); n < 2 {
			t.Errorf("%s fired %d time(s) on a range-for + while fixture, want >= 2 (loop-kind breadth regressed?):\n%s", e.ID, n, output)
		}
	}
}

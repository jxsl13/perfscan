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
//	PX2107 pow-const-exponent, PX2108 vector-bool, PX2109 std-list,
//	PX2110 count-for-existence, PX2111 map-double-lookup, PX2112 return-move-temporary.
const customTriggerSrc = `#include <vector>
#include <regex>
#include <sstream>
#include <exception>
#include <cmath>
#include <list>
#include <algorithm>
#include <map>
#include <utility>
struct B { virtual ~B(){} }; struct D : B {};
int dbl(std::map<int,int>& m, int k) { if (m.count(k)) { return m[k]; } return 0; } // PX2111 (double lookup)
std::vector<int> makeVecPX2112();
std::vector<int> retMoveTemp() { return std::move(makeVecPX2112()); } // PX2112 (move of a prvalue temporary)
std::vector<bool> g_flags; // PX2108 (space-optimized bitfield, not a real container)
std::list<int> g_items;    // PX2109 (node-per-element linked list)
bool present(const std::vector<int>& v, int x) {
  return std::count(v.begin(), v.end(), x) > 0; // PX2110 (full scan for existence)
}
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

// px2108VectorBoolSrc pins PX2108's scope: std::vector<bool> is flagged at a
// FIELD and a LOCAL declaration (the storage-choice sites), but NOT on a
// std::vector<int> (a real container) and NOT on a by-value PARAMETER (a
// pass-through whose storage choice lives at the caller's declaration site). The
// query keys on the canonical std::vector<bool> specialization with
// unless(parmVarDecl()).
const px2108VectorBoolSrc = `#include <vector>
struct Widget { std::vector<bool> flags; };   // field: PX2108 (line 2)
void f() {
  std::vector<bool> local;                     // local: PX2108 (line 4)
  std::vector<int> ints;                        // real container: NOT flagged
  (void)ints;
}
void byval(std::vector<bool> p);               // parameter: NOT flagged
`

// TestPX2108DoesNotFireOnNonBoolVectorOrParam pins that PX2108 fires on the
// vector<bool> field and local (2 findings) and stays silent on vector<int> and
// on the by-value vector<bool> parameter.
func TestPX2108DoesNotFireOnNonBoolVectorOrParam(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	e, ok := catalog.ByID("PX2108")
	if !ok || !e.Custom {
		t.Fatal("PX2108 missing or not a custom check")
	}
	cfg := catalog.ClangTidyConfig([]catalog.Entry{e})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".clang-tidy")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "vb.cpp")
	if err := os.WriteFile(src, []byte(px2108VectorBoolSrc), 0o644); err != nil {
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
		t.Fatalf("clang-tidy rejected the PX2108 query:\n%s", output)
	}
	if strings.Contains(output, "file not found") || strings.Contains(output, "fatal error:") {
		t.Skipf("toolchain could not parse the fixture; skipping:\n%s", output)
	}

	tag := "[" + e.TidyName + "]"
	if n := strings.Count(output, tag); n != 2 {
		t.Errorf("PX2108 fired %d time(s), want exactly 2 (the field and the local):\n%s", n, output)
	}
	if !strings.Contains(output, "vb.cpp:2:") {
		t.Errorf("PX2108 must fire on the vector<bool> FIELD at line 2:\n%s", output)
	}
	if !strings.Contains(output, "vb.cpp:4:") {
		t.Errorf("PX2108 must fire on the vector<bool> LOCAL at line 4:\n%s", output)
	}
	if strings.Contains(output, "vb.cpp:8:") {
		t.Errorf("PX2108 must NOT fire on the by-value vector<bool> PARAMETER at line 8:\n%s", output)
	}
}

// px2109StdListSrc pins PX2109's scope: std::list and std::forward_list are
// flagged at a FIELD and a LOCAL declaration, but NOT on a std::vector (the
// recommended container) and NOT on a by-value list PARAMETER (a pass-through).
const px2109StdListSrc = `#include <list>
#include <forward_list>
#include <vector>
struct Node { std::list<int> kids; };          // field: PX2109 (line 4)
void f() {
  std::list<int> local;                         // local: PX2109 (line 6)
  std::forward_list<int> fl;                     // local: PX2109 (line 7)
  std::vector<int> v;                            // recommended: NOT flagged
  (void)local; (void)fl; (void)v;
}
void byval(std::list<int> p);                    // parameter: NOT flagged
`

// TestPX2109DoesNotFireOnVectorOrParam pins that PX2109 fires on the list and
// forward_list field/locals (3 findings) and stays silent on std::vector and on
// the by-value list parameter.
func TestPX2109DoesNotFireOnVectorOrParam(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	e, ok := catalog.ByID("PX2109")
	if !ok || !e.Custom {
		t.Fatal("PX2109 missing or not a custom check")
	}
	cfg := catalog.ClangTidyConfig([]catalog.Entry{e})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".clang-tidy")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "list.cpp")
	if err := os.WriteFile(src, []byte(px2109StdListSrc), 0o644); err != nil {
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
		t.Fatalf("clang-tidy rejected the PX2109 query:\n%s", output)
	}
	if strings.Contains(output, "file not found") || strings.Contains(output, "fatal error:") {
		t.Skipf("toolchain could not parse the fixture; skipping:\n%s", output)
	}

	tag := "[" + e.TidyName + "]"
	if n := strings.Count(output, tag); n != 3 {
		t.Errorf("PX2109 fired %d time(s), want exactly 3 (list+forward_list field and locals):\n%s", n, output)
	}
	if strings.Contains(output, "list.cpp:8:") {
		t.Errorf("PX2109 must NOT fire on the std::vector at line 8:\n%s", output)
	}
	if strings.Contains(output, "list.cpp:11:") {
		t.Errorf("PX2109 must NOT fire on the by-value list PARAMETER at line 11:\n%s", output)
	}
}

// px2110CountSrc pins PX2110's precise scope: std::count compared for EXISTENCE
// (> 0, != 0, >= 1) is flagged, but count(...) > 1 (a genuine "more than one"
// test that needs the count), count(...) == k, a bare count with no comparison,
// and a member .count() on a set (its own O(log n) primitive, not the free
// algorithm) are all silent.
const px2110CountSrc = `#include <vector>
#include <algorithm>
#include <set>
bool exists0(const std::vector<int>& v, int x) { return std::count(v.begin(), v.end(), x) > 0; }  // MATCH line 4
bool existsNe(const std::vector<int>& v, int x){ return std::count(v.begin(), v.end(), x) != 0; }  // MATCH line 5
bool existsGe(const std::vector<int>& v, int x){ return std::count(v.begin(), v.end(), x) >= 1; }  // MATCH line 6
bool dup(const std::vector<int>& v, int x)     { return std::count(v.begin(), v.end(), x) > 1; }   // NO (count needed)
bool eqK(const std::vector<int>& v, int x)     { return std::count(v.begin(), v.end(), x) == 3; }  // NO (specific count)
long bare(const std::vector<int>& v, int x)    { return std::count(v.begin(), v.end(), x); }        // NO (bare)
bool member(const std::set<int>& s, int x)     { return s.count(x) > 0; }                            // NO (member count)
`

// TestPX2110DoesNotFireOnCountNeeded pins that PX2110 fires on the three
// existence comparisons (3 findings) and stays silent on > 1, == k, bare count,
// and a member .count().
func TestPX2110DoesNotFireOnCountNeeded(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	e, ok := catalog.ByID("PX2110")
	if !ok || !e.Custom {
		t.Fatal("PX2110 missing or not a custom check")
	}
	cfg := catalog.ClangTidyConfig([]catalog.Entry{e})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".clang-tidy")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "count.cpp")
	if err := os.WriteFile(src, []byte(px2110CountSrc), 0o644); err != nil {
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
		t.Fatalf("clang-tidy rejected the PX2110 query:\n%s", output)
	}
	if strings.Contains(output, "file not found") || strings.Contains(output, "fatal error:") {
		t.Skipf("toolchain could not parse the fixture; skipping:\n%s", output)
	}

	tag := "[" + e.TidyName + "]"
	if n := strings.Count(output, tag); n != 3 {
		t.Errorf("PX2110 fired %d time(s), want exactly 3 (the >0, !=0, >=1 existence checks):\n%s", n, output)
	}
	for _, bad := range []string{"count.cpp:7:", "count.cpp:8:", "count.cpp:9:", "count.cpp:10:"} {
		if strings.Contains(output, bad) {
			t.Errorf("PX2110 fired at %s — it must not flag > 1, == k, bare count, or a member .count():\n%s", bad, output)
		}
	}
}

// px2111DoubleLookupSrc pins PX2111's precision: it fires only when the SAME map
// and SAME key are looked up in both the condition (count) and the body (m[k]).
// Different keys, a missing body [], and the free std::count algorithm stay silent.
const px2111DoubleLookupSrc = `#include <map>
#include <vector>
#include <algorithm>
int same(std::map<int,int>& m, int k)          { if (m.count(k)) { return m[k]; } return 0; }        // MATCH line 4
int write(std::map<int,int>& m, int k)         { if (m.count(k) > 0) { m[k] = 1; } return 0; }       // MATCH line 5
int diffKey(std::map<int,int>& m, int a, int b){ if (m.count(a)) { return m[b]; } return 0; }         // NO (diff keys)
int noAccess(std::map<int,int>& m, int k)      { if (m.count(k)) { return 7; } return 0; }            // NO (no [] in body)
int noCheck(std::map<int,int>& m, int k)       { return m[k]; }                                        // NO (no existence check)
bool freeCount(const std::vector<int>& v,int x){ return std::count(v.begin(), v.end(), x) > 0; }       // NO (PX2110, not this)
int findForm(std::map<int,int>& m, int k)      { if (m.find(k) != m.end()) { return m[k]; } return 0; } // MATCH line 10 (find!=end)
int findAbsent(std::map<int,int>& m, int k)    { if (m.find(k) == m.end()) { return 0; } return m[k]; } // NO line 11 (==end: insert, not redundant)
`

// TestPX2111DoesNotFireOnDifferentKeyOrNoAccess pins that PX2111 fires on the two
// genuine same-map/same-key double lookups (2 findings) and stays silent on a
// different-key body access, a body with no operator[], a bare access with no
// existence check, and the free std::count algorithm.
func TestPX2111DoesNotFireOnDifferentKeyOrNoAccess(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	e, ok := catalog.ByID("PX2111")
	if !ok || !e.Custom {
		t.Fatal("PX2111 missing or not a custom check")
	}
	cfg := catalog.ClangTidyConfig([]catalog.Entry{e})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".clang-tidy")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "dl.cpp")
	if err := os.WriteFile(src, []byte(px2111DoubleLookupSrc), 0o644); err != nil {
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
		t.Fatalf("clang-tidy rejected the PX2111 query:\n%s", output)
	}
	if strings.Contains(output, "file not found") || strings.Contains(output, "fatal error:") {
		t.Skipf("toolchain could not parse the fixture; skipping:\n%s", output)
	}

	tag := "[" + e.TidyName + "]"
	if n := strings.Count(output, tag); n != 3 {
		t.Errorf("PX2111 fired %d time(s), want exactly 3 (count read, count-write, find!=end):\n%s", n, output)
	}
	for _, bad := range []string{"dl.cpp:6:", "dl.cpp:7:", "dl.cpp:8:", "dl.cpp:9:", "dl.cpp:11:"} {
		if strings.Contains(output, bad) {
			t.Errorf("PX2111 fired at %s — it must not flag a different key, a missing body access, a bare access, the free std::count, or the find()==end() absence form:\n%s", bad, output)
		}
	}
}

// px2112MoveTempSrc pins PX2112's scope: it fires on `return std::move(<prvalue
// temporary>)` (a by-value call or ctor temporary), but NOT on std::move of an
// lvalue reference (a real move), NOT on std::move of a named local (PX2102's
// job), and NOT on a plain `return call();` with no move.
const px2112MoveTempSrc = `#include <utility>
#include <string>
#include <vector>
std::string byVal();
std::string& byRef();
std::string prvalueCall(){ return std::move(byVal()); }                 // MATCH line 6
std::vector<int> ctorTemp(){ return std::move(std::vector<int>{1,2}); }  // MATCH line 7
std::string lvalueRef(){ return std::move(byRef()); }                    // NO line 8 (real move of an lvalue)
std::string namedLocal(){ std::string s; return std::move(s); }          // NO line 9 (PX2102, a local)
std::string noMove(){ return byVal(); }                                  // NO line 10 (no move)
`

// TestPX2112DoesNotFireOnLvalueOrLocal pins that PX2112 fires on the two
// move-of-prvalue-temporary returns (2 findings) and stays silent on a move of an
// lvalue reference, a move of a named local, and a plain return without move.
func TestPX2112DoesNotFireOnLvalueOrLocal(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	e, ok := catalog.ByID("PX2112")
	if !ok || !e.Custom {
		t.Fatal("PX2112 missing or not a custom check")
	}
	cfg := catalog.ClangTidyConfig([]catalog.Entry{e})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".clang-tidy")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "mv.cpp")
	if err := os.WriteFile(src, []byte(px2112MoveTempSrc), 0o644); err != nil {
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
		t.Fatalf("clang-tidy rejected the PX2112 query:\n%s", output)
	}
	if strings.Contains(output, "file not found") || strings.Contains(output, "fatal error:") {
		t.Skipf("toolchain could not parse the fixture; skipping:\n%s", output)
	}

	tag := "[" + e.TidyName + "]"
	if n := strings.Count(output, tag); n != 2 {
		t.Errorf("PX2112 fired %d time(s), want exactly 2 (the prvalue call and the ctor temporary):\n%s", n, output)
	}
	for _, bad := range []string{"mv.cpp:8:", "mv.cpp:9:", "mv.cpp:10:"} {
		if strings.Contains(output, bad) {
			t.Errorf("PX2112 fired at %s — it must not flag a move of an lvalue ref, a named local, or a plain return:\n%s", bad, output)
		}
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

// px2104StaticSrc distinguishes the in-loop-construction anti-pattern from its
// idiomatic hoist: an AUTOMATIC std::regex / std::ostringstream declared in a
// loop is re-created every iteration (PX2104 / PX2106 fire), but a STATIC or
// thread_local one is initialized ONCE — `static const std::regex re(...)` inside
// a function is in fact the recommended way to hoist a regex — so it does not
// recompile/reallocate per iteration and must NOT fire (both queries carry
// hasAutomaticStorageDuration()).
const px2104StaticSrc = `#include <regex>
#include <sstream>
void rxPerIter() { for (int i=0;i<9;++i){ std::regex re("a+"); (void)re; } }             // PX2104
void rxStatic()  { for (int i=0;i<9;++i){ static std::regex re("a+"); (void)re; } }       // NOT
void rxThread()  { for (int i=0;i<9;++i){ thread_local std::regex re("a+"); (void)re; } } // NOT
void ssPerIter() { for (int i=0;i<9;++i){ std::ostringstream ss; (void)ss; } }            // PX2106
void ssStatic()  { for (int i=0;i<9;++i){ static std::ostringstream ss; (void)ss; } }      // NOT
`

// TestRegexAndStreamInLoopExcludeStaticStorage pins that PX2104 and PX2106 fire
// only on the automatic (per-iteration) declarations, never on static/thread_local.
func TestRegexAndStreamInLoopExcludeStaticStorage(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	var sel []catalog.Entry
	for _, id := range []string{"PX2104", "PX2106"} {
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
	src := filepath.Join(dir, "loop.cpp")
	if err := os.WriteFile(src, []byte(px2104StaticSrc), 0o644); err != nil {
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
		t.Fatalf("clang-tidy rejected a PX2104/PX2106 query:\n%s", output)
	}
	if strings.Contains(output, "file not found") || strings.Contains(output, "fatal error:") {
		t.Skipf("toolchain could not parse the fixture headers; skipping:\n%s", output)
	}

	total := strings.Count(output, "[custom-regex-in-loop]") + strings.Count(output, "[custom-stringstream-in-loop]")
	if total != 2 {
		t.Errorf("PX2104+PX2106 fired %d time(s), want 2 (the automatic regex + stream only):\n%s", total, output)
	}
	if !strings.Contains(output, "loop.cpp:3:") || !strings.Contains(output, "loop.cpp:6:") {
		t.Errorf("must fire on the automatic declarations at lines 3 (regex) and 6 (stream):\n%s", output)
	}
	for _, ln := range []string{"loop.cpp:4:", "loop.cpp:5:", "loop.cpp:7:"} {
		if strings.Contains(output, ln) {
			t.Errorf("must NOT fire on a static/thread_local declaration (%s):\n%s", ln, output)
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

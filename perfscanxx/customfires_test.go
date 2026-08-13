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

// customTriggerSrc exhibits ALL six query-based custom-check anti-patterns in one
// main-file translation unit (the checks carry isExpansionInMainFile guards):
//
//	PX2101 reserve-before-loop, PX2102 pessimizing-move, PX2103 catch-by-value,
//	PX2104 regex-in-loop, PX2105 dynamic-cast-in-loop, PX2106 stringstream-in-loop.
const customTriggerSrc = `#include <vector>
#include <regex>
#include <sstream>
#include <exception>
struct B { virtual ~B(){} }; struct D : B {};
std::vector<int> pessimizing() {
  std::vector<int> v;
  return std::move(v); // PX2102
}
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

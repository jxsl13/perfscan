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

// cppCompileCmdForTest returns the clang compile command for a fixture, adding
// the SDK sysroot on darwin (libc++ headers live under the SDK). ok is false
// when the sysroot cannot be located, so the caller skips rather than fails —
// the repo's setup-difference idiom (see TestFixAppliesPX3023).
func cppCompileCmdForTest(file string) (cmd string, ok bool) {
	cmd = "clang++ -std=c++20"
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("xcrun", "--show-sdk-path").Output()
		if err != nil {
			return "", false
		}
		cmd += " -isysroot " + strings.TrimSpace(string(out))
	}
	return cmd + " -c " + file, true
}

// TestHasFixChecksActuallyApply is the catalog-wide generalization of
// TestFixAppliesPX3023: for a representative HasFix:true built-in check spanning
// the major categories (copies, io, strings, containers, allocation), it runs
// `perfscanxx -fix` (which drives clang-tidy --fix) over a minimal triggering
// fixture and asserts the anti-pattern is actually rewritten. This enforces the
// catalog's "HasFix true only if a fix-it actually applies" discipline against
// the LIVE toolchain — if clang-tidy ever drops, renames, or gates a fix-it, the
// HasFix claim (which perfscanxx advertises as "fix available" and applies under
// -fix) fails here instead of silently no-op'ing on user code.
//
// It uses -fix, NOT --export-fixes: some checks (e.g.
// readability-container-size-empty) DO apply a fix via clang-tidy --fix yet emit
// an EMPTY Replacements list in --export-fixes, so an export-based assertion
// would spuriously fail. -fix is also the exact path a user hits.
func TestHasFixChecksActuallyApply(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	cases := []struct {
		id     string // catalog PX id; the test asserts it is HasFix:true
		src    string
		want   string // must appear after the fix ("" skips this assertion)
		unwant string // must be GONE after the fix ("" skips this assertion)
	}{
		{"PX1001", "#include <vector>\n#include <string>\nvoid f(const std::vector<std::string>& v){ for(auto s : v){ (void)s; } }\n", "const auto&", ""},
		{"PX1003", "#include <string>\nvoid sink(std::string); void f(const std::string& s){ std::string t = s; sink(t); }\n", "", "std::string t = s"},
		{"PX3003", "#include <iostream>\nvoid f(){ std::cout << std::endl; }\n", "", "endl"},
		{"PX3008", "#include <vector>\nvoid use(bool); void f(const std::vector<int>& v){ if (v.size() == 0) use(true); }\n", ".empty()", "size() == 0"},
		{"PX3009", "#include <string>\nvoid g(const std::string&); void f(const std::string& s){ g(s.c_str()); }\n", "", ".c_str()"},
		{"PX3014", "#include <string>\nbool f(const std::string& a, const std::string& b){ return a.compare(b) == 0; }\n", "a == b", ".compare("},
		{"PX3018", "#include <string>\nvoid f(){ std::string s = \"\"; (void)s; }\n", "", "= \"\""},
		{"PX2005", "#include <memory>\nvoid f(){ std::unique_ptr<int> p(new int(5)); (void)p; }\n", "make_unique", "new int"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			e, ok := catalog.ByID(tc.id)
			if !ok || !e.HasFix {
				t.Fatalf("%s must be a HasFix:true catalog entry (ok=%v); fixture is stale", tc.id, ok)
			}
			dir := t.TempDir()
			cpp := filepath.Join(dir, "s.cpp")
			if err := os.WriteFile(cpp, []byte(tc.src), 0o644); err != nil {
				t.Fatal(err)
			}
			compile, okc := cppCompileCmdForTest("s.cpp")
			if !okc {
				t.Skip("xcrun --show-sdk-path failed; cannot locate the C++ sysroot")
			}
			cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"` + compile + `"}]`
			if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
				t.Fatal(err)
			}

			_, stderr, _ := runCLI("-tidy", bin, "-fix", "-checks", tc.id, "-p", dir, cpp)
			gotB, err := os.ReadFile(cpp)
			if err != nil {
				t.Fatal(err)
			}
			got := string(gotB)

			if got == tc.src {
				// Distinguish a genuine HasFix regression from a stripped toolchain
				// that cannot parse the standard headers (no libc++): clang-tidy
				// prints a fatal "file not found" and applies no fix when the TU
				// does not parse.
				if strings.Contains(stderr, "file not found") || strings.Contains(stderr, "fatal error") {
					t.Skipf("toolchain could not parse the fixture headers; skipping. stderr:\n%s", stderr)
				}
				t.Fatalf("%s (%s) is HasFix:true but -fix changed nothing — clang-tidy applied no fix-it:\n%s", tc.id, e.TidyName, got)
			}
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("%s (%s): expected %q in the fixed file:\n%s", tc.id, e.TidyName, tc.want, got)
			}
			if tc.unwant != "" && strings.Contains(got, tc.unwant) {
				t.Errorf("%s (%s): anti-pattern %q still present after -fix:\n%s", tc.id, e.TidyName, tc.unwant, got)
			}
		})
	}
}

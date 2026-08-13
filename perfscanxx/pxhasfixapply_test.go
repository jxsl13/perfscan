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
	for _, tc := range hasFixFixtures() {
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

// hasFixCase is one HasFix:true built-in fixture: src triggers check id, and
// after `perfscanxx -fix` want must appear and unwant must be gone (either "" to
// skip that assertion).
type hasFixCase struct {
	id     string
	src    string
	want   string
	unwant string
}

// hasFixFixtures is a triggering fixture for EVERY HasFix:true built-in check
// (all 25). TestHasFixFixtureCompleteness keeps it exhaustive; the fix-it is
// verified end-to-end by TestHasFixChecksActuallyApply.
func hasFixFixtures() []hasFixCase {
	return []hasFixCase{
		{"PX1001", "#include <vector>\n#include <string>\nvoid f(const std::vector<std::string>& v){ for(auto s : v){ (void)s; } }\n", "const auto&", ""},
		{"PX1002", "#include <string>\nvoid g(const std::string&); void f(std::string s){ g(s); }\n", "const std::string& s", "f(std::string s)"},
		{"PX1003", "#include <string>\nvoid sink(std::string); void f(const std::string& s){ std::string t = s; sink(t); }\n", "", "std::string t = s"},
		{"PX3003", "#include <iostream>\nvoid f(){ std::cout << std::endl; }\n", "", "endl"},
		{"PX3008", "#include <vector>\nvoid use(bool); void f(const std::vector<int>& v){ if (v.size() == 0) use(true); }\n", ".empty()", "size() == 0"},
		{"PX3009", "#include <string>\nvoid g(const std::string&); void f(const std::string& s){ g(s.c_str()); }\n", "", ".c_str()"},
		{"PX3014", "#include <string>\nbool f(const std::string& a, const std::string& b){ return a.compare(b) == 0; }\n", "a == b", ".compare("},
		{"PX3018", "#include <string>\nvoid f(){ std::string s = \"\"; (void)s; }\n", "", "= \"\""},
		{"PX2005", "#include <memory>\nvoid f(){ std::unique_ptr<int> p(new int(5)); (void)p; }\n", "make_unique", "new int"},
		{"PX3001", "#include <utility>\nstruct T{}; void sink(T); void f(){ const T t; sink(std::move(t)); }\n", "", "std::move"},
		{"PX3002", "#include <string>\nstd::string::size_type f(const std::string& s){ return s.find(\"x\"); }\n", "find('x')", "find(\"x\")"},
		{"PX3010", "#include <set>\nbool f(const std::set<int>& s, int k){ return s.find(k) != s.end(); }\n", ".contains(", "!= s.end()"},
		{"PX3019", "#include <string>\nbool f(const std::string& s, const std::string& pre){ return s.substr(0, pre.size()) == pre; }\n", "starts_with", ".substr("},
		{"PX3023", "#include <vector>\nvoid f(std::vector<int>& v){ std::vector<int>(v).swap(v); }\n", "shrink_to_fit", ".swap("},
		{"PX2003", "#include <vector>\n#include <utility>\nvoid f(std::vector<std::pair<int,int>>& v){ v.push_back(std::make_pair(1,2)); }\n", "emplace_back", "make_pair"},
		{"PX2004", "#include <memory>\nvoid f(){ std::shared_ptr<int> p(new int(5)); (void)p; }\n", "make_shared", "new int"},
		{"PX3005", "#include <set>\n#include <algorithm>\nbool f(const std::set<int>& s, int k){ return std::find(s.begin(), s.end(), k) != s.end(); }\n", ".find(k)", "std::find("},
		{"PX3013", "struct S { S() {} };\n", "= default", "S() {}"},
		{"PX2001", "#include <vector>\nvoid f(){ std::vector<int> v; for(int i=0;i<100;++i) v.push_back(i); }\n", "reserve(", ""},
		{"PX3004", "#include <vector>\nstruct S { std::vector<int> v; S(S&& o){ v = static_cast<std::vector<int>&&>(o.v); } };\n", "noexcept", ""},
		{"PX3012", "#include <set>\n#include <functional>\nstd::set<int, std::less<int>> f(){ return {}; }\n", "std::less<>", "std::less<int>"},
		{"PX3015", "struct S { int x; S(int v){ x = v; } };\n", ": x(v)", "x = v"},
		{"PX3006", "#include <vector>\nstruct S { std::vector<int> v; void swap(S& o){ v.swap(o.v); } };\n", "noexcept", ""},
		{"PX3007", "#include <string>\nstruct S { std::string m; S(const std::string& s) : m(s) {} };\n", "std::move(s)", "const std::string& s"},
		{"PX3011", "struct Big { int a[8]; };\nconst Big make() { return Big(); }\n", "", "const Big"},
		{"PX3016", "#include <functional>\nvoid g(int,int); auto f(){ return std::bind(g, 1, 2); }\n", "[]", "std::bind"},
	}
}

// TestHasFixFixtureCompleteness asserts every HasFix:true built-in catalog entry
// has a fixture in hasFixFixtures — so a future fixable check cannot be added to
// the catalog without an end-to-end verification that its fix-it actually
// applies. Custom query-based checks carry no clang-tidy fix-it (HasFix:false)
// and are excluded. Needs no clang-tidy (pure catalog-vs-fixtures).
func TestHasFixFixtureCompleteness(t *testing.T) {
	covered := make(map[string]bool)
	for _, tc := range hasFixFixtures() {
		covered[tc.id] = true
	}
	for _, e := range catalog.All() {
		if e.HasFix && !e.Custom && !covered[e.ID] {
			t.Errorf("%s (%s) is HasFix:true but has no fixture in hasFixFixtures — add one", e.ID, e.TidyName)
		}
	}
}

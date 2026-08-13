package main

import (
	"encoding/json"
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

// TestPX3026DoesNotFireOnNonTrivialMember pins the SAFETY BOUNDARY of the
// trivially-destructible fix (PX3026): it must fire ONLY on a type that can
// actually be made trivially destructible. A class with a non-trivial member
// (std::string here) is NOT trivially destructible even with a defaulted
// destructor, so clang-tidy must leave the out-of-line `= default` alone —
// removing it would neither help nor be what the check claims. If a future
// clang-tidy broadened the check to such types, applying the "fix" would be
// misleading, so this locks the boundary against the LIVE toolchain.
//
// The fixture is the exact positive shape (out-of-line defaulted destructor)
// EXCEPT for the non-trivial member, isolating that single dimension.
func TestPX3026DoesNotFireOnNonTrivialMember(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	const src = "#include <string>\nstruct S { std::string s; ~S(); };\nS::~S() = default;\n"
	dir := t.TempDir()
	cpp := filepath.Join(dir, "s.cpp")
	if err := os.WriteFile(cpp, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	compile, ok := cppCompileCmdForTest("s.cpp")
	if !ok {
		t.Skip("xcrun --show-sdk-path failed; cannot locate the C++ sysroot")
	}
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"` + compile + `"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}
	out, stderr, _ := runCLI("-tidy", bin, "-fix", "-checks", "PX3026", "-p", dir, cpp)
	if strings.Contains(stderr, "file not found") || strings.Contains(stderr, "fatal error") {
		t.Skipf("toolchain could not parse the fixture headers; skipping. stderr:\n%s", stderr)
	}
	gotB, err := os.ReadFile(cpp)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotB) != src {
		t.Errorf("PX3026 rewrote a type with a non-trivial member (std::string) — the fix must only touch types that CAN be made trivially destructible:\n%s", string(gotB))
	}
	if strings.Contains(out, "PX3026") {
		t.Errorf("PX3026 reported on a non-trivially-destructible type; it must stay silent:\n%s", out)
	}
}

// TestFixWithArgumentsFormCompdb pins the FULL pipeline against a compilation
// database that uses the "arguments" ARRAY form instead of the "command" STRING
// form. The JSON Compilation Database spec allows either and real generators
// (Bazel; CMake with some generators) emit "arguments" — every other test here
// uses "command", so this is the untested real-world variant. perfscanxx
// enumerates the TU from directory+file (form-agnostic) and hands it to
// clang-tidy, which reads the arguments to compile; the PX3013 fix must apply
// end-to-end exactly as it does for the command form. Headerless TU, so no
// sysroot is needed. Skipped when clang-tidy is unavailable.
func TestFixWithArgumentsFormCompdb(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	const before = "struct S {\n  ~S() {}\n};\n"
	dir := t.TempDir()
	cpp := filepath.Join(dir, "t.cpp")
	if err := os.WriteFile(cpp, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	// The "arguments" array form (json.Marshal keeps each path a valid literal).
	fileJSON, _ := json.Marshal(cpp)
	dirJSON, _ := json.Marshal(dir)
	cc := `[{"directory":` + string(dirJSON) + `,"file":` + string(fileJSON) +
		`,"arguments":["clang++","-std=c++17","-c","t.cpp"]}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, _ := runCLI("-tidy", bin, "-fix", "-checks", "PX3013", "-p", dir, cpp)
	gotB, err := os.ReadFile(cpp)
	if err != nil {
		t.Fatal(err)
	}
	got := string(gotB)
	if got == before {
		if strings.Contains(stderr, "file not found") || strings.Contains(stderr, "fatal error") {
			t.Skipf("toolchain could not parse the fixture; skipping. stderr:\n%s", stderr)
		}
		t.Fatalf("PX3013 did not apply with an arguments-form compdb — the array form is not being honored:\n%s", got)
	}
	if !strings.Contains(got, "= default") {
		t.Errorf("expected the PX3013 fix (`= default`) with an arguments-form compdb:\n%s", got)
	}
}

// TestPX3026FiresWithDeletedCopyOps pins PX3026 against a production-realistic
// class shape found during corpus -fix validation on google/leveldb
// (db/log_writer.h, class Writer): a non-copyable class (deleted copy ctor and
// copy-assignment) with an OUT-OF-LINE defaulted destructor. On the real repo the
// fix rewrote header + source and the translation unit recompiled cleanly; the
// deleted copy operations must not block the trivially-destructible rewrite. This
// single-TU condensation of that shape asserts the fix still applies (defaults
// the destructor on the first declaration and deletes the out-of-line def).
// Complements the plain-struct positive (hasFixFixtures/PX3026) and the
// non-trivial-member boundary (TestPX3026DoesNotFireOnNonTrivialMember).
func TestPX3026FiresWithDeletedCopyOps(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	const src = "class Writer {\n" +
		" public:\n" +
		"  Writer() {}\n" +
		"  Writer(const Writer&) = delete;\n" +
		"  Writer& operator=(const Writer&) = delete;\n" +
		"  ~Writer();\n" +
		" private:\n" +
		"  int crc_;\n" +
		"};\n" +
		"Writer::~Writer() = default;\n"
	dir := t.TempDir()
	cpp := filepath.Join(dir, "w.cpp")
	if err := os.WriteFile(cpp, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	compile, ok := cppCompileCmdForTest("w.cpp")
	if !ok {
		t.Skip("xcrun --show-sdk-path failed; cannot locate the C++ sysroot")
	}
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"` + compile + `"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, _ := runCLI("-tidy", bin, "-fix", "-checks", "PX3026", "-p", dir, cpp)
	gotB, err := os.ReadFile(cpp)
	if err != nil {
		t.Fatal(err)
	}
	got := string(gotB)
	if got == src {
		if strings.Contains(stderr, "file not found") || strings.Contains(stderr, "fatal error") {
			t.Skipf("toolchain could not parse the fixture; skipping. stderr:\n%s", stderr)
		}
		t.Fatalf("PX3026 did not fire on a non-copyable class with an out-of-line defaulted dtor (deleted copy ops must not block it):\n%s", got)
	}
	if !strings.Contains(got, "~Writer() = default") {
		t.Errorf("destructor should be defaulted on its first (in-class) declaration:\n%s", got)
	}
	if strings.Contains(got, "Writer::~Writer()") {
		t.Errorf("the out-of-line destructor definition should have been removed:\n%s", got)
	}
}

// TestDefaultFixAppliesWholeCatalog pins the EVERYDAY user path: `perfscanxx
// -fix` with NO -checks restriction. That resolves the enabled set from the whole
// catalog (all built-in + custom checks, via TidyChecksArg) rather than an
// explicit id list, then applies every fixable check that fired — a different
// selection branch from the -checks tests here (including
// TestFixAppliesMultipleChecksInOneRun, which pins the same two fixes under an
// explicit -checks). On the same PX3013 + PX3008 TU, the default run must apply
// both. Skipped when clang-tidy is absent. Validated broadly by the leveldb and
// Catch2 adversarial audits (a bare -fix over 39 / 107 TUs recompiled clean).
func TestDefaultFixAppliesWholeCatalog(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	const src = "#include <vector>\n" +
		"struct S { ~S() {} };\n" +
		"bool isEmpty(const std::vector<int>& v) { return v.size() == 0; }\n"
	dir := t.TempDir()
	cpp := filepath.Join(dir, "m.cpp")
	if err := os.WriteFile(cpp, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	compile, ok := cppCompileCmdForTest("m.cpp")
	if !ok {
		t.Skip("xcrun --show-sdk-path failed; cannot locate the C++ sysroot")
	}
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"` + compile + `"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}
	// No -checks: the whole catalog is enabled.
	_, stderr, _ := runCLI("-tidy", bin, "-fix", "-p", dir, cpp)
	gotB, err := os.ReadFile(cpp)
	if err != nil {
		t.Fatal(err)
	}
	got := string(gotB)
	if got == src {
		if strings.Contains(stderr, "file not found") || strings.Contains(stderr, "fatal error") {
			t.Skipf("toolchain could not parse the fixture; skipping. stderr:\n%s", stderr)
		}
		t.Fatalf("default -fix (whole catalog) applied nothing:\n%s", got)
	}
	if !strings.Contains(got, "= default") {
		t.Errorf("default -fix missing the PX3013 rewrite (~S() = default):\n%s", got)
	}
	if !strings.Contains(got, ".empty()") || strings.Contains(got, "size() == 0") {
		t.Errorf("default -fix missing the PX3008 rewrite (v.empty()):\n%s", got)
	}
}

// TestFixAppliesMultipleChecksInOneRun pins the multi-check compose path: a
// single -fix run over one TU that has findings from TWO different fixable checks
// must apply BOTH. This is the everyday case (a broad -fix applies every fixable
// check that fired) and the path exercised heavily during corpus validation on
// google/leveldb (23 fix-its across many checks and 15 files, all 39 TUs still
// compiled) — but every other -fix test here restricts to a single -checks id.
// Here PX3013 (~S(){} -> = default) and PX3008 (v.size()==0 -> v.empty()) fire in
// one TU; after -fix both rewrites must be present. Skipped when clang-tidy absent.
func TestFixAppliesMultipleChecksInOneRun(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	const src = "#include <vector>\n" +
		"struct S { ~S() {} };\n" +
		"bool isEmpty(const std::vector<int>& v) { return v.size() == 0; }\n"
	dir := t.TempDir()
	cpp := filepath.Join(dir, "m.cpp")
	if err := os.WriteFile(cpp, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	compile, ok := cppCompileCmdForTest("m.cpp")
	if !ok {
		t.Skip("xcrun --show-sdk-path failed; cannot locate the C++ sysroot")
	}
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"` + compile + `"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, _ := runCLI("-tidy", bin, "-fix", "-checks", "PX3013,PX3008", "-p", dir, cpp)
	gotB, err := os.ReadFile(cpp)
	if err != nil {
		t.Fatal(err)
	}
	got := string(gotB)
	if got == src {
		if strings.Contains(stderr, "file not found") || strings.Contains(stderr, "fatal error") {
			t.Skipf("toolchain could not parse the fixture; skipping. stderr:\n%s", stderr)
		}
		t.Fatalf("neither fix applied in a multi-check run:\n%s", got)
	}
	if !strings.Contains(got, "= default") {
		t.Errorf("PX3013 fix (~S() = default) missing in the multi-check run:\n%s", got)
	}
	if !strings.Contains(got, ".empty()") || strings.Contains(got, "size() == 0") {
		t.Errorf("PX3008 fix (v.empty()) missing in the multi-check run:\n%s", got)
	}
}

// TestFixLevelGatesWhichFixesApply pins the -level fix-gating contract: with -fix
// -level 1, only L1 (behavior-preserving-by-design) fix-its are applied — L2/L3
// fixes, which the tool warns can change behavior, must be withheld. A user who
// runs `perfscanxx -fix -level 1` for safe-only fixes in CI would be harmed if an
// L2 fix slipped through. The existing fixreminder_test only checks the review-
// reminder MESSAGE, not that the fixes themselves are gated. Here one TU carries a
// PX3008 finding (L1: v.size()==0 -> v.empty()) and a PX3013 finding (L2:
// ~S(){} -> = default): at -level 1 ONLY the L1 rewrite lands; at -level 2 BOTH
// do. Skipped when clang-tidy is absent.
func TestFixLevelGatesWhichFixesApply(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	const src = "#include <vector>\n" +
		"struct S { ~S() {} };\n" +
		"bool e(const std::vector<int>& v) { S s; (void)s; return v.size() == 0; }\n"
	write := func() (dir, cpp string, ok bool) {
		dir = t.TempDir()
		cpp = filepath.Join(dir, "m.cpp")
		if err := os.WriteFile(cpp, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		compile, okc := cppCompileCmdForTest("m.cpp")
		if !okc {
			return "", "", false
		}
		cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"` + compile + `"}]`
		if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir, cpp, true
	}

	// -level 1: only the L1 PX3008 fix; the L2 PX3013 fix withheld.
	dir1, cpp1, ok := write()
	if !ok {
		t.Skip("xcrun --show-sdk-path failed; cannot locate the C++ sysroot")
	}
	_, stderr1, _ := runCLI("-tidy", bin, "-fix", "-level", "1", "-p", dir1, cpp1)
	got1, err := os.ReadFile(cpp1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stderr1, "fatal error") || strings.Contains(stderr1, "file not found") {
		t.Skipf("toolchain could not parse the fixture; skipping. stderr:\n%s", stderr1)
	}
	if !strings.Contains(string(got1), ".empty()") {
		t.Errorf("-level 1 must apply the L1 PX3008 fix (v.empty()):\n%s", got1)
	}
	if strings.Contains(string(got1), "= default") {
		t.Errorf("-level 1 must NOT apply the L2 PX3013 fix (= default):\n%s", got1)
	}

	// -level 2: both the L1 and the L2 fix land.
	dir2, cpp2, _ := write()
	runCLI("-tidy", bin, "-fix", "-level", "2", "-p", dir2, cpp2)
	got2, err := os.ReadFile(cpp2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got2), ".empty()") || !strings.Contains(string(got2), "= default") {
		t.Errorf("-level 2 must apply BOTH the L1 and L2 fixes:\n%s", got2)
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
		{"PX3026", "struct S { int a; ~S(); };\nS::~S() = default;\n", "~S() = default", "S::~S()"},
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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/catalog"
	"github.com/jxsl13/perfscan/perfscanxx/internal/report"
	"github.com/jxsl13/perfscan/perfscanxx/internal/tidy"
)

// runCLI drives the real entry point and returns (stdout, stderr, exit).
func runCLI(args ...string) (string, string, int) {
	var out, errBuf bytes.Buffer
	code := run(args, &out, &errBuf)
	return out.String(), errBuf.String(), code
}

func catalogCounts() (total, fixable int) {
	for _, e := range catalog.All() {
		total++
		if e.HasFix {
			fixable++
		}
	}
	return
}

func TestListShowsEveryCheckAndSummary(t *testing.T) {
	out, _, code := runCLI("-list")
	if code != 0 {
		t.Fatalf("-list exit = %d, want 0", code)
	}
	total, fixable := catalogCounts()
	for _, e := range catalog.All() {
		if !strings.Contains(out, e.ID) {
			t.Errorf("-list output missing check %s", e.ID)
		}
	}
	// Footer must report the true fix-coverage split.
	for _, want := range []string{
		"checks", "auto-fixable", "advisory",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("-list footer missing %q", want)
		}
	}
	if !strings.Contains(out, itoa(total)) || !strings.Contains(out, itoa(fixable)) {
		t.Errorf("-list footer must mention total=%d and fixable=%d; got:\n%s", total, fixable, out)
	}
}

func TestListFixableFiltersToAutoFixOnly(t *testing.T) {
	out, _, code := runCLI("-list", "-fixable")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	for _, e := range catalog.All() {
		listed := strings.Contains(out, e.ID)
		if e.HasFix && !listed {
			t.Errorf("-fixable dropped auto-fix check %s", e.ID)
		}
		if !e.HasFix && listed {
			t.Errorf("-fixable leaked advisory check %s", e.ID)
		}
	}
}

func TestListJSONIsValidAndComplete(t *testing.T) {
	out, _, code := runCLI("-list", "-json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var got []struct {
		ID       string `json:"id"`
		Level    int    `json:"level"`
		TidyName string `json:"tidyCheck"`
		AutoFix  bool   `json:"autoFix"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("-list -json is not valid JSON: %v", err)
	}
	total, fixable := catalogCounts()
	if len(got) != total {
		t.Errorf("-json has %d entries, want %d", len(got), total)
	}
	af := 0
	for _, c := range got {
		if c.AutoFix {
			af++
		}
		if c.ID == "" || c.TidyName == "" || c.Level < 1 {
			t.Errorf("-json entry incomplete: %+v", c)
		}
	}
	if af != fixable {
		t.Errorf("-json autoFix count = %d, want %d", af, fixable)
	}

	// -fixable narrows the JSON too.
	fx, _, _ := runCLI("-list", "-json", "-fixable")
	var fxGot []struct{}
	if err := json.Unmarshal([]byte(fx), &fxGot); err != nil {
		t.Fatalf("-list -json -fixable not valid JSON: %v", err)
	}
	if len(fxGot) != fixable {
		t.Errorf("-json -fixable has %d entries, want %d", len(fxGot), fixable)
	}
}

func TestExplainBuildsCorrectUpstreamURLPerFamily(t *testing.T) {
	cases := []struct{ id, want string }{
		{"PX1001", "/checks/performance/for-range-copy.html"},
		{"PX3009", "/checks/readability/redundant-string-cstr.html"},
		{"PX2003", "/checks/modernize/use-emplace.html"},
		{"PX3015", "/checks/cppcoreguidelines/prefer-member-initializer.html"},
	}
	for _, c := range cases {
		out, _, code := runCLI("-explain", c.id)
		if code != 0 {
			t.Errorf("-explain %s exit = %d", c.id, code)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("-explain %s: want URL containing %q, got:\n%s", c.id, c.want, out)
		}
	}
	// Query-based custom check has no upstream page.
	out, _, _ := runCLI("-explain", "PX2101")
	if strings.Contains(out, "clang.llvm.org") {
		t.Errorf("-explain PX2101 should NOT print an upstream URL, got:\n%s", out)
	}
	if !strings.Contains(out, "perfscanxx-defined") {
		t.Errorf("-explain PX2101 should note it is perfscanxx-defined, got:\n%s", out)
	}
}

// TestExplainURLForEveryCheck guards the -explain doc URL for the WHOLE catalog:
// every non-custom check must point at checks/<family>/<name>.html built from its
// clang-tidy family prefix (not a hard-coded performance/ path), and every
// query-based custom check must say it's perfscanxx-defined with no upstream URL.
// This catches a new check family (or a new custom check) that -explain can't map.
func TestExplainURLForEveryCheck(t *testing.T) {
	for _, e := range catalog.All() {
		out, _, code := runCLI("-explain", e.ID)
		if code != 0 {
			t.Errorf("-explain %s exit = %d", e.ID, code)
			continue
		}
		if e.Custom {
			if strings.Contains(out, "clang.llvm.org") {
				t.Errorf("-explain %s (custom) must not print an upstream URL:\n%s", e.ID, out)
			}
			if !strings.Contains(out, "perfscanxx-defined") {
				t.Errorf("-explain %s (custom) must note it is perfscanxx-defined:\n%s", e.ID, out)
			}
			continue
		}
		family, name, ok := strings.Cut(e.TidyName, "-")
		if !ok {
			t.Errorf("%s: TidyName %q has no family prefix", e.ID, e.TidyName)
			continue
		}
		want := "/checks/" + family + "/" + name + ".html"
		if !strings.Contains(out, want) {
			t.Errorf("-explain %s: want URL containing %q, got:\n%s", e.ID, want, out)
		}
	}
}

func TestExplainUnknownCheckFails(t *testing.T) {
	_, errOut, code := runCLI("-explain", "PX9999")
	if code == 0 {
		t.Error("-explain PX9999: want non-zero exit")
	}
	if !strings.Contains(errOut, "unknown check") {
		t.Errorf("-explain PX9999: want 'unknown check' on stderr, got %q", errOut)
	}
}

// stubTidy replaces tidy.LookPath/Executor so run() can exercise the -diff
// path without a real clang-tidy, modelling clang-tidy's two-phase behavior:
//
//   - The reporting invocation (no --fix) writes exportYAML to the
//     --export-fixes destination in argv.
//   - The -diff driver's second invocation (WITH --fix) rewrites each file named
//     in fixWrites to its post-fix contents, in place — exactly as real
//     clang-tidy --fix does (already coalesced/cleaned).
//
// sawFix (if non-nil) records whether any invocation carried --fix.
func stubTidy(t *testing.T, exportYAML string, fixWrites map[string]string, sawFix *bool) func() {
	t.Helper()
	origLook, origExec := tidy.LookPath, tidy.Executor
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
	tidy.Executor = func(_ context.Context, argv []string, stdout, stderr *bytes.Buffer) (int, error) {
		fixMode := false
		for _, a := range argv {
			if a == "--fix" {
				fixMode = true
				if sawFix != nil {
					*sawFix = true
				}
			}
			if strings.HasPrefix(a, "--export-fixes=") {
				dst := strings.TrimPrefix(a, "--export-fixes=")
				if err := os.WriteFile(dst, []byte(exportYAML), 0o644); err != nil {
					return -1, err
				}
			}
		}
		if fixMode {
			for path, content := range fixWrites {
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					return -1, err
				}
			}
		}
		return 0, nil
	}
	return func() { tidy.LookPath, tidy.Executor = origLook, origExec }
}

func TestDiffFixMutuallyExclusive(t *testing.T) {
	_, errOut, code := runCLI("-diff", "-fix", "x.cpp")
	if code != 2 {
		t.Errorf("-diff -fix: exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, "mutually exclusive") {
		t.Errorf("-diff -fix: want 'mutually exclusive' on stderr, got %q", errOut)
	}
}

func TestDiffDryRunPrintsPatchLeavesFileUnchanged(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.cpp")
	const orig = "for (auto x : items) {}\n"
	if err := os.WriteFile(src, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	// Synthetic --export-fixes: replace "auto x" (offset 5, len 6).
	exportYAML := "" +
		"MainSourceFile: '" + src + "'\n" +
		"Diagnostics:\n" +
		"  - DiagnosticName: performance-for-range-copy\n" +
		"    DiagnosticMessage:\n" +
		"      Message: 'loop variable is copied'\n" +
		"      FilePath: '" + src + "'\n" +
		"      FileOffset: 5\n" +
		"      Replacements:\n" +
		"        - FilePath: '" + src + "'\n" +
		"          Offset: 5\n" +
		"          Length: 6\n" +
		"          ReplacementText: 'const auto& x'\n"

	// What the --fix invocation writes in place (the real, cleaned result).
	const fixed = "for (const auto& x : items) {}\n"
	var sawFix bool
	restore := stubTidy(t, exportYAML, map[string]string{src: fixed}, &sawFix)
	defer restore()

	out, errOut, code := runCLI("-diff", src)
	if !sawFix {
		t.Error("-diff must run clang-tidy --fix to build the preview")
	}
	if code != 1 {
		t.Errorf("-diff with a pending fix: exit = %d, want 1\nstderr: %s", code, errOut)
	}
	if !strings.Contains(out, "-for (auto x : items) {}") || !strings.Contains(out, "+for (const auto& x : items) {}") {
		t.Errorf("-diff stdout missing expected patch lines:\n%s", out)
	}
	if !strings.HasPrefix(out, "--- a/") {
		t.Errorf("-diff stdout should start with a unified-diff header, got:\n%s", out)
	}
	if !strings.Contains(errOut, "would change") {
		t.Errorf("-diff should print a summary to stderr, got %q", errOut)
	}
	// The file on disk must be byte-for-byte unchanged after -diff (restored).
	after, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != orig {
		t.Errorf("-diff left the source file modified: %q", after)
	}
}

func TestDiffNoChangesExitsZero(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "clean.cpp")
	if err := os.WriteFile(src, []byte("int main(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	restore := stubTidy(t, "", nil, nil) // empty export => no diagnostics
	defer restore()

	out, _, code := runCLI("-diff", src)
	if code != 0 {
		t.Errorf("-diff with nothing to change: exit = %d, want 0", code)
	}
	if out != "" {
		t.Errorf("-diff with nothing to change: stdout should be empty, got %q", out)
	}
}

// itoa avoids importing strconv just for the footer assertion.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// TestExpandInputs covers the Go-style path-pattern expansion (./..., a subtree,
// a concrete file) into the translation units of a compile database — including
// the "skip TUs that don't exist on disk yet (generated sources)" rule, which is
// what keeps `perfscanxx ./...` from erroring on un-generated codegen files.
func TestExpandInputs(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir()) // resolve macOS /var -> /private/var
	if err != nil {
		t.Fatal(err)
	}
	write := func(rel string) string {
		p := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("int main(){}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := write("src/a.cpp")
	b := write("src/sub/b.cpp")
	c := write("other/c.cpp")
	gen := filepath.Join(base, "src", "gen.cpp") // listed in the DB but NOT on disk

	type entry struct{ Directory, File string }
	data, err := json.Marshal([]entry{{base, a}, {base, b}, {base, c}, {base, gen}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "build", "compile_commands.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(base); err != nil {
		t.Fatal(err)
	}

	rel := func(paths []string) []string {
		out := make([]string, 0, len(paths))
		for _, p := range paths {
			r, _ := filepath.Rel(base, p)
			out = append(out, filepath.ToSlash(r))
		}
		sort.Strings(out)
		return out
	}

	cases := []struct {
		name     string
		args     []string
		buildDir string
		want     []string
	}{
		{"whole project skips the non-existent generated TU", []string{"./..."}, "", []string{"other/c.cpp", "src/a.cpp", "src/sub/b.cpp"}},
		{"subtree pattern", []string{"./src/..."}, "", []string{"src/a.cpp", "src/sub/b.cpp"}},
		{"a directory arg", []string{"other"}, "", []string{"other/c.cpp"}},
		{"a concrete file with -p", []string{"src/a.cpp"}, "build", []string{"src/a.cpp"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files, _, err := expandInputs(tc.args, tc.buildDir)
			if err != nil {
				t.Fatalf("expandInputs(%v, %q): %v", tc.args, tc.buildDir, err)
			}
			if got := rel(files); !slices.Equal(got, tc.want) {
				t.Errorf("expandInputs(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

// exportYAMLFor builds a clang-tidy --export-fixes document carrying one
// Diagnostic per (tidyName,message,offset) triple, all anchored at src. It lets
// a test synthesize an exact finding set without a real compiler.
func exportYAMLFor(src string, diags []struct {
	name, msg string
	offset    int
}) string {
	b := &strings.Builder{}
	b.WriteString("MainSourceFile: '" + src + "'\nDiagnostics:\n")
	for _, d := range diags {
		fmt.Fprintf(b, "  - DiagnosticName: %s\n", d.name)
		b.WriteString("    DiagnosticMessage:\n")
		fmt.Fprintf(b, "      Message: '%s'\n", d.msg)
		fmt.Fprintf(b, "      FilePath: '%s'\n", src)
		fmt.Fprintf(b, "      FileOffset: %d\n", d.offset)
		b.WriteString("      Replacements: []\n")
	}
	return b.String()
}

// TestBaselineRatchetSeedSuppressRegress drives the -baseline flow through main's
// run() (not just the baseline package): the first run must SEED the file and
// exit 0; a second run with the identical finding set must SUPPRESS everything
// and exit 0; a third run that adds a NEW finding must report only that one and
// exit 1. This covers the flag wiring + exit codes in main.go, which the
// baseline package's own unit tests do not exercise.
func TestBaselineRatchetSeedSuppressRegress(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.cpp")
	// Long enough that offsets 5/40 resolve to a line:col.
	const content = "void f(std::string s, std::vector<int> v) { for (auto x : v) {} }\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	blPath := filepath.Join(dir, "baseline.yaml")

	type diag = struct {
		name, msg string
		offset    int
	}
	findingA := diag{"performance-for-range-copy", "loop variable is copied", 50}
	findingB := diag{"performance-unnecessary-value-param", "parameter is passed by value", 5}

	// Run 1: baseline absent -> seed, exit 0, nothing on stdout.
	restore := stubTidy(t, exportYAMLFor(src, []diag{findingA}), nil, nil)
	out, errOut, code := runCLI("-baseline", blPath, src)
	restore()
	if code != 0 {
		t.Fatalf("seed run: exit = %d, want 0\nstderr: %s", code, errOut)
	}
	if !strings.Contains(errOut, "wrote 1 finding") {
		t.Errorf("seed run stderr should report the write, got %q", errOut)
	}
	if strings.Contains(out, "loop variable is copied") {
		t.Errorf("seed run must not report findings on stdout, got:\n%s", out)
	}
	if _, err := os.Stat(blPath); err != nil {
		t.Fatalf("seed run should have created %s: %v", blPath, err)
	}

	// Run 2: same finding set, baseline present -> fully suppressed, exit 0.
	restore = stubTidy(t, exportYAMLFor(src, []diag{findingA}), nil, nil)
	out, errOut, code = runCLI("-baseline", blPath, src)
	restore()
	if code != 0 {
		t.Fatalf("clean run: exit = %d, want 0 (all baselined)\nstderr: %s", code, errOut)
	}
	if !strings.Contains(errOut, "suppressed") {
		t.Errorf("clean run stderr should note suppression, got %q", errOut)
	}
	if strings.Contains(out, "loop variable is copied") {
		t.Errorf("clean run must suppress the baselined finding, got:\n%s", out)
	}

	// Run 3: a NEW finding appears alongside the baselined one -> only the new
	// one is reported and the exit code flips to 1 (a regression).
	restore = stubTidy(t, exportYAMLFor(src, []diag{findingA, findingB}), nil, nil)
	out, errOut, code = runCLI("-baseline", blPath, src)
	restore()
	if code != 1 {
		t.Fatalf("regression run: exit = %d, want 1\nstderr: %s", code, errOut)
	}
	if !strings.Contains(out, "parameter is passed by value") {
		t.Errorf("regression run must report the NEW finding, got:\n%s", out)
	}
	if strings.Contains(out, "loop variable is copied") {
		t.Errorf("regression run must still suppress the baselined finding, got:\n%s", out)
	}
}

// TestCountMissingHeaderErrors pins the substring/name filter that drives the
// "-cmake-build" hint: only clang-diagnostic-error findings whose message says
// "file not found" count as a missing generated header.
func TestCountMissingHeaderErrors(t *testing.T) {
	findings := []report.Finding{
		{TidyName: "clang-diagnostic-error", Message: "'gen/proto.h' file not found"},
		{TidyName: "clang-diagnostic-error", Message: "'other.h' file not found"},
		{TidyName: "clang-diagnostic-error", Message: "expected ';' after declaration"}, // error, but not a missing header
		{TidyName: "performance-for-range-copy", Message: "file not found"},             // right words, wrong check
		{TidyName: "clang-diagnostic-warning", Message: "'x.h' file not found"},         // warning, not error
	}
	if got := countMissingHeaderErrors(findings); got != 2 {
		t.Errorf("countMissingHeaderErrors = %d, want 2", got)
	}
	if got := countMissingHeaderErrors(nil); got != 0 {
		t.Errorf("countMissingHeaderErrors(nil) = %d, want 0", got)
	}
}

// TestSummarizeParseErrors covers the parse-error degradation summary: the empty
// no-op, unique-file counting (dedup), the terse vs -v listing, and the
// missing-header -> -cmake-build hint (suppressed when -cmake-build was used).
func TestSummarizeParseErrors(t *testing.T) {
	// Two findings in fileA, one in fileB -> 2 unique TUs.
	base := []report.Finding{
		{File: "/proj/a.cpp", TidyName: "clang-diagnostic-error", Message: "'gen.h' file not found"},
		{File: "/proj/a.cpp", TidyName: "performance-for-range-copy", Message: "copy"},
		{File: "/proj/b.cpp", TidyName: "clang-diagnostic-error", Message: "'gen2.h' file not found"},
	}

	t.Run("empty is a no-op", func(t *testing.T) {
		var buf bytes.Buffer
		summarizeParseErrors(&buf, nil, false, false)
		if buf.Len() != 0 {
			t.Errorf("empty input should print nothing, got %q", buf.String())
		}
	})

	t.Run("terse counts unique TUs and points at -v", func(t *testing.T) {
		var buf bytes.Buffer
		summarizeParseErrors(&buf, base, false, false)
		out := buf.String()
		if !strings.Contains(out, "2 translation unit(s) did not fully parse") {
			t.Errorf("want unique-TU count of 2, got:\n%s", out)
		}
		if !strings.Contains(out, "re-run with -v") {
			t.Errorf("terse mode should point at -v, got:\n%s", out)
		}
		if strings.Contains(out, "did not fully parse: ") {
			t.Errorf("terse mode must not list individual files, got:\n%s", out)
		}
	})

	t.Run("verbose lists each unique file", func(t *testing.T) {
		var buf bytes.Buffer
		summarizeParseErrors(&buf, base, true, false)
		out := buf.String()
		if !strings.Contains(out, "a.cpp") || !strings.Contains(out, "b.cpp") {
			t.Errorf("verbose mode should list both files, got:\n%s", out)
		}
		if strings.Contains(out, "re-run with -v") {
			t.Errorf("verbose mode should not also suggest -v, got:\n%s", out)
		}
	})

	t.Run("missing-header hint appears unless -cmake-build", func(t *testing.T) {
		var withBuf, withoutBuf bytes.Buffer
		summarizeParseErrors(&withoutBuf, base, false, false) // cmakeBuild=false -> hint
		summarizeParseErrors(&withBuf, base, false, true)     // cmakeBuild=true -> suppressed
		if !strings.Contains(withoutBuf.String(), "-cmake-build") {
			t.Errorf("missing headers without -cmake-build should hint it, got:\n%s", withoutBuf.String())
		}
		if strings.Contains(withBuf.String(), "re-run with -cmake-build") {
			t.Errorf("hint must be suppressed when -cmake-build already used, got:\n%s", withBuf.String())
		}
	})

	t.Run("no missing headers -> no hint", func(t *testing.T) {
		var buf bytes.Buffer
		noHeaders := []report.Finding{
			{File: "/proj/a.cpp", TidyName: "clang-diagnostic-error", Message: "expected ';'"},
		}
		summarizeParseErrors(&buf, noHeaders, false, false)
		if strings.Contains(buf.String(), "-cmake-build") {
			t.Errorf("no missing-header errors should not hint -cmake-build, got:\n%s", buf.String())
		}
	})
}

func TestVendoredSegment(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/proj/src/main.cpp", false},
		{"/proj/vendor/lib/x.cpp", true},
		{"/proj/third_party/re2/re2.cc", true},
		{"/proj/build/_deps/fmt-src/src/format.cc", true},
		{"/proj/test/gtest/gmock-gtest-all.cc", true},
		{"/proj/external/googletest/src/gtest.cc", true},
		{"relative/thirdparty/a.cpp", true},
		{"/proj/myvendorlib/x.cpp", false}, // substring, not a full segment -> not vendored
	}
	for _, c := range cases {
		if _, ok := vendoredSegment(c.path); ok != c.want {
			t.Errorf("vendoredSegment(%q) = %v, want %v", c.path, ok, c.want)
		}
	}
}

// TestWarnVendoredFixes covers the -fix vendored-path warning: only findings
// that actually carried a fix-it (Fixes>0) under a vendored path are reported,
// each file once; first-party and advisory-only findings never warn.
func TestWarnVendoredFixes(t *testing.T) {
	findings := []report.Finding{
		{File: "/p/src/a.cpp", Fixes: 1},                     // first-party fix -> no warn
		{File: "/p/third_party/dep/b.cpp", Fixes: 2},         // vendored + fixable -> warn
		{File: "/p/third_party/dep/b.cpp", Fixes: 1},         // same file again -> dedup
		{File: "/p/vendor/c.cpp", Fixes: 0},                  // vendored but advisory -> no warn
		{File: "/p/test/gtest/gmock-gtest-all.cc", Fixes: 3}, // bundled gtest -> warn
	}
	var buf bytes.Buffer
	warnVendoredFixes(&buf, findings)
	out := buf.String()
	if !strings.Contains(out, "2 file(s) under vendored") {
		t.Errorf("expected a warning for 2 distinct vendored files, got:\n%s", out)
	}
	if !strings.Contains(out, "b.cpp") || !strings.Contains(out, "gmock-gtest-all.cc") {
		t.Errorf("warning should name the two vendored files, got:\n%s", out)
	}
	if strings.Contains(out, "a.cpp") {
		t.Errorf("first-party file must not be warned, got:\n%s", out)
	}
	if strings.Contains(out, "c.cpp") {
		t.Errorf("advisory-only vendored file (Fixes==0) must not be warned, got:\n%s", out)
	}
	// No vendored fixes -> silent.
	var buf2 bytes.Buffer
	warnVendoredFixes(&buf2, []report.Finding{{File: "/p/src/a.cpp", Fixes: 1}})
	if buf2.Len() != 0 {
		t.Errorf("no vendored fixes should be silent, got %q", buf2.String())
	}
}

func TestSplitExcludes(t *testing.T) {
	got := splitExcludes([]string{"vendor/,third_party/", " _deps/ ", "", "gtest/"})
	want := []string{"vendor/", "third_party/", "_deps/", "gtest/"}
	if len(got) != len(want) {
		t.Fatalf("splitExcludes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("splitExcludes[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if splitExcludes(nil) != nil {
		t.Error("splitExcludes(nil) should be nil")
	}
}

func TestPathExcluded(t *testing.T) {
	ex := []string{"vendor/", "third_party/", "test/gtest/"}
	cases := []struct {
		p    string
		want bool
	}{
		{"/proj/src/main.cpp", false},
		{"/proj/vendor/lib/x.cpp", true},
		{"/proj/third_party/re2/re2.cc", true},
		{"/proj/test/gtest/gmock-gtest-all.cc", true},
		{"/proj/mytest/gtestish.cpp", false},
	}
	for _, c := range cases {
		if got := pathExcluded(c.p, ex); got != c.want {
			t.Errorf("pathExcluded(%q) = %v, want %v", c.p, got, c.want)
		}
	}
	if pathExcluded("/anything", nil) {
		t.Error("no excludes must never match")
	}
}

// TestExcludeFiltersInvocation is an integration test over run(): -exclude must
// remove matching files from the actual clang-tidy invocation (so they are
// neither analyzed nor, under -fix, rewritten). Concrete file args bypass the
// compile-database lookup, so no compile_commands.json is needed; a stub tidy
// captures the argv.
func TestExcludeFiltersInvocation(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "src", "keep.cpp")
	drop := filepath.Join(dir, "vendor", "drop.cpp")
	for _, f := range []string{keep, drop} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("int x;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var gotArgv []string
	origLook, origExec := tidy.LookPath, tidy.Executor
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
	tidy.Executor = func(_ context.Context, argv []string, stdout, stderr *bytes.Buffer) (int, error) {
		gotArgv = append([]string(nil), argv...)
		for _, a := range argv {
			if strings.HasPrefix(a, "--export-fixes=") {
				_ = os.WriteFile(strings.TrimPrefix(a, "--export-fixes="), nil, 0o644)
			}
		}
		return 0, nil
	}
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()

	// Without -exclude: both files reach clang-tidy.
	gotArgv = nil
	runCLI(keep, drop)
	base := strings.Join(gotArgv, " ")
	if !strings.Contains(base, keep) || !strings.Contains(base, drop) {
		t.Fatalf("baseline: both files should be passed, got argv: %v", gotArgv)
	}

	// With -exclude vendor/: the vendored file is dropped from the invocation.
	gotArgv = nil
	runCLI("-exclude", "vendor/", keep, drop)
	joined := strings.Join(gotArgv, " ")
	if !strings.Contains(joined, keep) {
		t.Errorf("kept file must still be passed to clang-tidy, argv: %v", gotArgv)
	}
	if strings.Contains(joined, drop) {
		t.Errorf("excluded file must NOT be passed to clang-tidy, argv: %v", gotArgv)
	}

	// Excluding every input is an error, not an empty clang-tidy run.
	gotArgv = nil
	_, errOut, code := runCLI("-exclude", "src/,vendor/", keep, drop)
	if code != 2 {
		t.Errorf("all-excluded: exit = %d, want 2", code)
	}
	if len(gotArgv) != 0 {
		t.Errorf("all-excluded: clang-tidy must not run, but got argv: %v", gotArgv)
	}
	if !strings.Contains(errOut, "removed by -exclude") {
		t.Errorf("all-excluded: want a helpful stderr message, got %q", errOut)
	}
}

// TestApplySequentialFixes verifies that -fix-sequential runs clang-tidy once
// per fixable BUILT-IN check (advisory + query-based custom checks skipped),
// each isolated to a single --checks value and carrying --fix.
func TestApplySequentialFixes(t *testing.T) {
	origLook, origExec := tidy.LookPath, tidy.Executor
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
	var checksSeen []string
	allHadFix := true
	tidy.Executor = func(_ context.Context, argv []string, _, _ *bytes.Buffer) (int, error) {
		fix := false
		for _, a := range argv {
			if a == "--fix" {
				fix = true
			}
			if strings.HasPrefix(a, "--checks=") {
				checksSeen = append(checksSeen, strings.TrimPrefix(a, "--checks="))
			}
		}
		if !fix {
			allHadFix = false
		}
		return 0, nil
	}
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()

	selected := []catalog.Entry{
		{ID: "PX1001", TidyName: "performance-for-range-copy", HasFix: true},
		{ID: "PX2002", TidyName: "performance-inefficient-string-concatenation", HasFix: false}, // advisory → skip
		{ID: "PX2101", TidyName: "custom-reserve-before-loop", Custom: true},                    // query-based → skip
		{ID: "PX3007", TidyName: "modernize-pass-by-value", HasFix: true},
		{ID: "PX3013", TidyName: "modernize-use-equals-default", HasFix: true}, // fixable but did NOT fire -> skip
	}
	// Only PX1001 and PX3007 actually fired in the report run; PX3013 is fixable
	// but produced no finding, so it gets no pass.
	fired := map[string]bool{"PX1001": true, "PX3007": true, "PX2002": true, "PX2101": true}
	base := tidy.Options{Binary: "clang-tidy", Files: []string{"x.cpp"}}
	var buf bytes.Buffer
	if err := applySequentialFixes(context.Background(), &buf, base, selected, fired, false); err != nil {
		t.Fatal(err)
	}
	// Argv builds `--checks=-*,<check>`; only fixable built-in checks that FIRED run
	// (advisory PX2002, custom PX2101, and non-firing PX3013 are skipped).
	want := []string{"-*,performance-for-range-copy", "-*,modernize-pass-by-value"}
	if len(checksSeen) != len(want) {
		t.Fatalf("ran %d passes %v, want %d %v", len(checksSeen), checksSeen, len(want), want)
	}
	for i := range want {
		if checksSeen[i] != want[i] {
			t.Errorf("pass %d checks = %q, want %q", i, checksSeen[i], want[i])
		}
	}
	if !allHadFix {
		t.Error("every sequential pass must carry --fix")
	}
	if !strings.Contains(buf.String(), "applied 2 check(s)") {
		t.Errorf("want a summary of 2 applied checks, got %q", buf.String())
	}
}

func TestExcludeHeaderRegex(t *testing.T) {
	if got := excludeHeaderRegex(nil); got != "" {
		t.Errorf("empty excludes = %q, want \"\"", got)
	}
	// Substrings are regex-escaped (the '.' and '+' are literal) and OR-joined.
	got := excludeHeaderRegex([]string{"deps/", "third_party/", "a.b+c/"})
	want := `deps/|third_party/|a\.b\+c/`
	if got != want {
		t.Errorf("excludeHeaderRegex = %q, want %q", got, want)
	}
}

// TestUnderDir pins the containment check that gates the vendored-fix warning:
// it must treat the directory boundary correctly — a mere string prefix that
// is NOT a path-component boundary ("/a/bc" under "/a/b") is NOT containment.
func TestUnderDir(t *testing.T) {
	sep := string(filepath.Separator)
	dir := sep + filepath.Join("a", "b")
	cases := []struct {
		file string
		want bool
	}{
		{dir, true},                                  // the dir itself
		{filepath.Join(dir, "c.cpp"), true},          // direct child
		{filepath.Join(dir, "c", "d.cpp"), true},     // nested child
		{sep + filepath.Join("a", "bc"), false},      // string prefix, NOT a boundary
		{sep + filepath.Join("a", "bc", "x"), false}, // deeper non-boundary prefix
		{sep + filepath.Join("a"), false},            // parent, not contained
		{sep + filepath.Join("x", "y"), false},       // unrelated
	}
	for _, tc := range cases {
		if got := underDir(tc.file, dir); got != tc.want {
			t.Errorf("underDir(%q, %q) = %v, want %v", tc.file, dir, got, tc.want)
		}
	}
}

// TestRelPathCwd pins the display-path helper: a path under the working dir is
// shown relative, but a path OUTSIDE it (whose relative form would climb via
// "..") is left absolute rather than printed as a confusing "../.." string.
func TestRelPathCwd(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Skip("cannot determine cwd")
	}
	inside := filepath.Join(wd, "a", "b.cpp")
	if got, want := relPathCwd(inside), filepath.Join("a", "b.cpp"); got != want {
		t.Errorf("relPathCwd(inside) = %q, want %q", got, want)
	}
	// A sibling of the working dir climbs via ".." -> kept absolute (fallback).
	outside := filepath.Join(filepath.Dir(wd), "some-sibling-dir", "c.cpp")
	if got := relPathCwd(outside); got != outside {
		t.Errorf("relPathCwd(outside) = %q, want it unchanged %q", got, outside)
	}
}

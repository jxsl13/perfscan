package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
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
	// A fix that carries a safety caveat is marked with ⚠ and explained by a
	// legend, matching -explain/-json/-sarif/text. The catalog has caveated
	// fixable checks (PX3015 etc.), so both must appear.
	if !strings.Contains(out, "yes ⚠") {
		t.Errorf("-list must mark a caveated fix with \"yes ⚠\"; got:\n%s", out)
	}
	if !strings.Contains(out, "⚠ = the fix carries a caveat") {
		t.Errorf("-list must print the ⚠ caveat legend when a caveated fix is shown; got:\n%s", out)
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
	// A caveated fix (PX3015 etc.) is fixable, so it appears under -fixable —
	// and must still carry the ⚠ marker + legend, since -fixable is exactly the
	// view a user consults before a bulk -fix.
	if !strings.Contains(out, "yes ⚠") || !strings.Contains(out, "⚠ = the fix carries a caveat") {
		t.Errorf("-list -fixable must still mark caveated fixes with ⚠ and print the legend; got:\n%s", out)
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
		Caveat   string `json:"caveat"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("-list -json is not valid JSON: %v", err)
	}
	// The safety caveat must reach the machine-readable output too, not just
	// -explain: a check with a Caveat in the catalog must expose it in -json
	// (PX3015 is one such fixable-but-caveated check).
	caveated := map[string]bool{}
	for _, c := range got {
		if c.Caveat != "" {
			caveated[c.ID] = true
		}
	}
	if !caveated["PX3015"] {
		t.Errorf("-json must surface the safety caveat (PX3015 has one in the catalog but it is missing from -json); got caveated=%v", caveated)
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

// TestExplainRendersForEveryCheck is the completeness backstop for the -explain
// command: TestExplainBuildsCorrectUpstreamURLPerFamily only spot-checks one ID
// per family, so a newly added check could break -explain (wrong/blank doc line,
// non-zero exit, missing title) and slip through. This drives the REAL CLI path
// (-explain <id> -> ByID -> printExplain -> DocURL) for EVERY catalog entry and
// asserts: exit 0, the ID and Title are shown, and a doc line is present that is
// EITHER the perfscanxx-defined note (custom checks) OR a family-specific upstream
// URL (built-ins) — never the generic list-page fallback, which would mean DocURL
// silently declined for a real check.
func TestExplainRendersForEveryCheck(t *testing.T) {
	for _, e := range catalog.All() {
		out, _, code := runCLI("-explain", e.ID)
		if code != 0 {
			t.Errorf("-explain %s: exit = %d, want 0\n%s", e.ID, code, out)
			continue
		}
		if !strings.Contains(out, e.ID) || !strings.Contains(out, e.Title) {
			t.Errorf("-explain %s: output missing ID or Title:\n%s", e.ID, out)
		}
		if e.Custom {
			if !strings.Contains(out, "perfscanxx-defined") {
				t.Errorf("-explain %s (custom) must note it is perfscanxx-defined:\n%s", e.ID, out)
			}
			if strings.Contains(out, "clang.llvm.org") {
				t.Errorf("-explain %s (custom) must not print an upstream URL:\n%s", e.ID, out)
			}
			continue
		}
		// Built-in: must point at its specific upstream page, not the generic
		// fallback list (which signals DocURL declined for a real check).
		if !strings.Contains(out, "clang.llvm.org/extra/clang-tidy/checks/") {
			t.Errorf("-explain %s (built-in) must print an upstream doc URL:\n%s", e.ID, out)
		}
		if strings.Contains(out, "checks/list.html") {
			t.Errorf("-explain %s (built-in) fell back to the generic check-list URL — DocURL declined for a real check:\n%s", e.ID, out)
		}
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

// TestExplainByTidyName pins printExplain's ByTidyName FALLBACK: -explain accepts
// a clang-tidy check NAME (what a user sees in output and docs), not only a PX id.
// Every existing -explain test passes a PX id, so the ByID-miss -> ByTidyName path
// was unexercised — a regression dropping the fallback would break `-explain
// performance-for-range-copy` while the PX-id suite stayed green.
func TestExplainByTidyName(t *testing.T) {
	// A clang-tidy name resolves to the same entry its PX id does.
	out, _, code := runCLI("-explain", "performance-for-range-copy")
	if code != 0 {
		t.Fatalf("-explain performance-for-range-copy exit = %d, want 0", code)
	}
	if !strings.Contains(out, "PX1001") {
		t.Errorf("-explain by tidy name must resolve to PX1001:\n%s", out)
	}
	if !strings.Contains(out, "performance-for-range-copy") {
		t.Errorf("-explain output should name the clang-tidy check:\n%s", out)
	}
	// Surrounding whitespace is trimmed before the tidy-name lookup.
	if _, _, code := runCLI("-explain", "  performance-for-range-copy  "); code != 0 {
		t.Errorf("-explain with a padded tidy name should still resolve (TrimSpace), exit = %d", code)
	}
	// A tidy-name-shaped string that matches nothing still fails cleanly (exit 2),
	// exercising the ByID-miss -> ByTidyName-miss -> error path.
	if _, _, code := runCLI("-explain", "performance-does-not-exist"); code != 2 {
		t.Errorf("-explain of an unknown tidy name: exit = %d, want 2", code)
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
	// Exit 2 specifically (the usage/config-error code), NOT 1. perfscanxx's exit
	// contract is 0 = clean, 1 = findings, 2 = usage/config error; a CI script
	// that treats exit 1 as "found perf issues" would misfire if a bad -explain id
	// returned 1. Assert the exact code, not merely non-zero.
	if code != 2 {
		t.Errorf("-explain PX9999: exit = %d, want 2 (usage error, not 1=findings)", code)
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
	// src is an absolute temp path, so the header is bare (no git a/ b/ prefix,
	// which git apply rejects on absolute paths) and never double-slashed.
	if !strings.HasPrefix(out, "--- "+src+"\n+++ "+src+"\n") {
		t.Errorf("-diff stdout should start with a bare absolute unified-diff header, got:\n%s", out)
	}
	if strings.Contains(out, "a//") || strings.Contains(out, "b//") {
		t.Errorf("-diff header must not double-slash an absolute path:\n%s", out)
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
		// A file named BOTH explicitly and via an overlapping pattern must appear
		// exactly once — the set-dedup contract. Without it a.cpp would be scanned
		// twice (double findings, double fix passes).
		{"concrete file overlapping a pattern dedups", []string{"./src/...", "src/a.cpp"}, "", []string{"src/a.cpp", "src/sub/b.cpp"}},
		// Two overlapping patterns likewise collapse to the union, no duplicates.
		{"overlapping patterns dedup to the union", []string{"./...", "./src/..."}, "", []string{"other/c.cpp", "src/a.cpp", "src/sub/b.cpp"}},
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

	// The ratchet's core safety property: a run against an EXISTING baseline
	// FILTERS, it must never REWRITE the file. If the regression run had
	// re-seeded the baseline with {A, B}, the new finding B would be silently
	// accepted and the next run would suppress it — the ratchet would leak
	// regressions. Assert the on-disk baseline still holds ONLY finding A.
	blData, err := os.ReadFile(blPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(blData), findingA.msg) {
		t.Errorf("baseline lost its seeded finding (%q) after a filter run:\n%s", findingA.msg, blData)
	}
	if strings.Contains(string(blData), findingB.msg) {
		t.Errorf("regression finding (%q) was written into the baseline — a filter run must NOT overwrite it (the ratchet would silently accept regressions):\n%s", findingB.msg, blData)
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

	// Without -exclude: both files reach clang-tidy. -j 1 pins a single
	// invocation so both files land in one argv (this test isolates exclude
	// filtering, not the parallel split covered by TestParallelReportRun).
	gotArgv = nil
	runCLI("-j", "1", keep, drop)
	base := strings.Join(gotArgv, " ")
	if !strings.Contains(base, keep) || !strings.Contains(base, drop) {
		t.Fatalf("baseline: both files should be passed, got argv: %v", gotArgv)
	}

	// With -exclude vendor/: the vendored file is dropped from the invocation.
	gotArgv = nil
	runCLI("-j", "1", "-exclude", "vendor/", keep, drop)
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

// TestExplainDocLine pins the three -explain doc-line branches: a custom check
// (no upstream page), a built-in whose TidyName yields a doc URL, and a built-in
// whose (malformed) TidyName has no family/name split so DocURL declines and the
// generic checks-list page is offered instead.
func TestExplainDocLine(t *testing.T) {
	custom := explainDocLine(catalog.Entry{Custom: true, TidyName: "custom-x"})
	if !strings.Contains(custom, "query-based") || strings.Contains(custom, "https://clang.llvm.org") {
		t.Errorf("custom explain line = %q, want the no-upstream-page message", custom)
	}
	withURL := explainDocLine(catalog.Entry{TidyName: "performance-for-range-copy"})
	if !strings.Contains(withURL, "checks/performance/for-range-copy.html") {
		t.Errorf("built-in explain line = %q, want the upstream doc URL", withURL)
	}
	noSplit := explainDocLine(catalog.Entry{TidyName: "nohyphen"})
	if !strings.Contains(noSplit, "checks/list.html") {
		t.Errorf("malformed-TidyName explain line = %q, want the checks-list fallback", noSplit)
	}
}

// TestWarnVendoredFixesTruncates pins the maxList cap: with more than 10 distinct
// vendored files, the warning lists the first 10 and summarizes the rest as
// "… and N more" rather than flooding stderr.
func TestWarnVendoredFixesTruncates(t *testing.T) {
	var findings []report.Finding
	for i := 0; i < 13; i++ {
		findings = append(findings, report.Finding{
			File:  fmt.Sprintf("/p/vendor/dep%02d/x.cpp", i),
			Fixes: 1,
		})
	}
	var buf bytes.Buffer
	warnVendoredFixes(&buf, findings)
	out := buf.String()
	if !strings.Contains(out, "13 file(s) under vendored") {
		t.Errorf("expected the 13-file count in the header, got:\n%s", out)
	}
	if !strings.Contains(out, "… and 3 more") {
		t.Errorf("expected the '… and 3 more' truncation (13-10), got:\n%s", out)
	}
	if n := strings.Count(out, "modified vendored file:"); n != 10 {
		t.Errorf("expected exactly 10 listed files, got %d:\n%s", n, out)
	}
}

// TestApplySequentialFixesVerboseAndError covers the two branches TestApplySequentialFixes
// leaves out: the verbose per-check log line, and a failing tidy.Run being wrapped
// with the check ID.
func TestApplySequentialFixesVerboseAndError(t *testing.T) {
	selected := []catalog.Entry{{ID: "PX1001", TidyName: "performance-for-range-copy", HasFix: true}}
	fired := map[string]bool{"PX1001": true}
	base := tidy.Options{Binary: "clang-tidy", Files: []string{"x.cpp"}}

	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }

	// verbose=true logs the per-check "applying" line.
	tidy.Executor = func(_ context.Context, _ []string, _, _ *bytes.Buffer) (int, error) { return 0, nil }
	var buf bytes.Buffer
	if err := applySequentialFixes(context.Background(), &buf, base, selected, fired, true); err != nil {
		t.Fatalf("applySequentialFixes: %v", err)
	}
	if !strings.Contains(buf.String(), "applying PX1001 (performance-for-range-copy)") {
		t.Errorf("verbose output missing the per-check apply line:\n%s", buf.String())
	}

	// A failing tidy.Run is wrapped with the check ID.
	tidy.Executor = func(_ context.Context, _ []string, _, _ *bytes.Buffer) (int, error) {
		return -1, errors.New("boom")
	}
	err := applySequentialFixes(context.Background(), &bytes.Buffer{}, base, selected, fired, false)
	if err == nil || !strings.Contains(err.Error(), "-fix-sequential applying PX1001") {
		t.Fatalf("expected a wrapped -fix-sequential error, got %v", err)
	}
}

// TestExpandInputsNoDBPaths covers the two database-less branches: explicit
// files with no -p need no compilation database (returned verbatim), while a
// directory/pattern arg with no resolvable database is an error.
func TestExpandInputsNoDBPaths(t *testing.T) {
	// Concrete files, no -p, no db anywhere: returned as-is, no error.
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(t.TempDir()); err != nil { // an empty dir with no compile_commands.json
		t.Fatal(err)
	}
	files, eff, err := expandInputs([]string{"a.cpp", "sub/b.cpp"}, "")
	if err != nil {
		t.Fatalf("concrete files with no -p should not error: %v", err)
	}
	if eff != "" {
		t.Errorf("effBuildDir = %q, want empty (no database consulted)", eff)
	}
	if len(files) != 2 || files[0] != "a.cpp" || files[1] != "sub/b.cpp" {
		t.Errorf("concrete files = %v, want [a.cpp sub/b.cpp] verbatim", files)
	}

	// A pattern arg with no database resolvable -> error.
	if _, _, err := expandInputs([]string{"./..."}, ""); err == nil {
		t.Error("a ./... pattern with no compilation database should error")
	}
}

// TestFlagValidationErrors pins the CLI-contract guards in run() that reject
// incoherent flag combinations before any clang-tidy work: each must print a
// specific diagnostic to stderr and exit 2 (a usage/config fatal). The -diff+-fix
// conflict is covered elsewhere (TestDiffFixMutuallyExclusive); this closes the
// remaining three, which were unexercised.
func TestFlagValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"level too low", []string{"-level", "0", "x.cpp"}, "-level must be 1, 2 or 3"},
		{"level too high", []string{"-level", "4", "x.cpp"}, "-level must be 1, 2 or 3"},
		{"fix-sequential without fix", []string{"-fix-sequential", "x.cpp"}, "-fix-sequential has no effect without -fix"},
		{"fix-errors without fix", []string{"-fix-errors", "x.cpp"}, "-fix-errors has no effect without -fix"},
		{"json and sarif together", []string{"-json", "-sarif", "x.cpp"}, "-json and -sarif are mutually exclusive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errOut, code := runCLI(tc.args...)
			if code != 2 {
				t.Errorf("exit = %d, want 2 (usage fatal)", code)
			}
			if !strings.Contains(errOut, tc.want) {
				t.Errorf("stderr = %q, want it to contain %q", errOut, tc.want)
			}
		})
	}
}

// TestExplainShowsCaveat pins the end-to-end -explain caveat contract across the
// WHOLE catalog: a check that carries a Caveat (PX3004 noexcept-move, PX3007
// pass-by-value, PX3015 member-initializer — all of which fire heavily on real
// class-heavy code, e.g. googletest) must print a "⚠ caveat:" line containing
// its caveat text, and a check WITHOUT a caveat must not. TestCaveatsAreWell-
// Formed checks the catalog struct; this checks the user-facing CLI output a
// reviewer actually sees before -fix.
func TestExplainShowsCaveat(t *testing.T) {
	sawCaveat := false
	for _, e := range catalog.All() {
		out, _, code := runCLI("-explain", e.ID)
		if code != 0 {
			t.Errorf("-explain %s exit = %d", e.ID, code)
			continue
		}
		if e.Caveat != "" {
			sawCaveat = true
			if !strings.Contains(out, "⚠ caveat:") {
				t.Errorf("-explain %s has a Caveat but no '⚠ caveat:' line:\n%s", e.ID, out)
			}
			// A distinctive slice of the caveat text must be reproduced.
			head := e.Caveat
			if len(head) > 24 {
				head = head[:24]
			}
			if !strings.Contains(out, head) {
				t.Errorf("-explain %s: caveat text %q not reproduced:\n%s", e.ID, head, out)
			}
		} else if strings.Contains(out, "⚠ caveat:") {
			t.Errorf("-explain %s has no Caveat but printed a '⚠ caveat:' line:\n%s", e.ID, out)
		}
	}
	if !sawCaveat {
		t.Fatal("no caveat'd checks found — the caveat mechanism appears unwired")
	}
}

// TestParallelReportRun pins the -j parallel analysis path: with more than one TU
// and -j >= 2, perfscanxx splits the files across concurrent clang-tidy workers
// (each analyzing a disjoint subset into its own export) and MERGES the results,
// so every file's findings still surface. An in-place -fix must NOT parallelize
// (a single pass, since parallel workers editing a shared header could race).
func TestParallelReportRun(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for _, name := range []string{"a.cpp", "b.cpp", "c.cpp", "d.cpp"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("int x;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}

	// Thread-safe stub (workers invoke it concurrently): record each analysis
	// invocation's input file and emit a one-diagnostic export for it.
	var mu sync.Mutex
	var analysisRuns int
	var seenFiles []string
	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
	tidy.Executor = func(_ context.Context, argv []string, stdout, stderr *bytes.Buffer) (int, error) {
		// The version probe (custom checks are selected by default) is not an
		// analysis invocation — answer it with a modern LLVM and don't count it.
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("Homebrew LLVM version 22.1.8\n")
			return 0, nil
		}
		var cpp, export string
		for _, a := range argv {
			if strings.HasSuffix(a, ".cpp") {
				cpp = a
			}
			if strings.HasPrefix(a, "--export-fixes=") {
				export = strings.TrimPrefix(a, "--export-fixes=")
			}
		}
		mu.Lock()
		analysisRuns++
		seenFiles = append(seenFiles, cpp)
		mu.Unlock()
		if export != "" {
			// A single performance-for-range-copy (PX1001) diagnostic in this file.
			yaml := "MainSourceFile: '" + cpp + "'\n" +
				"Diagnostics:\n" +
				"  - DiagnosticName: performance-for-range-copy\n" +
				"    DiagnosticMessage:\n" +
				"      Message: 'copy'\n" +
				"      FilePath: '" + cpp + "'\n" +
				"      FileOffset: 0\n" +
				"      Replacements: []\n"
			_ = os.WriteFile(export, []byte(yaml), 0o644)
		}
		return 0, nil
	}

	t.Run("splits across workers and merges", func(t *testing.T) {
		mu.Lock()
		analysisRuns, seenFiles = 0, nil
		mu.Unlock()
		stdout, _, code := runCLI(append([]string{"-j", "4", "-json"}, files...)...)
		if code != 1 {
			t.Fatalf("exit=%d, want 1 (findings reported)", code)
		}
		if analysisRuns != 4 {
			t.Errorf("analysis ran %d times, want 4 (one per worker); files seen: %v", analysisRuns, seenFiles)
		}
		// Every file's finding must survive the merge.
		for _, f := range files {
			if !strings.Contains(stdout, f) {
				t.Errorf("merged -json is missing a finding for %s:\n%s", f, stdout)
			}
		}
	})

	t.Run("in-place -fix stays a single invocation", func(t *testing.T) {
		mu.Lock()
		analysisRuns, seenFiles = 0, nil
		mu.Unlock()
		runCLI(append([]string{"-j", "4", "-fix"}, files...)...)
		if analysisRuns != 1 {
			t.Errorf("in-place -fix ran %d clang-tidy invocations, want 1 (parallelism must be disabled to avoid racing on shared headers); files seen: %v", analysisRuns, seenFiles)
		}
	})
}

// TestRunContextRespectsCancellation pins that the run's context reaches the
// clang-tidy invocation: with an already-cancelled context (as Ctrl-C produces),
// the analysis must abort with exit 2 — NOT report a clean run — and the
// cancellation must be observed at the invocation (so exec.CommandContext would
// kill the child rather than orphan it). The stub mimics exec.CommandContext,
// which fails immediately on a cancelled context.
func TestRunContextRespectsCancellation(t *testing.T) {
	dir := t.TempDir()
	cpp := filepath.Join(dir, "t.cpp")
	if err := os.WriteFile(cpp, []byte("int x;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"clang++ -std=c++17 -c t.cpp"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
	var sawCancelled bool
	tidy.Executor = func(ctx context.Context, argv []string, stdout, stderr *bytes.Buffer) (int, error) {
		if err := ctx.Err(); err != nil { // mimic exec.CommandContext on a cancelled ctx
			sawCancelled = true
			return -1, err
		}
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("LLVM version 22.0.0\n")
			return 0, nil
		}
		return 0, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled, as after Ctrl-C

	var out, errBuf bytes.Buffer
	code := runContext(ctx, []string{"-p", dir, cpp}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("cancelled run: exit=%d, want 2 (must not report clean); stderr:\n%s", code, errBuf.String())
	}
	if !sawCancelled {
		t.Error("the cancelled context never reached the clang-tidy invocation — child processes would be orphaned on Ctrl-C")
	}
}

// TestFixBreakdownByCheck pins the per-check breakdown a plain -fix prints after
// its summary: the applied fix-its grouped by check id (ascending) with counts,
// and a caveat marker on a caveated check (PX3007) so a reviewer knows a
// behavior-affecting fix landed. The in-place edits are otherwise invisible.
func TestFixBreakdownByCheck(t *testing.T) {
	dir := t.TempDir()
	cpp := filepath.Join(dir, "t.cpp")
	if err := os.WriteFile(cpp, []byte("int x;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"clang++ -std=c++17 -c t.cpp"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
	// Emit a synthetic export: two PX3008 fixes + one caveated PX3007 fix.
	rep := func(off int) string {
		return "      Replacements:\n        - FilePath: '" + cpp + "'\n          Offset: " +
			itoa(off) + "\n          Length: 1\n          ReplacementText: 'x'\n"
	}
	diag := func(name string, off int) string {
		return "  - DiagnosticName: " + name + "\n    DiagnosticMessage:\n      Message: 'm'\n      FilePath: '" +
			cpp + "'\n      FileOffset: " + itoa(off) + "\n" + rep(off)
	}
	export := "MainSourceFile: '" + cpp + "'\nDiagnostics:\n" +
		diag("readability-container-size-empty", 0) +
		diag("readability-container-size-empty", 2) +
		diag("modernize-pass-by-value", 4)
	tidy.Executor = func(_ context.Context, argv []string, stdout, stderr *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("LLVM version 22.0.0\n")
			return 0, nil
		}
		for _, a := range argv {
			if strings.HasPrefix(a, "--export-fixes=") {
				_ = os.WriteFile(strings.TrimPrefix(a, "--export-fixes="), []byte(export), 0o644)
			}
		}
		return 0, nil
	}

	_, errOut, _ := runCLI("-checks", "PX3008,PX3007", "-fix", "-p", dir, cpp)
	// The breakdown lines, ascending by id, with counts.
	if !strings.Contains(errOut, "PX3007 modernize-pass-by-value: 1") {
		t.Errorf("missing PX3007 breakdown line:\n%s", errOut)
	}
	if !strings.Contains(errOut, "PX3008 readability-container-size-empty: 2") {
		t.Errorf("missing PX3008 breakdown line with count 2:\n%s", errOut)
	}
	// PX3007 is caveated -> the marker must appear on its line.
	i3007 := strings.Index(errOut, "PX3007 modernize-pass-by-value: 1")
	tail := errOut[i3007:]
	if nl := strings.IndexByte(tail, '\n'); nl >= 0 {
		if !strings.Contains(tail[:nl], "⚠ caveat") {
			t.Errorf("caveated PX3007 line lacks the ⚠ caveat marker:\n%s", errOut)
		}
	}
	// PX3008 is NOT caveated -> its line must not carry the marker.
	i3008 := strings.Index(errOut, "PX3008 readability-container-size-empty: 2")
	tail8 := errOut[i3008:]
	if nl := strings.IndexByte(tail8, '\n'); nl >= 0 && strings.Contains(tail8[:nl], "caveat") {
		t.Errorf("non-caveated PX3008 line wrongly carries a caveat marker:\n%s", errOut)
	}
}

// TestReportSummaryByCheck pins the report-mode per-check tally printed after the
// finding lines: each check id (ascending) with its count and whether it is
// fixable vs advisory, plus a ⚠ on a caveated fix. Complements the -fix breakdown
// (TestFixBreakdownByCheck) for the default reporting path.
func TestReportSummaryByCheck(t *testing.T) {
	dir := t.TempDir()
	cpp := filepath.Join(dir, "t.cpp")
	if err := os.WriteFile(cpp, []byte("int x;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"clang++ -std=c++17 -c t.cpp"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
	// PX3008 x2 (fixable), PX3007 x1 (fixable+caveat), PX3021 x1 (advisory, no fix-it).
	diag := func(name string, off int, withFix bool) string {
		s := "  - DiagnosticName: " + name + "\n    DiagnosticMessage:\n      Message: 'm'\n      FilePath: '" +
			cpp + "'\n      FileOffset: " + itoa(off) + "\n"
		if withFix {
			s += "      Replacements:\n        - FilePath: '" + cpp + "'\n          Offset: " +
				itoa(off) + "\n          Length: 1\n          ReplacementText: 'x'\n"
		} else {
			s += "      Replacements: []\n"
		}
		return s
	}
	export := "MainSourceFile: '" + cpp + "'\nDiagnostics:\n" +
		diag("readability-container-size-empty", 0, true) +
		diag("readability-container-size-empty", 2, true) +
		diag("modernize-pass-by-value", 4, true) +
		diag("performance-no-int-to-ptr", 6, false)
	tidy.Executor = func(_ context.Context, argv []string, stdout, stderr *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("LLVM version 22.0.0\n")
			return 0, nil
		}
		for _, a := range argv {
			if strings.HasPrefix(a, "--export-fixes=") {
				_ = os.WriteFile(strings.TrimPrefix(a, "--export-fixes="), []byte(export), 0o644)
			}
		}
		return 0, nil
	}

	_, errOut, code := runCLI("-checks", "PX3008,PX3007,PX3021", "-p", dir, cpp)
	if code != 1 {
		t.Fatalf("exit=%d, want 1 (findings reported); stderr:\n%s", code, errOut)
	}
	for _, want := range []string{
		"PX3007 modernize-pass-by-value: 1 (fixable) ⚠",
		"PX3008 readability-container-size-empty: 2 (fixable)",
		"PX3021 performance-no-int-to-ptr: 1 (advisory)",
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("report summary missing %q:\n%s", want, errOut)
		}
	}
	// The advisory line must NOT be tagged fixable, and PX3008 (no caveat) no ⚠.
	if strings.Contains(errOut, "performance-no-int-to-ptr: 1 (fixable)") {
		t.Errorf("advisory PX3021 wrongly tagged fixable:\n%s", errOut)
	}
}

// TestSplitFiles pins the -j work distribution: round-robin (file i -> worker
// i%n), group sizes differing by at most one, and every file placed exactly once
// (no loss or duplication — a dropped file would silently skip a TU). Round-robin
// (not contiguous chunks) is what balances load when cost correlates with position
// in the compilation database.
func TestSplitFiles(t *testing.T) {
	// Round-robin layout for 5 files across 2 workers.
	got := splitFiles([]string{"a", "b", "c", "d", "e"}, 2)
	if len(got) != 2 {
		t.Fatalf("got %d groups, want 2", len(got))
	}
	if !slices.Equal(got[0], []string{"a", "c", "e"}) || !slices.Equal(got[1], []string{"b", "d"}) {
		t.Errorf("round-robin split = %v, want [[a c e] [b d]]", got)
	}

	// Property check across a range of sizes and worker counts: exactly-once
	// placement and balanced group sizes.
	for _, total := range []int{1, 2, 3, 7, 16, 100} {
		files := make([]string, total)
		for i := range files {
			files[i] = itoa(i)
		}
		for _, n := range []int{1, 2, 3, 4, 8} {
			if n > total {
				continue
			}
			groups := splitFiles(files, n)
			if len(groups) != n {
				t.Errorf("total=%d n=%d: %d groups, want %d", total, n, len(groups), n)
			}
			seen := map[string]int{}
			minSz, maxSz := total+1, -1
			for _, g := range groups {
				if len(g) < minSz {
					minSz = len(g)
				}
				if len(g) > maxSz {
					maxSz = len(g)
				}
				for _, f := range g {
					seen[f]++
				}
			}
			if len(seen) != total {
				t.Errorf("total=%d n=%d: %d distinct files placed, want %d", total, n, len(seen), total)
			}
			for f, c := range seen {
				if c != 1 {
					t.Errorf("total=%d n=%d: file %s placed %d times, want 1", total, n, f, c)
				}
			}
			if maxSz-minSz > 1 {
				t.Errorf("total=%d n=%d: group sizes span %d..%d (differ by >1)", total, n, minSz, maxSz)
			}
		}
	}
}

// TestTimeoutAbortsRun pins that -timeout bounds the analysis: with a tiny
// deadline and a clang-tidy invocation that outlives it (the stub blocks until
// the context is cancelled, as a hung clang-tidy would relative to a killing
// exec.CommandContext), the run aborts with exit 2 and the timeout message —
// never a false "clean" result.
func TestTimeoutAbortsRun(t *testing.T) {
	dir := t.TempDir()
	cpp := filepath.Join(dir, "t.cpp")
	if err := os.WriteFile(cpp, []byte("int x;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"clang++ -std=c++17 -c t.cpp"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
	tidy.Executor = func(ctx context.Context, argv []string, stdout, stderr *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("LLVM version 22.0.0\n")
			return 0, nil
		}
		// A "hung" analysis: block until the deadline cancels the context, then
		// report the cancellation the way exec.CommandContext does.
		<-ctx.Done()
		return -1, ctx.Err()
	}

	_, errOut, code := runCLI("-timeout", "20ms", "-checks", "PX3008", "-p", dir, cpp)
	if code != 2 {
		t.Fatalf("timed-out run: exit=%d, want 2; stderr:\n%s", code, errOut)
	}
	if !strings.Contains(errOut, "exceeded -timeout") {
		t.Errorf("expected the timeout message; stderr:\n%s", errOut)
	}
}

// TestTimeoutCancelsParallelWorkers pins the -timeout x -j interaction that
// TestTimeoutAbortsRun (single TU) does not reach: with several TUs and -j, the
// deadline must cancel ALL parallel workers and let runReport's wg.Wait return —
// a worker that ignored the context would deadlock the merge (and hang this test).
// Asserts the run aborts (exit 2 + timeout message) and that more than one worker
// actually entered analysis and observed the cancellation.
func TestTimeoutCancelsParallelWorkers(t *testing.T) {
	dir := t.TempDir()
	var files, entries []string
	for _, n := range []string{"a", "b", "c", "d", "e", "f"} {
		p := filepath.Join(dir, n+".cpp")
		if err := os.WriteFile(p, []byte("int x;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
		entries = append(entries, `{"directory":"`+dir+`","file":"`+p+`","command":"clang++ -std=c++17 -c `+n+`.cpp"}`)
	}
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte("["+strings.Join(entries, ",")+"]"), 0o644); err != nil {
		t.Fatal(err)
	}

	origLook, origExec := tidy.LookPath, tidy.Executor
	defer func() { tidy.LookPath, tidy.Executor = origLook, origExec }()
	tidy.LookPath = func(string) (string, error) { return "/usr/bin/clang-tidy", nil }
	var mu sync.Mutex
	var entered, cancelled int
	tidy.Executor = func(ctx context.Context, argv []string, stdout, _ *bytes.Buffer) (int, error) {
		if len(argv) >= 2 && argv[1] == "--version" {
			stdout.WriteString("LLVM version 22.0.0\n")
			return 0, nil
		}
		mu.Lock()
		entered++
		mu.Unlock()
		<-ctx.Done() // every worker hangs until the deadline cancels it
		mu.Lock()
		cancelled++
		mu.Unlock()
		return -1, ctx.Err()
	}

	_, errOut, code := runCLI(append([]string{"-timeout", "30ms", "-j", "4", "-checks", "PX3008", "-p", dir}, files...)...)
	if code != 2 || !strings.Contains(errOut, "exceeded -timeout") {
		t.Fatalf("parallel timed-out run: code=%d, want 2 + timeout message; stderr:\n%s", code, errOut)
	}
	mu.Lock()
	e, c := entered, cancelled
	mu.Unlock()
	if e < 2 {
		t.Errorf("only %d worker(s) entered analysis; expected the -j split to run >= 2 in parallel", e)
	}
	if c != e {
		t.Errorf("%d/%d workers observed cancellation — a worker ignoring the context would deadlock wg.Wait", c, e)
	}
}

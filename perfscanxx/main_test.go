package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/catalog"
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

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/catalog"
)

// TestCustomQueriesAcceptedByClangTidy empirically confirms every query-based
// custom check is a matcher clang-tidy actually accepts — the gap the pure
// structural TestCustomCheckInvariants cannot close, since a well-formed,
// balanced string can still name a matcher that does not exist (e.g. a typo
// like cxxDynmaicCastExpr). It runs the real clang-tidy custom-check engine
// over the generated config and a trivial translation unit; clang-tidy reports
// a rejected query as a [clang-tidy-config] diagnostic ("Matcher not found:
// ..."). Skipped when clang-tidy is unavailable or too old for
// --experimental-custom-checks (e.g. CI without LLVM >= 20).
func TestCustomQueriesAcceptedByClangTidy(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found; skipping empirical custom-query validation")
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
	if err := os.WriteFile(src, []byte("int main() { return 0; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Trailing `-- -std=c++17` supplies a fixed compile command so clang-tidy
	// does not fail looking for a compilation database.
	cmd := exec.Command(bin, src, "--experimental-custom-checks", "--config-file="+cfgPath, "--", "-std=c++17")
	out, _ := cmd.CombinedOutput()
	output := string(out)

	// An older clang-tidy without custom-check support cannot validate these.
	if strings.Contains(output, "Unknown command line argument") && strings.Contains(output, "experimental-custom-checks") {
		t.Skip("clang-tidy is too old for --experimental-custom-checks; skipping")
	}

	// A malformed or unknown matcher surfaces as a [clang-tidy-config]
	// diagnostic; none means every query in the live catalog parsed.
	if strings.Contains(output, "[clang-tidy-config]") {
		t.Fatalf("clang-tidy rejected a custom-check query:\n%s\n\ngenerated config:\n%s", output, cfg)
	}
}

// findClangTidyForTest resolves clang-tidy from PATH or the keg-only Homebrew
// LLVM locations, returning "" when none is present.
func findClangTidyForTest() string {
	if p, err := exec.LookPath("clang-tidy"); err == nil {
		return p
	}
	for _, p := range []string{
		"/opt/homebrew/opt/llvm/bin/clang-tidy",
		"/usr/local/opt/llvm/bin/clang-tidy",
	} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// TestEveryStandardTidyNameIsKnownToClangTidy pins the catalog's load-bearing
// claim that "every TidyName is a real clang-tidy check". A standard (non-custom)
// entry whose TidyName is a typo or a check clang-tidy has since renamed would
// SILENTLY never fire — no error, just missing findings. This runs the real
// `clang-tidy --checks=* --list-checks` once and asserts every non-custom
// TidyName is in that set. (Custom query checks are validated separately by
// TestCustomQueriesAcceptedByClangTidy; their "custom-<name>" is not listed.)
func TestEveryStandardTidyNameIsKnownToClangTidy(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found; skipping standard-TidyName validation")
	}
	out, err := exec.Command(bin, "--checks=*", "--list-checks").CombinedOutput()
	if err != nil {
		t.Fatalf("clang-tidy --list-checks: %v\n%s", err, out)
	}
	known := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			known[s] = true
		}
	}
	// Sanity: the parse found a plausible number of checks (guards against a
	// format change silently emptying the set and passing vacuously).
	if len(known) < 100 {
		t.Fatalf("parsed only %d checks from --list-checks; output format may have changed:\n%s", len(known), out)
	}
	for _, e := range catalog.All() {
		if e.Custom {
			continue
		}
		if !known[e.TidyName] {
			t.Errorf("%s: TidyName %q is not a check clang-tidy recognizes — it would silently never fire (typo, or renamed upstream)", e.ID, e.TidyName)
		}
	}
}

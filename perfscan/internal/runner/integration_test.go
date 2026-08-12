package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscan/checks"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// The integration corpus is a miniature tensor library carrying both
// generic anti-patterns and domain shapes that only fire with the
// vocabulary config — the end-to-end proof that a domain codebase is fully
// supported given the right configuration, while a config-less run stays
// generic-only (domain checks are opt-in).

const corpusGoMod = `module corpus

go 1.23
`

const corpusMain = `package main

type tensor struct{ data []float64 }

func (t *tensor) Numel() int                 { return len(t.data) }
func (t *tensor) AtF64(idx ...int) float64   { return t.data[idx[0]] }
func (t *tensor) SetF64(v float64, idx ...int) { t.data[idx[0]] = v }

func Zeros(n int) *tensor { return &tensor{data: make([]float64, n)} }

func parallelFor(n int, f func(int)) {
	for i := 0; i < n; i++ {
		f(i)
	}
}

func fanned(t *tensor) {
	parallelFor(t.Numel(), func(i int) { t.SetF64(0, i) })
}

// Domain shape: per-element dispatch in a Numel loop (PS1001).
func double(t *tensor) {
	n := t.Numel()
	for i := 0; i < n; i++ {
		t.SetF64(t.AtF64(i)*2, i)
	}
}

// Domain shape: allocator in a loop (PS2001).
func perItem(batches [][]float64, n int) {
	for range batches {
		tmp := Zeros(n)
		_ = tmp
	}
}

// Generic shape: append without prealloc (PS2101), fires with or without
// any config.
func collect(src []string) []string {
	out := []string{}
	for _, s := range src {
		out = append(out, s)
	}
	return out
}

func main() {}
`

const corpusVocab = `elementAccessors: [AtF64, SetF64]
elementCountMethods: [Numel]
allocatorFuncs: [Zeros]
fanOutHelpers: [parallelFor]
`

func runOnCorpus(t *testing.T, withConfig bool) (string, int) {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", corpusGoMod)
	write("main.go", corpusMain)
	if withConfig {
		write("perfscan.yaml", corpusVocab)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	var out, errBuf bytes.Buffer
	code := Run(checks.All(), Options{
		Patterns: []string{"./..."},
		MaxLevel: lint.LevelAggressive,
		Stdout:   &out,
		Stderr:   &errBuf,
	})
	return out.String() + errBuf.String(), code
}

func TestIntegrationDomainCorpusWithVocabulary(t *testing.T) {
	out, code := runOnCorpus(t, true)
	if code != 1 {
		t.Fatalf("want exit 1 with findings, got %d\n%s", code, out)
	}
	for _, id := range []string{"PS1001", "PS2001", "PS2101"} {
		if !strings.Contains(out, id) {
			t.Errorf("expected %s finding with vocabulary config; output:\n%s", id, out)
		}
	}
}

func TestIntegrationDomainChecksOptInWithoutConfig(t *testing.T) {
	out, code := runOnCorpus(t, false)
	if code != 1 {
		t.Fatalf("want exit 1 (generic findings remain), got %d\n%s", code, out)
	}
	// Generic finding still present…
	if !strings.Contains(out, "PS2101") {
		t.Errorf("expected generic PS2101 finding without config; output:\n%s", out)
	}
	// …domain checks silently opted out, with no warning spam.
	for _, id := range []string{"PS1001", "PS2001"} {
		if strings.Contains(out, id) {
			t.Errorf("domain check %s must not fire or warn without vocabulary; output:\n%s", id, out)
		}
	}
	if strings.Contains(out, "WARNING") {
		t.Errorf("no warning expected on a wildcard run without config; output:\n%s", out)
	}
}

// TestFixIsIdempotent pins convergence: applying -fix, then -fix again, must
// leave the file byte-identical and apply zero fixes the second time. A fix
// that re-matched itself (or introduced a finding another check re-fixes) would
// make CI oscillate; this catches that.
func TestFixIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", corpusGoMod)
	write("main.go", corpusMain)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	fixOnce := func() string {
		var out, errBuf bytes.Buffer
		Run(checks.All(), Options{
			Patterns: []string{"./..."},
			MaxLevel: lint.LevelAggressive,
			Fix:      true,
			Stdout:   &out,
			Stderr:   &errBuf,
		})
		return errBuf.String()
	}

	// First pass applies the generic PS2101 prealloc (out := []string{} grown
	// by append in a loop).
	fixOnce()
	after1, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(after1, []byte("make([]string, 0, len(")) {
		t.Fatalf("first -fix did not apply the expected PS2101 prealloc; got:\n%s", after1)
	}

	// Second pass must be a no-op — the fixed code no longer matches, so nothing
	// changes and zero fixes are applied.
	stderr2 := fixOnce()
	after2, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after1, after2) {
		t.Errorf("second -fix changed the file — not idempotent:\n--- after pass 1 ---\n%s\n--- after pass 2 ---\n%s", after1, after2)
	}
	if !strings.Contains(stderr2, "applied 0 fix(es)") {
		t.Errorf("second -fix should apply 0 fixes; stderr:\n%s", stderr2)
	}
}

// TestSyntaxErrorPackageIsSafe pins that a package which does not parse never
// crashes the runner and never modifies a file — even under -fix. go/analysis
// cannot analyze an incomplete package, so the correct behavior is to surface
// the load error and leave every file untouched; a linter that panicked or
// spliced a fix into a broken tree would be dangerous.
func TestSyntaxErrorPackageIsSafe(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const broken = "package s\n\nfunc f( {\n\tx := []string{}\n\t_ = x\n}\n"
	// A valid sibling with a genuine PS2101 site: it must NOT be fixed, because
	// the package as a whole does not type-check.
	const valid = "package s\n\nfunc g(xs []string) []string {\n\tout := []string{}\n\tfor _, x := range xs {\n\t\tout = append(out, x)\n\t}\n\treturn out\n}\n"
	write("go.mod", "module s\n\ngo 1.23\n")
	write("broken.go", broken)
	write("ok.go", valid)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	var out, errBuf bytes.Buffer
	// The call must not panic (the test fails automatically if it does).
	Run(checks.All(), Options{
		Patterns: []string{"./..."},
		MaxLevel: lint.LevelAggressive,
		Fix:      true,
		Stdout:   &out,
		Stderr:   &errBuf,
	})

	if gb, _ := os.ReadFile(filepath.Join(dir, "broken.go")); string(gb) != broken {
		t.Errorf("broken.go was modified:\n%s", gb)
	}
	if gv, _ := os.ReadFile(filepath.Join(dir, "ok.go")); string(gv) != valid {
		t.Errorf("ok.go was modified even though the package does not type-check:\n%s", gv)
	}
}

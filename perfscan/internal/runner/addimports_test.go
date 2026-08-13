package runner

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscan/checks"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// These tests pin the runner-level addReferencedStdlibImports pass: an
// import-adding check attaches its `import "slices"`-style edit to only the
// FIRST fixable finding in a file, but findings are filtered
// (//perfscan:ignore, baseline, -exclude) AFTER the checks run and BEFORE
// fixes apply — so filtering the carrier while a sibling survives used to
// leave a rewrite referencing a package that is never imported
// ("undefined: slices", a broken build).

// assertFixedCompiles writes the fixed bytes into a fresh temp module and
// runs `go build ./...` — the strongest possible assertion that -fix left a
// buildable file.
func assertFixedCompiles(t *testing.T, src []byte) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(diffGoMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.go"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("fixed file does not compile: %v\n%s\nsource:\n%s", err, out, src)
	}
}

// TestFixAddsImportWhenIgnoreDropsImportCarrier is the original repro: the
// FIRST PS3104 site (which carries the `import "slices"` edit) is suppressed
// by //perfscan:ignore, the second survives and is rewritten to slices.Sort.
// Without the runner's add-missing-import pass the result referenced slices
// without importing it and did not build.
func TestFixAddsImportWhenIgnoreDropsImportCarrier(t *testing.T) {
	const src = `package p

import "sort"

func f(a []int) {
	sort.Ints(a) //perfscan:ignore PS3104
}

func g(b []int) {
	sort.Ints(b)
}
`
	got := runFixMode(t, src)
	s := string(got)

	if !strings.Contains(s, "slices.Sort(b)") {
		t.Errorf("g should be rewritten to slices.Sort(b):\n%s", s)
	}
	if !strings.Contains(s, "sort.Ints(a)") {
		t.Errorf("f is ignore-suppressed and must keep sort.Ints(a):\n%s", s)
	}
	if !strings.Contains(s, `"slices"`) {
		t.Errorf("the rewrite references slices, so \"slices\" must be imported:\n%s", s)
	}
	if !strings.Contains(s, `"sort"`) {
		t.Errorf("f's surviving sort.Ints keeps \"sort\" live, it must not be pruned:\n%s", s)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Fatalf("fixed file does not parse: %v\n%s", err, s)
	}
	assertFixedCompiles(t, got)
}

// TestFixAddsImportWhenBaselineDropsImportCarrier pins the second filtering
// pathway to the same bug (the -exclude analog is not reproducible
// intra-file: -exclude is path-based and always drops a file's findings
// wholesale). A baseline entry with count 1 consumes the FIRST finding in
// position order — exactly the one carrying the import-add — while the
// second survives and is rewritten.
func TestFixAddsImportWhenBaselineDropsImportCarrier(t *testing.T) {
	const one = `package p

import "sort"

func f(a []int) {
	sort.Ints(a)
}
`
	const two = `package p

import "sort"

func f(a []int) {
	sort.Ints(a)
}

func g(b []int) {
	sort.Ints(b)
}
`
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", diffGoMod)
	write("p.go", one)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	baseline := filepath.Join(dir, "baseline.yaml")
	var out, errBuf bytes.Buffer
	Run(checks.All(), Options{
		Patterns:      []string{"./..."},
		MaxLevel:      lint.LevelAggressive,
		Baseline:      baseline,
		WriteBaseline: true,
		Stdout:        &out,
		Stderr:        &errBuf,
	})

	// A second sort.Ints appears; the baseline suppresses the first (the
	// import-add carrier), the new one must still be fixed AND import slices.
	write("p.go", two)
	Run(checks.All(), Options{
		Patterns: []string{"./..."},
		MaxLevel: lint.LevelAggressive,
		Baseline: baseline,
		Fix:      true,
		Stdout:   &out,
		Stderr:   &errBuf,
	})

	got, err := os.ReadFile(filepath.Join(dir, "p.go"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, "slices.Sort(b)") {
		t.Errorf("the unbaselined g should be rewritten to slices.Sort(b):\n%s", s)
	}
	if !strings.Contains(s, "sort.Ints(a)") {
		t.Errorf("f is baselined and must keep sort.Ints(a):\n%s", s)
	}
	if !strings.Contains(s, `"slices"`) {
		t.Errorf("the rewrite references slices, so \"slices\" must be imported:\n%s", s)
	}
	if !strings.Contains(s, `"sort"`) {
		t.Errorf("f's surviving sort.Ints keeps \"sort\" live:\n%s", s)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Fatalf("fixed file does not parse: %v\n%s", err, s)
	}
	assertFixedCompiles(t, got)
}

// TestFixDoesNotAddImportForLocalSlicesQualifier is the no-over-add guard: a
// LOCAL identifier named slices used as a qualifier (slices.Sort() on a
// value) must never attract an `import "slices"`, even while an unrelated
// fix (PS2107: fmt.Sprintf("%d", n) -> strconv.Itoa) rewrites the same file
// and sends it through the add pass.
func TestFixDoesNotAddImportForLocalSlicesQualifier(t *testing.T) {
	const src = `package p

import "fmt"

type sorter struct{}

func (sorter) Sort() {}

func h(n int) string {
	var slices sorter
	slices.Sort()
	return fmt.Sprintf("%d", n)
}
`
	got := runFixMode(t, src)
	s := string(got)

	if !strings.Contains(s, "strconv.Itoa(n)") {
		t.Fatalf("expected the PS2107 rewrite to strconv.Itoa (the file must go through the fix pipeline):\n%s", s)
	}
	if strings.Contains(s, `"slices"`) {
		t.Errorf("local variable slices must not attract an import \"slices\":\n%s", s)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Fatalf("fixed file does not parse: %v\n%s", err, s)
	}
	assertFixedCompiles(t, got)
}

// TestAddReferencedStdlibImports unit-tests the pass directly, including the
// pre-fix guard: a qualifier the ORIGINAL (compiling) file already used
// without importing resolves to something else (cross-file package-level
// identifier, dot import) and must not gain an import.
func TestAddReferencedStdlibImports(t *testing.T) {
	orig := []byte(`package p

import "sort"

func f(a []int) { sort.Ints(a) }

func g(b []int) { sort.Ints(b) }
`)
	fixed := []byte(`package p

import "sort"

func f(a []int) { sort.Ints(a) }

func g(b []int) { slices.Sort(b) }
`)
	got := addReferencedStdlibImports(fixed, orig)
	if !bytes.Contains(got, []byte(`"slices"`)) {
		t.Errorf("expected \"slices\" import to be added:\n%s", got)
	}
	if !bytes.Contains(got, []byte(`"sort"`)) {
		t.Errorf("\"sort\" must be left alone (still referenced):\n%s", got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("result does not parse: %v\n%s", err, got)
	}

	// Pre-existing unimported qualifier: fmt here is a package-level
	// identifier from another file of the package — untouched by any fix
	// (orig == src) — so no import may be added.
	pre := []byte(`package p

func f() { fmt.Print() }
`)
	if got := addReferencedStdlibImports(pre, pre); !bytes.Equal(got, pre) {
		t.Errorf("pre-existing unimported qualifier must not gain an import:\n%s", got)
	}

	// Nothing missing: fast path returns the input unchanged.
	clean := []byte(`package p

import "sort"

func f(a []int) { sort.Ints(a) }
`)
	if got := addReferencedStdlibImports(clean, clean); !bytes.Equal(got, clean) {
		t.Errorf("file with no missing imports must be returned unchanged:\n%s", got)
	}
}

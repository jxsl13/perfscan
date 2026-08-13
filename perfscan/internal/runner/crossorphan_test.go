package runner

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscan/checks"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// runFixMode writes src as a package into a temp module, runs the full check
// set with -fix (applying edits AND the runner's orphaned-import pruning), and
// returns the on-disk file afterward. It reuses diffGoMod from diff_test.go.
func runFixMode(t *testing.T, src string) []byte {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", diffGoMod)
	write("p.go", src)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	var out, errBuf bytes.Buffer
	Run(checks.All(), Options{
		Patterns: []string{"./..."},
		MaxLevel: lint.LevelAggressive,
		Fix:      true,
		Stdout:   &out,
		Stderr:   &errBuf,
	})
	got, err := os.ReadFile(filepath.Join(dir, "p.go"))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// TestFixPrunesCrossCheckOrphanImport pins the full-pipeline guarantee that two
// DIFFERENT checks each removing the last-but-one reference to a shared stdlib
// import must not leave it orphaned. Here fmt is used ONLY by fmt.Fprint (PS2129
// -> io.WriteString, which adds io) and fmt.Sprintf("%d", n) (PS2107 ->
// strconv.Itoa, which adds strconv): neither check drops fmt (each still sees
// the other's reference when it runs), and the runner's pruneOrphanedImports
// removes the combined orphan. This exercises the real analyzers + patchedFiles
// together, complementing the unit-level TestPruneOrphanedImports.
func TestFixPrunesCrossCheckOrphanImport(t *testing.T) {
	const src = `package p

import (
	"bytes"
	"fmt"
)

func f(buf *bytes.Buffer, s string, n int) {
	fmt.Fprint(buf, s)
	_ = fmt.Sprintf("%d", n)
}
`
	got := string(runFixMode(t, src))

	if strings.Contains(got, `"fmt"`) {
		t.Errorf("fmt import should have been pruned (orphaned by PS2129 + PS2107):\n%s", got)
	}
	for _, want := range []string{`"io"`, `"strconv"`, "io.WriteString(buf, s)", "strconv.Itoa(n)"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in the fixed file:\n%s", want, got)
		}
	}
	// A leftover orphaned import would still PARSE, but a truncated/corrupted
	// import block (co-located edits) would not — assert the result is valid Go.
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

// TestFixDedupesCrossCheckImportAdd pins that two DIFFERENT checks each ADDING
// the same import do not leave a duplicate declaration. PS3104 (sort.Ints ->
// slices.Sort) and PS3105 (sort.Sort(sort.StringSlice) -> slices.Sort) both add
// "slices"; without the runner's dedupe the file has two `import "slices"` and
// fails with "slices redeclared in this block".
func TestFixDedupesCrossCheckImportAdd(t *testing.T) {
	const src = `package p

import "sort"

func f(a []int, b []string) {
	sort.Ints(a)
	sort.Sort(sort.StringSlice(b))
}
`
	got := string(runFixMode(t, src))

	if n := strings.Count(got, `"slices"`); n != 1 {
		t.Errorf("expected exactly one \"slices\" import, got %d:\n%s", n, got)
	}
	if strings.Contains(got, `"sort"`) {
		t.Errorf("sort should have been pruned after both rewrites:\n%s", got)
	}
	if !strings.Contains(got, "slices.Sort(a)") || !strings.Contains(got, "slices.Sort(b)") {
		t.Errorf("both sorts should be rewritten to slices.Sort:\n%s", got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse (duplicate import?): %v\n%s", err, got)
	}
}

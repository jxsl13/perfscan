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

// TestFixHoistCrossFileNameCollision pins a package-scope naming hazard of the
// hoist family (PS2127/PS2132/PS2134): each seeds its fresh package-level var
// name from the ENCLOSING FUNCTION'S SOURCE LINE, so two hoists in DIFFERENT
// files of the same package whose functions start on the same line would both
// want `psRegexpL<line>`. The check's per-run `used` set spans all files in one
// Run, so the second is bumped (…_2) — without that, -fix would emit two
// `var psRegexpL5` in one package and fail with "psRegexpL5 redeclared". This
// exercises the real analyzer over a two-file package, the case single-file
// golden tests cannot reach.
func TestFixHoistCrossFileNameCollision(t *testing.T) {
	// a.go and b.go each have an inline regexp.MustCompile whose enclosing func
	// starts on the SAME line (5), forcing the name-collision path.
	const aSrc = `package p

import "regexp"

func A(s string) bool {
	return regexp.MustCompile("^a+$").MatchString(s)
}
`
	const bSrc = `package p

import "regexp"

func B(s string) bool {
	return regexp.MustCompile("^b+$").MatchString(s)
}
`
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", diffGoMod)
	write("a.go", aSrc)
	write("b.go", bSrc)

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

	aGot, err := os.ReadFile(filepath.Join(dir, "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	bGot, err := os.ReadFile(filepath.Join(dir, "b.go"))
	if err != nil {
		t.Fatal(err)
	}
	combined := string(aGot) + string(bGot)

	// Both hoisted, and the two package-level var names must be DISTINCT.
	names := map[string]int{}
	for _, ln := range strings.Split(combined, "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "var psRegexpL") {
			name := strings.Fields(ln)[1]
			names[name]++
		}
	}
	if len(names) != 2 {
		t.Errorf("expected 2 distinct hoisted var names, got %v\n--a--\n%s\n--b--\n%s", names, aGot, bGot)
	}
	for name, n := range names {
		if n != 1 {
			t.Errorf("var %s declared %d times — cross-file collision not disambiguated:\n%s", name, n, combined)
		}
	}
	// Each file must still parse (a duplicate/broken decl would not compile).
	for name, src := range map[string]string{"a.go": string(aGot), "b.go": string(bGot)} {
		if _, err := parser.ParseFile(token.NewFileSet(), name, src, 0); err != nil {
			t.Errorf("%s does not parse after fix: %v\n%s", name, err, src)
		}
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

// TestFixMaximalCrossCheckComposition stresses the whole import-machinery
// interaction at once: one file where four different checks each churn the
// import block — PS2129 (fmt.Fprint -> io.WriteString, adds io), PS2118
// (io.WriteString(w,string(b)) -> w.Write(b), removes an io ref), PS2107
// (fmt.Sprintf("%d",n) -> strconv.Itoa, adds strconv + removes the last fmt),
// PS3104 (sort.Ints -> slices.Sort, adds slices + removes the last sort). The
// combined result must add slices+strconv, keep io (still used), prune fmt+sort,
// and compile — exercising prune + dedupe + the widening together.
func TestFixMaximalCrossCheckComposition(t *testing.T) {
	const src = `package p

import (
	"bytes"
	"fmt"
	"io"
	"sort"
)

func f(buf *bytes.Buffer, b []byte, a []int, s string, n int) {
	fmt.Fprint(buf, s)
	io.WriteString(buf, string(b))
	_ = fmt.Sprintf("%d", n)
	sort.Ints(a)
}
`
	got := string(runFixMode(t, src))

	for _, gone := range []string{`"fmt"`, `"sort"`} {
		if strings.Contains(got, gone) {
			t.Errorf("%s should have been pruned:\n%s", gone, got)
		}
	}
	for _, want := range []string{`"io"`, `"slices"`, `"strconv"`,
		"io.WriteString(buf, s)", "buf.Write(b)", "strconv.Itoa(n)", "slices.Sort(a)"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in the fixed file:\n%s", want, got)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

// TestFixKeepsVersionedStdlibImport is the regression for a real -fix build break
// found during corpus validation on gonum (stat/distmv/studentst.go): a
// sort.Ints -> slices.Sort rewrite (PS3104) orphaned the adjacent "sort" import,
// and pruneOrphanedImports THEN wrongly deleted "math/rand/v2" too — because it
// inferred that import's package name from the last path segment ("v2") instead
// of its real name ("rand"), saw no `v2.` usage, and pruned a LIVE import,
// producing `undefined: rand`. The stdlib-only gate (dot-in-path) did not catch
// it because math/rand/v2 has no dot. A versioned import must be left intact.
func TestFixKeepsVersionedStdlibImport(t *testing.T) {
	const src = `package p

import (
	"math/rand/v2"
	"sort"
)

func f(xs []int) int {
	sort.Ints(xs)
	return rand.IntN(10)
}
`
	got := string(runFixMode(t, src))

	// sort was genuinely orphaned by the rewrite and must be pruned; slices added.
	if !strings.Contains(got, "slices.Sort(xs)") {
		t.Errorf("expected sort.Ints -> slices.Sort:\n%s", got)
	}
	if strings.Contains(got, `"sort"`) {
		t.Errorf(`orphaned "sort" should have been pruned:\n%s`, got)
	}
	// The live versioned import must NOT be pruned (rand is still used).
	if !strings.Contains(got, `"math/rand/v2"`) {
		t.Errorf(`"math/rand/v2" is live (rand.IntN) and must not be pruned:\n%s`, got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

func TestIsVersionSegment(t *testing.T) {
	for _, s := range []string{"v2", "v3", "v10", "v0"} {
		if !isVersionSegment(s) {
			t.Errorf("isVersionSegment(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"v", "rand", "sort", "v2x", "2", "", "version2"} {
		if isVersionSegment(s) {
			t.Errorf("isVersionSegment(%q) = true, want false", s)
		}
	}
}

// TestFixKeepsAliasedStdlibImport is the sibling regression to
// TestFixKeepsVersionedStdlibImport: both guard the "package name is not the
// last path segment" hazard in pruneOrphanedImports, but via DIFFERENT
// mechanisms. Here an ALIASED stdlib import (j "encoding/json", used as j.Marshal)
// sits next to a "sort" import that a PS3104 rewrite orphans. Pruning must keep
// the aliased import — its usage detection must honor the alias, not the inferred
// path name ("json"), which the code uses "j." for. This holds because
// astutil.UsesImport resolves the import spec's actual name; the test locks that
// so a future change to the pruner's usage check cannot silently prune live
// aliased imports (the same failure class as the math/rand/v2 bug).
func TestFixKeepsAliasedStdlibImport(t *testing.T) {
	const src = `package p

import (
	j "encoding/json"
	"sort"
)

func f(xs []int, v any) ([]byte, error) {
	sort.Ints(xs)
	return j.Marshal(v)
}
`
	got := string(runFixMode(t, src))

	if !strings.Contains(got, "slices.Sort(xs)") {
		t.Errorf("expected sort.Ints -> slices.Sort:\n%s", got)
	}
	if strings.Contains(got, `"sort"`) {
		t.Errorf(`orphaned "sort" should have been pruned:\n%s`, got)
	}
	if !strings.Contains(got, `j "encoding/json"`) {
		t.Errorf(`aliased "encoding/json" is live (j.Marshal) and must not be pruned:\n%s`, got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

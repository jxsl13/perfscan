package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/lint"
)

// TestFixSkipsExcludedFile pins the documented guarantee that -exclude drops a
// finding AND its fix: -fix must never write to a file whose slash-normalized
// path contains an -exclude substring. filterExcluded runs before the fix stage,
// so an excluded file gets no fix — this locks that end to end, with a
// NON-excluded sibling in the same run proving fixes still apply elsewhere.
//
// A normal subdirectory ("thirdparty/") is used rather than "vendor/", which Go
// tooling special-cases (go list ./... skips it) — that would make the excluded
// file untouched for the wrong reason (never analyzed, not filtered).
func TestFixSkipsExcludedFile(t *testing.T) {
	const excludedSrc = `package dep

import "sort"

func f(xs []int) { sort.Ints(xs) }
`
	const keptSrc = `package p

import "sort"

func g(xs []int) { sort.Ints(xs) }
`
	dir := t.TempDir()
	must := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("go.mod", diffGoMod)
	must("thirdparty/dep/dep.go", excludedSrc)
	must("p.go", keptSrc)

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
		Exclude:  []string{"thirdparty/"},
		Stdout:   &out,
		Stderr:   &errBuf,
	})

	gotExcluded, err := os.ReadFile(filepath.Join(dir, "thirdparty/dep/dep.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotExcluded) != excludedSrc {
		t.Errorf("excluded (thirdparty/) file was modified by -fix — it must stay byte-identical:\n%s", gotExcluded)
	}

	// The non-excluded sibling proves the run DID apply fixes (so the guarantee
	// above is over a run that actually fixes, not a no-op).
	gotKept, err := os.ReadFile(filepath.Join(dir, "p.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(gotKept, []byte("slices.Sort(xs)")) {
		t.Errorf("the non-excluded file should have been rewritten to slices.Sort:\n%s", gotKept)
	}
}

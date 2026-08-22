package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/lint"
)

// Regression: a //line directive remaps a finding's reported Position.Filename
// to an unrelated path. -fix used to bucket its (raw-offset) edits under that
// remapped name, so it wrote the edits into the WRONG file — silently
// corrupting a bystander while reporting success and leaving the real code
// untouched. patchedFiles now groups edits by the REAL token.File name, so the
// fix lands in the file the offsets actually address.
//
// The fixable site is PS3104 (sort.Ints -> slices.Sort, L1 auto-fix). The
// bystander is padded so the old code's raw offsets would fall inside it.
const lineFixMain = `package main

import "sort"

//line bystander.go:3
func work() {
	xs := []int{3, 1, 2}
	sort.Ints(xs)
	_ = xs
}

func main() { work() }
`

const lineFixBystander = `package main

// KEEP THIS LINE EXACTLY 03 aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
// KEEP THIS LINE EXACTLY 04 aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
// KEEP THIS LINE EXACTLY 05 aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
func bystander() string { return "do not touch me" }
`

func TestFixUnderLineDirectiveDoesNotCorruptOtherFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module linefix\n\ngo 1.23\n")
	write("main.go", lineFixMain)
	write("bystander.go", lineFixBystander)

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

	gotBystander, err := os.ReadFile(filepath.Join(dir, "bystander.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBystander) != lineFixBystander {
		t.Fatalf("bystander.go was corrupted by a fix that belonged to main.go:\n%s", gotBystander)
	}

	gotMain, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(gotMain, []byte("slices.Sort(xs)")) {
		t.Fatalf("the fix must land in the real file main.go (want slices.Sort), got:\n%s\nstderr:\n%s", gotMain, errBuf.String())
	}
}

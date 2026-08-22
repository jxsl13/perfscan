package runner

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestFixFasthttpSortStringsSlicesSwap pins a rewrite observed during corpus
// -fix validation on valyala/fasthttp (fs.go:1621): sort.Strings(filenames)
// became slices.Sort(filenames), and — since "sort" was imported solely for it —
// the fix swapped the import (add "slices", PRUNE the now-orphaned "sort"). The
// fasthttp root-package FS tests still PASS after the fix, confirming the
// rewrite is behavior-preserving on real code. This is the add-ONE/prune-one
// import composition (contrast the etcd PS3002 case, which adds TWO: cmp+slices).
func TestFixFasthttpSortStringsSlicesSwap(t *testing.T) {
	const src = `package p

import "sort"

func sortNames(filenames []string) {
	sort.Strings(filenames)
}
`
	got := string(runFixMode(t, src))

	if !strings.Contains(got, "slices.Sort(filenames)") {
		t.Errorf("expected sort.Strings -> slices.Sort:\n%s", got)
	}
	if strings.Contains(got, "sort.Strings") {
		t.Errorf("sort.Strings should have become slices.Sort:\n%s", got)
	}
	if !strings.Contains(got, `"slices"`) {
		t.Errorf("the slices import must be added:\n%s", got)
	}
	if strings.Contains(got, `"sort"`) {
		t.Errorf("the now-orphaned sort import must be pruned:\n%s", got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

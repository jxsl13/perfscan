package runner

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestFixEtcdSortSliceImportSwap pins a composition observed during corpus -fix
// validation (etcd tools/proto-annotations/cmd/etcd_version.go: PS3002 rewrites a
// sort.Slice on a NAMED struct type to slices.SortFunc; the tree still built).
// The file imports "sort" solely for that call, so the fix must: rewrite the
// comparator to slices.SortFunc(..., cmp.Compare(a.f, b.f)) on the concrete
// element type, ADD both "cmp" and "slices", and — crucially — PRUNE the now
// orphaned "sort" import. It exercises PS3002 + the add-two/prune-one import
// machinery + the type-aware sort-package resolution together on a real
// named-struct sort. The result must compile.
func TestFixEtcdSortSliceImportSwap(t *testing.T) {
	const src = `package p

import "sort"

type etcdVersionAnnotation struct {
	fullName string
	version  string
}

func sortAnnotations(annotations []etcdVersionAnnotation) {
	sort.Slice(annotations, func(i, j int) bool {
		return annotations[i].fullName < annotations[j].fullName
	})
}
`
	got := string(runFixMode(t, src))

	// The comparator becomes slices.SortFunc over the concrete element type,
	// with cmp.Compare on the same field (string field -> bit-identical order).
	for _, want := range []string{
		"slices.SortFunc(annotations, func(a, b etcdVersionAnnotation) int",
		"cmp.Compare(a.fullName, b.fullName)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in the fixed file:\n%s", want, got)
		}
	}
	if strings.Contains(got, "sort.Slice") {
		t.Errorf("sort.Slice should have become slices.SortFunc:\n%s", got)
	}
	// cmp and slices are added; sort is now orphaned and must be pruned.
	if !strings.Contains(got, `"cmp"`) || !strings.Contains(got, `"slices"`) {
		t.Errorf("cmp and slices imports must be added:\n%s", got)
	}
	if strings.Contains(got, `"sort"`) {
		t.Errorf("the now-orphaned sort import must be pruned:\n%s", got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}

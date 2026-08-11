package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3002 reports sort.Slice/sort.SliceStable with a comparator closure.
var PS3002 = register(&lint.Check{
	ID:       "PS3002",
	Category: "indirect",
	Slug:     "closure-comparator-sort",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "a package sort (sort.Slice/SliceStable) with a comparator closure",
		Text: `sort.Slice swaps elements through reflection and calls the
comparator closure through an interface — two indirections per comparison.
The generic slices.SortFunc (Go 1.21+) sorts the concrete slice type with a
direct call, and a sort.Sort on a concrete sort.Interface implementation
avoids the reflect-based swaps too.

Advisory (L2): the mechanical rewrite changes the comparator signature from
index-based func(i, j int) bool to element-based func(a, b T) int, which a
tool cannot do faithfully when the closure captures the slice in nontrivial
ways.`,
		Before: `sort.Slice(xs, func(i, j int) bool { return xs[i].Key < xs[j].Key })`,
		After:  `slices.SortFunc(xs, func(a, b Item) int { return cmp.Compare(a.Key, b.Key) })`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3002",
		Doc:  "sort.Slice with comparator closure",
		Run:  runPS3002,
	},
})

var sortSliceFuncs = map[string]bool{
	"Slice":       true,
	"SliceStable": true,
}

func runPS3002(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, ok := astutil.PkgFuncCall(call.Fun, "sort", sortSliceFuncs)
			if !ok {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "sort." + name + " swaps through reflection and calls its comparator indirectly; slices.SortFunc sorts the concrete type directly",
			})
			return true
		})
	}
	return nil, nil
}

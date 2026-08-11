package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3005 reports sorting an index slice with a comparator that dereferences
// the sorted element into a 2-D structure.
var PS3005 = register(&lint.Check{
	ID:       "PS3005",
	Category: "indirect",
	Slug:     "indirect-key-comparator",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "an index-slice sort whose comparator dereferences into a 2-D structure",
		Text: `sort.Slice(idx, func(a, b int) bool { return m[idx[a]][f] <
m[idx[b]][f] }) pays a row-pointer load plus an index per comparison,
O(n log n) times, for a value that depends only on the element. Fill a flat
id-indexed key column once (O(n)) and compare THAT.

The rewrite keeps the SAME PREDICATE, so the sort returns the same
permutation, ties included — this is not an argument about acceptable tie
orders. A comparator already reading a flat key (key[idx[a]]) is the fixed
form and is deliberately silent, so applying the advice clears the finding.
The flat key column is also what makes a radix pass practical afterwards.`,
		Before: `sort.Slice(idx, func(a, b int) bool {
	return m[idx[a]][f] < m[idx[b]][f]
})`,
		After: `key := make([]float64, len(m))
for i := range m {
	key[i] = m[i][f]
}
sort.Slice(idx, func(a, b int) bool { return key[idx[a]] < key[idx[b]] })`,
		MeasuredWin: "goai reference: GBM presort 1.05x (1.10x cumulative once the flat key enabled a radix pass), ball-tree median split 1.088x on KNN fit",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3005",
		Doc:  "index sort comparator dereferencing a 2-D structure",
		Run:  runPS3005,
	},
})

var sortWithComparator = map[string]bool{
	"Slice":          true,
	"SliceStable":    true,
	"SortFunc":       true,
	"SortStableFunc": true,
}

func runPS3005(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			if !sortWithComparator[callName(call)] {
				return true
			}
			sorted := baseIdentName(call.Args[0])
			if sorted == "" {
				return true
			}
			lit, ok := call.Args[1].(*ast.FuncLit)
			if !ok || lit.Type.Params == nil {
				return true
			}
			var params []string
			for _, fl := range lit.Type.Params.List {
				for _, nm := range fl.Names {
					params = append(params, nm.Name)
				}
			}
			if len(params) != 2 {
				return true
			}
			base, found := doubleIndexThrough(lit.Body, sorted, params)
			if !found {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: fmt.Sprintf("the comparator sorting %s dereferences %s[%s[…]][…] on every comparison — a row-pointer load plus an index, O(n log n) times, for a value that depends only on the element; fill a flat id-indexed key column once and compare that (same predicate, identical permutation)", sorted, base, sorted),
			})
			return true
		})
	}
	return nil, nil
}

// doubleIndexThrough reports whether body contains m[sorted[p]][…] for one
// of the comparator's parameters p. The OUTER IndexExpr is m[sorted[p]]
// indexed by f, so the sorted slice sits in the outer's own X's Index —
// getting that nesting backwards made the reference's first cut silent on
// all three sites it was written from.
func doubleIndexThrough(body *ast.BlockStmt, sorted string, params []string) (string, bool) {
	var base string
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		outer, ok := n.(*ast.IndexExpr)
		if !ok {
			return true
		}
		lookup, ok := outer.X.(*ast.IndexExpr)
		if !ok {
			return true
		}
		sel, ok := lookup.Index.(*ast.IndexExpr)
		if !ok || baseIdentName(sel.X) != sorted {
			return true
		}
		for _, p := range params {
			if exprMentions(sel.Index, p) {
				base, found = baseIdentName(lookup.X), true
				return false
			}
		}
		return true
	})
	return base, found
}

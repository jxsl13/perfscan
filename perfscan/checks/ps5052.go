package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5052 reports len(slices.Sorted(maps.Keys(m))) and its maps.Values twin —
// materializing every map key (or value) into a throwaway slice AND sorting it
// only to count it — where len(m) is that same count in O(1) with no allocation
// and no sort. The slices.Sorted sibling of PS5045 (the slices.Collect arm):
// even more wasteful, since an O(n log n) sort is thrown away on top of the
// allocation.
var PS5052 = register(&lint.Check{
	ID:       "PS5052",
	Category: "alloc",
	Slug:     "len-sorted-maps-to-len",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "len(slices.Sorted(maps.Keys(m))) allocates and sorts a slice of every key just to count it; len(m) is the identical count in O(1)",
		Text: `slices.Sorted(maps.Keys(m)) drains the maps.Keys iterator into a fresh
[]K, growing and reallocating the backing array as it fills, then sorts it in
O(n log n) — one O(len(m)) allocation, a full walk of the map, and a full sort —
and len(...) then reads the slice header. maps.Values is the same for the []V of
values. len(m) returns the identical count directly from the map header: no
iteration, no allocation, no sort.

The rewrite is BIT-IDENTICAL. Sorting permutes the slice but never changes its
length, and a map has exactly one entry per key, so the collected-and-sorted
slice has exactly len(m) elements — len(slices.Sorted(maps.Keys(m))) and
len(slices.Sorted(maps.Values(m))) both equal len(m) for every map, empty
included. slices.Sorted requires a cmp.Ordered element type and uses the builtin
total ordering (NaN included), so it is pure and cannot panic; the count is
therefore always defined. The map operand is evaluated exactly once in both
forms, so any side effect in the operand expression is preserved.

The match is deliberately narrow — it is the whole safety story:
  - the outer call is the builtin len (not a shadowing function) with one
    argument;
  - that argument is the package-level slices.Sorted (go1.23) of exactly one
    argument — the single-argument form only, never slices.SortedFunc or
    SortedStableFunc, whose user comparator could panic where len(m) would not;
  - which is the package-level maps.Keys(m) or maps.Values(m) of exactly one
    argument — the map itself. slices.Sorted of any OTHER iterator has no such
    O(1) count and never matches.
The fix keeps the len(...) and the map operand byte-verbatim and drops only the
slices.Sorted(maps.Keys( ... )) scaffolding around the map. A comment inside
that scaffolding keeps the report advisory.`,
		Before: `n := len(slices.Sorted(maps.Keys(m)))`,
		After:  `n := len(m)`,
		MeasuredWin: `On a 1024-entry map (Apple M2 Pro, go1.26): len(slices.Sorted(maps.Keys(m))) ` +
			`~55000 ns/op, 25208 B/op, 12 allocs/op vs len(m) ~0.4 ns/op, 0 B/op, 0 allocs/op ` +
			`(an O(len(m)) walk, a growing []K allocation, and an O(n log n) sort replaced by an ` +
			`O(1) header read); the saving grows without bound with the map size.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5052",
		Doc:  "len(slices.Sorted(maps.Keys/Values(m))) instead of len(m)",
		Run:  runPS5052,
	},
})

func runPS5052(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			// Outer: len(<one arg>) with len the predeclared builtin.
			lenCall, ok := n.(*ast.CallExpr)
			if !ok || len(lenCall.Args) != 1 || lenCall.Ellipsis.IsValid() {
				return true
			}
			lenIdent, ok := lenCall.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if b, ok := pass.TypesInfo.Uses[lenIdent].(*types.Builtin); !ok || b.Name() != "len" {
				return true
			}
			// Middle: slices.Sorted(<one arg>) — the single-argument form only.
			sorted, ok := lenCall.Args[0].(*ast.CallExpr)
			if !ok || len(sorted.Args) != 1 || sorted.Ellipsis.IsValid() {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, sorted.Fun, "slices", map[string]bool{"Sorted": true}); !ok {
				return true
			}
			// Inner: maps.Keys(m) or maps.Values(m).
			inner, ok := sorted.Args[0].(*ast.CallExpr)
			if !ok || len(inner.Args) != 1 || inner.Ellipsis.IsValid() {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, inner.Fun, "maps", map[string]bool{"Keys": true, "Values": true}); !ok {
				return true
			}
			m := inner.Args[0]
			diag := analysis.Diagnostic{
				Pos:     lenCall.Pos(),
				End:     lenCall.End(),
				Message: "len(slices.Sorted(maps.Keys/Values(m))) materializes and sorts a throwaway slice of every entry just to count it; len(m) is the identical count in O(1) with no allocation or sort",
			}
			// Drop the slices.Sorted(maps.Keys( ... )) scaffolding around the
			// map: delete sorted.Pos()..m.Pos() and m.End()..sorted.End(). A
			// comment in either deleted span withholds the fix.
			if !ps2111CommentIn(f, sorted.Pos(), m.Pos()) &&
				!ps2111CommentIn(f, m.End(), sorted.End()) {
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace with len(m)",
					TextEdits: []analysis.TextEdit{
						{Pos: sorted.Pos(), End: m.Pos()},
						{Pos: m.End(), End: sorted.End()},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

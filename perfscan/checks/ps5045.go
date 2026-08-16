package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5045 reports len(slices.Collect(maps.Keys(m))) and its maps.Values twin —
// materializing every map key (or value) into a throwaway slice only to count
// it — where len(m) is that same count in O(1) with no allocation. A memory
// bottleneck: the whole slice (one allocation that grows as it fills) exists
// purely to be measured.
var PS5045 = register(&lint.Check{
	ID:       "PS5045",
	Category: "alloc",
	Slug:     "len-collect-maps-to-len",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "len(slices.Collect(maps.Keys(m))) allocates a slice of every key just to count it; len(m) is the identical count in O(1)",
		Text: `slices.Collect(maps.Keys(m)) drains the maps.Keys iterator into a fresh
[]K, growing and reallocating the backing array as it fills — one O(len(m))
allocation and a full walk of the map — and len(...) then reads the slice header.
maps.Values is the same for the []V of values. len(m) returns the identical
count directly from the map header: no iteration, no allocation.

The rewrite is BIT-IDENTICAL. A map has exactly one entry per key, so the
collected slice has exactly len(m) elements — len(slices.Collect(maps.Keys(m)))
and len(slices.Collect(maps.Values(m))) both equal len(m) for every map, empty
included. The map operand is evaluated exactly once in both forms (len(m) reads
it once; the original passes it once to maps.Keys/Values), so any side effect in
the operand expression is preserved.

The match is deliberately narrow — it is the whole safety story:
  - the outer call is the builtin len (not a shadowing function) with one
    argument;
  - that argument is the package-level slices.Collect (go1.23) of exactly one
    argument;
  - which is the package-level maps.Keys(m) or maps.Values(m) of exactly one
    argument — the map itself. slices.Collect of any OTHER iterator has no such
    O(1) count and never matches.
The fix keeps the len(...) and the map operand byte-verbatim and drops only the
slices.Collect(maps.Keys( ... )) scaffolding around the map. A comment inside
that scaffolding keeps the report advisory.`,
		Before: `n := len(slices.Collect(maps.Keys(m)))`,
		After:  `n := len(m)`,
		MeasuredWin: `On a 1024-entry map (Apple M2 Pro, go1.26): len(slices.Collect(maps.Keys(m))) ` +
			`~9600 ns/op, 25208 B/op, 12 allocs/op vs len(m) ~0.4 ns/op, 0 B/op, 0 allocs/op ` +
			`(O(len(m)) walk + a growing []K allocation replaced by an O(1) header read); the ` +
			`saving grows without bound with the map size.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5045",
		Doc:  "len(slices.Collect(maps.Keys/Values(m))) instead of len(m)",
		Run:  runPS5045,
	},
})

func runPS5045(pass *analysis.Pass) (any, error) {
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
			// Middle: slices.Collect(<one arg>).
			collect, ok := lenCall.Args[0].(*ast.CallExpr)
			if !ok || len(collect.Args) != 1 || collect.Ellipsis.IsValid() {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, collect.Fun, "slices", map[string]bool{"Collect": true}); !ok {
				return true
			}
			// Inner: maps.Keys(m) or maps.Values(m).
			inner, ok := collect.Args[0].(*ast.CallExpr)
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
				Message: "len(slices.Collect(maps.Keys/Values(m))) materializes a throwaway slice of every entry just to count it; len(m) is the identical count in O(1) with no allocation",
			}
			// Drop the slices.Collect(maps.Keys( ... )) scaffolding around the
			// map: delete collect.Pos()..m.Pos() and m.End()..collect.End(). A
			// comment in either deleted span withholds the fix.
			if !ps2111CommentIn(f, collect.Pos(), m.Pos()) &&
				!ps2111CommentIn(f, m.End(), collect.End()) {
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace with len(m)",
					TextEdits: []analysis.TextEdit{
						{Pos: collect.Pos(), End: m.Pos()},
						{Pos: m.End(), End: collect.End()},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

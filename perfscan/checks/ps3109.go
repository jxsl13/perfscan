package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3109 reports a slices.BinarySearchFunc whose comparator is nothing but
// cmp.Compare on the two parameters in source order — slices.BinarySearch
// spelled the slow way — and rewrites it to slices.BinarySearch. Sibling of
// PS3107 (slices.SortFunc with a bare cmp.Compare comparator): the same
// "generic Func variant fed cmp.Compare" anti-pattern, on the binary search.
var PS3109 = register(&lint.Check{
	ID:       "PS3109",
	Category: "indirect",
	Slug:     "binarysearchfunc-cmp-compare-to-slices-binarysearch",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.BinarySearchFunc with a bare cmp.Compare comparator is slices.BinarySearch spelled the slow way",
		Text: `slices.BinarySearchFunc(x, target, func(a, b T) int { return cmp.Compare(a, b) })
searches for target exactly as slices.BinarySearch(x, target) does, but pays
an indirect call through the comparator func value on every probe (log n
probes), and that closure itself calls cmp.Compare — a second call layer.
slices.BinarySearch is a distinct monomorphized entry point that inlines the
ordered comparison. Passing cmp.Compare itself as the comparator
(slices.BinarySearchFunc(x, target, cmp.Compare)) is the same anti-pattern
minus one layer and is matched too.

The result is BIT-IDENTICAL on every input, for every ordered element type —
INCLUDING floats. slices.BinarySearch is DEFINED as BinarySearchFunc(x, target,
cmp.Compare): the two make the identical sequence of cmp.Compare probes and
return the identical (index, found) pair. Unlike the unstable slices.Sort family
(PS3107/PS3104/PS3105), a binary search has no tie-arrangement freedom to make
-0.0/+0.0 or NaN payloads observable — the result is a deterministic function of
the comparator alone — so float and named-float and type-parameter elements are
fixed here too, not held advisory.

The match is deliberately EXACT (the shared PS3107 comparator matcher). The
comparator must be cmp.Compare itself, or a func literal whose whole body is a
single return of cmp.Compare(p0, p1) with the two parameters in SOURCE ORDER — a
swapped cmp.Compare(b, a) inverts the order (a descending search over descending
data, out of scope), a field selector, a captured variable or any extra
computation fails the match silently. Both cmp and slices are resolved with type
information — only the stdlib packages match, never a shadowed local or a
same-named method — and an aliased import matches naturally. Because
cmp.Compare[E] type-checks only for a cmp.Ordered E, matching the comparator
already proves slices.BinarySearch's E cmp.Ordered constraint.

The fix replaces only the BinarySearchFunc selector name with BinarySearch and
deletes the comparator argument: the slice and target expressions are kept
VERBATIM in place (single evaluation preserved), the package qualifier keeps the
file's alias, and an explicit instantiation slices.BinarySearchFunc[S, E, T]
keeps its first two type arguments dropped to [S, E] — actually its brackets are
left as-is only when they carry no extra arg (BinarySearch has two type
parameters, not three), so an explicit instantiation stays advisory. Deleting the
comparator removes the file's cmp reference, so when the rewrites remove the
file's LAST cmp reference the fix also drops the cmp import (alias included); a
cgo file or a comment inside the deleted span keeps the report advisory. The
report fires only from go1.21 on (slices.BinarySearch and cmp.Compare appeared
there), the same gate as PS3104/PS3107.`,
		Before: `i, ok := slices.BinarySearchFunc(x, target, func(a, b int) int { return cmp.Compare(a, b) })`,
		After:  `i, ok := slices.BinarySearch(x, target)`,
		MeasuredWin: `BenchmarkPS3109 (a 4096-element sorted []int, targets cycled
across the full range so every search runs a full log n probes, Apple M2 Pro,
go1.26): ~57 ns/op before vs ~32 ns/op after, ~1.8x, 0 allocs either way. The
delta is pure comparison overhead — the indirect comparator call plus the
cmp.Compare hop inside the literal closure, on each probe, that the monomorphized
slices.BinarySearch inlines away.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3109",
		Doc:  "slices.BinarySearchFunc with a bare cmp.Compare comparator instead of slices.BinarySearch",
		Run:  runPS3109,
	},
})

func runPS3109(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.BinarySearch exists only from go1.21 on (same gate as
			// PS3104/PS3107); below that the advice is moot.
			continue
		}
		// Collect first, decide the cmp-import edit once per file: every
		// fixable call deletes exactly ONE cmp reference (the comparator's
		// cmp.Compare selector), and whether the cmp import is orphaned depends
		// on ALL of them together (same per-file collection as PS3107).
		type site struct {
			call *ast.CallExpr
			elem string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 || call.Ellipsis.IsValid() {
				return true
			}
			// An explicit instantiation slices.BinarySearchFunc[S, E, T] carries
			// THREE type arguments; slices.BinarySearch has only two, so those
			// brackets would not transfer — ps3107PkgFunc unwraps the brackets to
			// resolve the callee, and ps3109ExplicitInst below keeps such a site
			// advisory.
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "BinarySearchFunc")
			if !ok {
				return true
			}
			// The comparator must BE cmp.Compare on the parameters in source
			// order (reuses PS3107's exact matcher).
			if !ps3107BareCompare(pass, call.Args[2]) {
				return true
			}
			elem := ps3109Elem(pass, call.Args[2])
			var fix *analysis.SuggestedFix
			if !ps3109ExplicitInst(call.Fun) {
				fix = ps3109Fix(f, call, sel)
				if fix != nil {
					fixable++
				}
			}
			sites = append(sites, site{call, elem, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable call deletes exactly one cmp reference; when those are ALL
		// of the file's cmp references, the rewrites orphan the import and the fix
		// must drop it (the runner never prunes imports itself). The slices import
		// always survives: the rewritten call still spells slices.BinarySearch.
		dropCmp := fixable > 0 && pkgRefCount(pass, f, "cmp") == fixable
		importEdits, importsOK := ps3107ImportEdits(f, dropCmp)
		if !importsOK {
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			why := ""
			if st.fix == nil {
				why = " (no auto-fix: an explicit instantiation's three type arguments do not transfer to BinarySearch's two, a cgo file, or a comment in the deleted span)"
			}
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "slices.BinarySearchFunc with a bare cmp.Compare comparator pays an indirect comparator call per probe; slices.BinarySearch searches the " + st.elem + " elements with the identical (index, found) result and the comparison inlined" + why,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps3109Elem renders the searched element type (the comparator's first
// parameter type) for the message. Every type ps3107BareCompare accepts is
// cmp.Ordered and thus fixable, so there is no fixability verdict here.
func ps3109Elem(pass *analysis.Pass, comparator ast.Expr) string {
	sig, ok := pass.TypesInfo.TypeOf(comparator).(*types.Signature)
	if !ok || sig.Params().Len() != 2 {
		return "searched"
	}
	return types.TypeString(sig.Params().At(0).Type(), types.RelativeTo(pass.Pkg))
}

// ps3109ExplicitInst reports whether the callee is an explicit instantiation
// slices.BinarySearchFunc[...]: its three type arguments (S, E, T) do not
// transfer to slices.BinarySearch's two (S, E), so the mechanical rewrite would
// leave a bracket list that does not type-check — keep such a site advisory.
func ps3109ExplicitInst(fun ast.Expr) bool {
	switch ps2110Unparen(fun).(type) {
	case *ast.IndexExpr, *ast.IndexListExpr:
		return true
	}
	return false
}

// ps3109Fix builds the slices.BinarySearch(x, target) rewrite for one call, or
// nil when a comment in the deleted span would be destroyed. Only the
// BinarySearchFunc selector name and the text after the target argument (the
// comma, the comparator and the closing paren) are replaced; the slice and
// target stay untouched in place. The cmp-import edit is appended later, once
// per file.
func ps3109Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	if ps2111CommentIn(f, call.Args[1].End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + ps3107Qualifier(sel) + "BinarySearch(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("BinarySearch")},
			{Pos: call.Args[1].End(), End: call.End(), NewText: []byte(")")},
		},
	}
}

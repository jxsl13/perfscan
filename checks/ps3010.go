package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3010 reports a slices.IsSortedFunc whose comparator is nothing but
// cmp.Compare on the two parameters in source order — slices.IsSorted spelled
// the slow way — and rewrites it to slices.IsSorted. Sibling of PS3107
// (SortFunc), PS3006 (SortStableFunc), PS3109 (BinarySearchFunc) and PS3111
// (MaxFunc/MinFunc): the same "generic Func variant fed cmp.Compare"
// anti-pattern, on the sortedness scan.
var PS3010 = register(&lint.Check{
	ID:       "PS3010",
	Category: "indirect",
	Slug:     "issortedfunc-cmp-compare-to-slices-issorted",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.IsSortedFunc with a bare cmp.Compare comparator is slices.IsSorted spelled the slow way",
		Text: `slices.IsSortedFunc(s, func(a, b T) int { return cmp.Compare(a, b) })
scans s once and reports whether it is ascending — exactly what
slices.IsSorted(s) does — but pays an indirect call through the comparator func
value on EVERY adjacent pair, and that closure itself calls cmp.Compare, a
second call layer. slices.IsSorted is a distinct monomorphized entry point that
inlines the ordered comparison (cmp.Less) directly into the scan: same O(n)
single pass, same early return at the first out-of-order pair, zero comparator
indirection. Passing cmp.Compare itself as the comparator
(slices.IsSortedFunc(s, cmp.Compare)) is the same anti-pattern minus one layer
and is matched too.

The result is BIT-IDENTICAL on every input, for every ordered element type —
INCLUDING floats, unlike the sort-family siblings. slices.IsSorted is defined as
the descending scan 'if cmp.Less(x[i], x[i-1]) return false'; IsSortedFunc is
the same loop with 'cmp(x[i], x[i-1]) < 0'. With cmp = cmp.Compare,
cmp.Compare(a, b) < 0 holds iff cmp.Less(a, b) for ALL inputs of any cmp.Ordered
type: for non-NaN operands both reduce to a < b; a NaN left operand against a
non-NaN right gives true on both sides; a NaN right operand gives false on both;
NaN against NaN gives false on both; and -0.0 vs +0.0 compare equal on both. So
every iteration answers identically and the loop returns the identical bool at
the identical point. Because the output is a pure bool — there is no reordered
slice — the -0.0/NaN-payload tie concern that keeps PS3107/PS3104 advisory on
floats cannot arise: float, named-float and type-parameter elements are all
fixed here. Named element types (type Celsius int) are safe too: cmp.Compare
and cmp.Less use only the ordered operators, never a String()/Less() method.
Type validity is guaranteed by construction — cmp.Compare[E] already requires
E cmp.Ordered, exactly slices.IsSorted's constraint — and an explicit
instantiation slices.IsSortedFunc[S, E] keeps its brackets: slices.IsSorted has
the same two type parameters.

The match is deliberately EXACT (the shared PS3107 comparator matcher). The
comparator must be cmp.Compare itself, or a func literal whose whole body is a
single return of cmp.Compare(p0, p1) with the two parameters in SOURCE ORDER —
a swapped cmp.Compare(b, a) asks whether s is DESCENDING and is never matched; a
field selector, a captured variable or any extra computation fails the match
silently. Both cmp and slices are resolved with type information — only the
stdlib packages match, never a shadowed local or a same-named method — and an
aliased import matches naturally.

The fix replaces only the IsSortedFunc selector name with IsSorted and deletes
the comparator argument: the slice expression is kept VERBATIM in place (single
evaluation preserved) and the package qualifier keeps the file's alias. Deleting
the comparator removes the file's cmp reference, so when the rewrites remove the
file's LAST cmp references the fix also drops the cmp import (alias included); a
cgo file or a comment inside the deleted span keeps the report advisory. The
report fires only from go1.21 on (slices.IsSorted, slices.IsSortedFunc and
cmp.Compare all appeared there), the same gate as PS3104/PS3107.`,
		Before: `ok := slices.IsSortedFunc(s, func(a, b int) int { return cmp.Compare(a, b) })`,
		After:  `ok := slices.IsSorted(s)`,
		MeasuredWin: `BenchmarkPS3010 (a sorted 4096-element []int scanned per op —
sorted input forces the full O(n) pass, Apple M2 Pro, go1.26): ~4.83 µs/op
before vs ~1.21 µs/op after, ~4x, 0 allocs either way. The delta is pure
comparison overhead — the indirect comparator call plus the cmp.Compare hop
inside the literal closure, on each adjacent pair, that the monomorphized
slices.IsSorted inlines into a direct '<' scan.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3010",
		Doc:  "slices.IsSortedFunc with a bare cmp.Compare comparator instead of slices.IsSorted",
		Run:  runPS3010,
	},
})

func runPS3010(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.IsSorted exists only from go1.21 on (same gate as
			// PS3104/PS3107); below that the advice is moot.
			continue
		}
		// Collect first, decide the cmp-import edit once per file: every
		// fixable call deletes exactly ONE cmp reference (the comparator's
		// cmp.Compare selector), and whether the cmp import is orphaned
		// depends on ALL of them together (same per-file collection as
		// PS3107/PS3111).
		type site struct {
			call *ast.CallExpr
			elem string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			// The callee must be the receiver-less package function
			// slices.IsSortedFunc, resolved through type info — never a
			// shadowed slices or a same-named method. An explicit
			// instantiation slices.IsSortedFunc[S, E] is unwrapped; its
			// brackets survive the fix (slices.IsSorted has the same two
			// type parameters, and the matched cmp.Compare already proves
			// E cmp.Ordered).
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "IsSortedFunc")
			if !ok {
				return true
			}
			// The comparator must BE cmp.Compare on the parameters in
			// source order (reuses PS3107's exact matcher). Every element
			// type this accepts is cmp.Ordered and fixable — the bool
			// result has no tie arrangement that could expose float bit
			// patterns, so unlike PS3107/PS3111 there is no advisory
			// element class.
			if !ps3107BareCompare(pass, call.Args[1]) {
				return true
			}
			elem := ps3010Elem(pass, call.Args[1])
			fix := ps3010Fix(f, call, sel)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, elem, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable call deletes exactly one cmp reference; when those
		// are ALL of the file's cmp references, the rewrites orphan the
		// import and the fix must drop it (the runner never prunes imports
		// itself). The slices import always survives: the rewritten call
		// still spells slices.IsSorted through the same qualifier.
		dropCmp := fixable > 0 && pkgRefCount(pass, f, "cmp") == fixable
		importEdits, importsOK := ps3107ImportEdits(f, dropCmp)
		if !importsOK {
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edit (same convention as
			// PS3107/PS3111).
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
				why = " (no auto-fix: a cgo file or a comment in the deleted span)"
			}
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "slices.IsSortedFunc with a bare cmp.Compare comparator pays an indirect comparator call per adjacent pair; slices.IsSorted answers the identical bool over the " + st.elem + " elements with the comparison inlined" + why,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps3010Elem renders the scanned element type (the comparator's first
// parameter type) for the message. Every type ps3107BareCompare accepts is
// cmp.Ordered and fixable — the identical-bool argument holds for integers,
// strings, floats (NaN and ±0.0 included) and type parameters alike — so
// there is no fixability verdict here (same shape as ps3109Elem).
func ps3010Elem(pass *analysis.Pass, comparator ast.Expr) string {
	sig, ok := pass.TypesInfo.TypeOf(comparator).(*types.Signature)
	if !ok || sig.Params().Len() != 2 {
		return "scanned"
	}
	return types.TypeString(sig.Params().At(0).Type(), types.RelativeTo(pass.Pkg))
}

// ps3010Fix builds the slices.IsSorted(s) rewrite for one call, or nil when a
// comment in the deleted span would be destroyed. Only the IsSortedFunc
// selector name and the text after the slice argument (the comma, the whole
// comparator and the closing paren) are replaced; the slice stays untouched in
// place and explicit instantiation brackets survive (slices.IsSorted has the
// same two type parameters). The cmp-import edit is appended later, once per
// file.
func ps3010Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	if ps2111CommentIn(f, call.Args[0].End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + ps3107Qualifier(sel) + "IsSorted(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("IsSorted")},
			{Pos: call.Args[0].End(), End: call.End(), NewText: []byte(")")},
		},
	}
}

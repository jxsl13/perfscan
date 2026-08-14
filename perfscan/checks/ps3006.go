package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3006 reports a slices.SortStableFunc whose comparator is nothing but
// cmp.Compare on the two parameters in source order — slices.Sort spelled
// the slow, STABLE way — and rewrites it to slices.Sort. Sibling of PS3107
// (the same bare-cmp.Compare comparator under the unstable SortFunc):
// PS3006 additionally drops the stable algorithm's merge overhead, which
// buys a strictly ordered tie arrangement that is unobservable for the
// element types the fix accepts.
var PS3006 = register(&lint.Check{
	ID:       "PS3006",
	Category: "indirect",
	Slug:     "sortstablefunc-cmp-compare-to-slices-sort",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.SortStableFunc with a bare cmp.Compare(a, b) comparator is slices.Sort spelled the slow, stable way",
		Text: `slices.SortStableFunc(s, func(a, b T) int { return cmp.Compare(a, b) })
sorts ascending — exactly what slices.Sort(s) does — but pays TWICE for
it. First, every comparison dispatches through the comparator func value
(plus the cmp.Compare hop inside the closure). Second, SortStableFunc
runs the STABLE algorithm — insertion-sorted runs merged by a
rotation-based symMerge — which does strictly more work than pdqsort to
preserve the relative order of equal elements. slices.Sort is a distinct
monomorphized entry point: unstable pdqsort with the ordered '<' inlined
directly, zero comparator indirection. The rewrite therefore drops BOTH
costs. Passing cmp.Compare itself as the comparator
(slices.SortStableFunc(s, cmp.Compare)) is the same anti-pattern minus
one layer and is matched too.

The result is IDENTICAL for the element types the fix accepts.
cmp.Compare(a, b) induces exactly the order slices.Sort uses: slices.Sort
is defined via cmp.Less, and cmp.Compare(a, b) < 0 iff cmp.Less(a, b), so
the non-tie permutation is the same on both sides. Stability is the only
remaining difference, and the fix is offered only when the element type's
underlying type is an integer or string kind — there any two elements
that compare equal under cmp.Compare are indistinguishable values
(integers bitwise so; equal strings are interchangeable in every safe-Go
operation), so the stable sort's tie-order guarantee buys nothing
observable and the unstable sort produces an identical output slice.
This is the same reasoning AUTHORING's bit-identical bar spells out for
sort.Stable(sort.IntSlice(x)) -> slices.Sort(x). Named element types
(type Celsius int) are fixed too: sorting orders by the ordered value, a
String()/Error() method on the element is never consulted, and equal
values remain identical.

FLOAT elements are reported ADVISORY only, never auto-fixed — the same
policy as PS3107 and the whole PS3002/PS3104/PS3105 family, and it bites
HARDER here: -0.0 and +0.0 compare equal but differ in bits, as do NaNs
with distinct payloads, and SortStableFunc contractually KEEPS such ties
in their original order while the unstable rewrite may arrange them
either way. (The overall ORDER is still identical — cmp.Compare and
slices.Sort both put NaNs first — which is why the report still fires as
advice.) A TYPE-PARAMETER element is likewise advisory: its
instantiations may include floats.

The match is deliberately EXACT — the same ps3107 matcher. The
comparator must be a func literal whose body is a single return of
cmp.Compare(p0, p1) with the two parameters in SOURCE ORDER — a swapped
cmp.Compare(b, a) is a descending sort and is never matched — or
cmp.Compare itself passed as the func value. The arguments must be the
bare parameters, resolved by object identity: a field selector
(cmp.Compare(a.f, b.f)), a captured outer variable, any extra statement
or arithmetic in the body, or a comparator that is anything but a fresh
literal / cmp.Compare itself all fail the match silently. Both cmp and
slices are resolved with type information — only the stdlib packages
match, never a shadowed local or a same-named method — and an aliased
import matches naturally.

The fix replaces only the SortStableFunc selector name with Sort and
deletes the comparator argument: the target slice expression is kept
VERBATIM in place (single evaluation preserved), the package qualifier
keeps whatever alias the file used, and an explicit instantiation
slices.SortStableFunc[S, E](...) keeps its brackets — slices.Sort has
the same two type parameters, so slices.Sort[S, E](s) still compiles.
Deleting the comparator removes the file's cmp reference, so when the
rewrites remove the file's LAST cmp references the fix also drops the
cmp import (alias included); a cgo file (whose import block must not be
edited) or a comment inside the deleted span keeps the report advisory.

The report only fires when the effective language version is at least
go1.21 (slices.SortStableFunc, slices.Sort and cmp.Compare all appeared
there) — the same gate PS3104/PS3105/PS3107 apply; in practice code
containing this pattern already compiles only on go1.21+.`,
		Before: `slices.SortStableFunc(s, func(a, b int) int { return cmp.Compare(a, b) })`,
		After:  `slices.Sort(s)`,
		MeasuredWin: `BenchmarkPS3006 (a shuffled 4096-element []int copied and
sorted per op, Apple M2 Pro, go1.26): 497 µs/op before vs 143 µs/op
after — about 3.5x — 0 allocs either way. The delta stacks the two
savings: the indirect comparator call (plus the cmp.Compare hop) per
comparison that the monomorphized slices.Sort inlines away (the ~1.6x
PS3107 share: 237 µs vs 136 µs on the same input) and the stable
insertion-run/symMerge machinery that the unstable pdqsort skips
entirely (the rest).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3006",
		Doc:  "slices.SortStableFunc with a bare cmp.Compare(a, b) comparator instead of slices.Sort",
		Run:  runPS3006,
	},
})

func runPS3006(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.Sort exists only from go1.21 on; below that the
			// advice is moot, so stay silent entirely (same gate as
			// PS3104/PS3105/PS3107).
			continue
		}
		// Collect first, decide the cmp-import edit once per file: every
		// fixable call deletes exactly ONE cmp reference (the comparator's
		// cmp.Compare selector), and whether the cmp import is orphaned
		// depends on ALL of them together (same per-file site collection
		// as PS3104/PS3105/PS3107).
		type site struct {
			call *ast.CallExpr
			elem string // rendered element type, for the message
			why  string // non-empty: advisory-by-design reason suffix
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
			// slices.SortStableFunc, resolved through type info — never a
			// shadowed slices or a same-named method. An explicit
			// instantiation slices.SortStableFunc[S, E] is unwrapped; its
			// brackets survive the fix (slices.Sort has the same two type
			// parameters).
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "SortStableFunc")
			if !ok {
				return true
			}
			// The comparator must BE cmp.Compare on the parameters in
			// source order — a fresh literal wrapping it, or cmp.Compare
			// itself — or the call is not an ascending slices.Sort and is
			// never reported. (Shared matcher with PS3107.)
			if !ps3107BareCompare(pass, call.Args[1]) {
				return true
			}
			elem, fixableElem, why := ps3107Elem(pass, call.Args[1])
			var fix *analysis.SuggestedFix
			if fixableElem {
				fix = ps3107Fix(f, call, sel)
				if fix != nil {
					fixable++
				}
			}
			sites = append(sites, site{call, elem, why, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable call deletes exactly one cmp reference; when those
		// are ALL of the file's cmp references, the rewrites orphan the
		// import and the fix must drop it (the runner never prunes imports
		// itself). The slices import always survives: the rewritten call
		// still spells slices.Sort through the same qualifier.
		dropCmp := fixable > 0 && pkgRefCount(pass, f, "cmp") == fixable
		importEdits, importsOK := ps3107ImportEdits(f, dropCmp)
		if !importsOK {
			// cgo file needing import surgery, or a cmp spec we could not
			// locate: keep every report advisory.
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edit (same convention as
			// PS3104/PS3105/PS3107).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			msg := "slices.SortStableFunc with a bare cmp.Compare(a, b) comparator pays an indirect comparator call per comparison plus the stable sort's merge overhead; slices.Sort sorts the " + st.elem + " elements with the identical ascending order, the comparison inlined and no stability cost" + st.why
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: msg,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

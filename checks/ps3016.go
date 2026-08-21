package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3016 reports a slices.SortedStableFunc whose comparator is nothing but
// cmp.Compare on the two parameters in source order — slices.Sorted spelled
// the slow, STABLE way — and rewrites it to slices.Sorted. It is exactly the
// union of its two siblings: PS3012 (SortedFunc's comparator indirection on
// the iterator-collecting sort) and PS3006 (SortStableFunc's stable-merge
// overhead on the eager sort) — the rewrite drops BOTH costs at once.
var PS3016 = register(&lint.Check{
	ID:       "PS3016",
	Category: "indirect",
	Slug:     "sortedstablefunc-cmp-compare-to-slices-sorted",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.SortedStableFunc with a bare cmp.Compare comparator is slices.Sorted spelled the slow, stable way",
		Text: `slices.SortedStableFunc(seq, func(a, b T) int { return cmp.Compare(a, b) })
collects the iter.Seq into a fresh slice and sorts it ascending — exactly
what slices.Sorted(seq) does — but pays TWICE for it. First, every
comparison dispatches through the comparator func value (plus the
cmp.Compare hop inside the closure). Second, SortedStableFunc runs the
STABLE algorithm — insertion-sorted runs merged by a rotation-based
symMerge — which does strictly more work than pdqsort to preserve the
relative order of equal elements. slices.Sorted is a distinct
monomorphized entry point: Collect then unstable pdqsort with the
ordered '<' inlined directly, zero comparator indirection. The rewrite
therefore drops BOTH costs — the union of PS3012 (SortedFunc's
comparator indirection) and PS3006 (SortStableFunc's stability
overhead). Passing cmp.Compare itself as the comparator
(slices.SortedStableFunc(seq, cmp.Compare)) is the same anti-pattern
minus one layer and is matched too.

The result is BIT-IDENTICAL for the element types the fix accepts.
slices.Collect(seq) is shared verbatim by both entry points — the seq
expression is kept in place, evaluated once, and consumed identically;
the deleted comparator is only referenced, never called, so no side
effect or evaluation count changes. For the sort itself cmp.Compare(a, b)
induces exactly the order slices.Sort uses: slices.Sort is defined via
cmp.Less, and cmp.Compare(a, b) < 0 iff cmp.Less(a, b), so every
comparison the shared pdqsort makes answers the same on both sides and
the non-tie permutation is identical. Stability is the only remaining
difference, and the fix is offered only when the element type's
underlying type is an integer or string kind — there any two elements
that compare equal under cmp.Compare are indistinguishable values
(integers bitwise so; equal strings are interchangeable in every safe-Go
operation), so the stable sort's tie-order guarantee buys nothing
observable and the unstable rewrite returns a byte-for-byte identical
slice. Named element types (type Celsius int) are fixed too: sorting
orders by the ordered value and a String()/Error() method on the element
is never consulted.

FLOAT elements are reported ADVISORY only, never auto-fixed — the same
policy as PS3006/PS3012, and it bites HARDER here: -0.0 and +0.0 compare
equal but differ in bits, as do NaNs with distinct payloads, and
SortedStableFunc contractually KEEPS such ties in their collection order
while the unstable rewrite may arrange them either way. (The overall
ORDER is still identical — cmp.Compare and slices.Sort both put NaNs
first — which is why the report still fires as advice.) A TYPE-PARAMETER
element is likewise advisory: its instantiations may include floats.

The match is deliberately EXACT (the shared PS3107 comparator matcher).
The comparator must be cmp.Compare itself, or a func literal whose whole
body is a single return of cmp.Compare(p0, p1) with the two parameters
in SOURCE ORDER — a swapped cmp.Compare(b, a) is a descending sort and
is never matched; a field selector, a captured variable or any extra
computation fails the match silently. Both cmp and slices are resolved
with type information — only the stdlib packages match, never a shadowed
local or a same-named method — and an aliased import matches naturally.

The fix replaces only the SortedStableFunc selector name with Sorted and
deletes the comparator argument: the seq expression is kept VERBATIM in
place (single evaluation preserved) and the package qualifier keeps the
file's alias. An explicit instantiation slices.SortedStableFunc[E](seq,
...) keeps its bracket — slices.Sorted has the same single element type
parameter, and the fixable element kinds all satisfy its cmp.Ordered
constraint. Deleting the comparator removes the file's cmp reference, so
when the rewrites remove the file's LAST cmp references the fix also
drops the cmp import (alias included); a cgo file or a comment inside
the deleted span keeps the report advisory. The report fires only from
go1.23 on (slices.SortedStableFunc, slices.Sorted and iter all appeared
there) — in practice code containing this pattern already compiles only
on go1.23+.`,
		Before: `out := slices.SortedStableFunc(maps.Values(m), func(a, b int) int { return cmp.Compare(a, b) })`,
		After:  `out := slices.Sorted(maps.Values(m))`,
		MeasuredWin: `BenchmarkPS3016 (a 4096-element scattered iter.Seq[int]
collected and sorted per op, Apple M2 Pro, go1.26): ~375 µs/op before vs
~128 µs/op after — about 2.9x — identical allocations either way
(128 KB, 19 allocs: the shared slices.Collect growth), so the delta is
pure sort work. It stacks the two savings: the indirect comparator call
(plus the cmp.Compare hop inside the literal) per comparison that the
monomorphized slices.Sort inlines away (the PS3012 share) and the stable
insertion-run/symMerge machinery that the unstable pdqsort skips
entirely (the PS3006 share).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3016",
		Doc:  "slices.SortedStableFunc with a bare cmp.Compare comparator instead of slices.Sorted",
		Run:  runPS3016,
	},
})

func runPS3016(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3012SortedAvailable(pass, f) {
			// slices.Sorted/SortedStableFunc exist only from go1.23 on;
			// below that the advice is moot, so stay silent entirely (same
			// per-file gate as PS3012).
			continue
		}
		// Collect first, decide the cmp-import edit once per file: every
		// fixable call deletes exactly ONE cmp reference (the comparator's
		// cmp.Compare selector), and whether the cmp import is orphaned
		// depends on ALL of them together (same per-file collection as
		// PS3006/PS3012).
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
			// slices.SortedStableFunc, resolved through type info — never a
			// shadowed slices or a same-named method. An explicit
			// instantiation slices.SortedStableFunc[E] is unwrapped; its
			// bracket survives the fix (slices.Sorted has the same single
			// type parameter, and the fixable element kinds satisfy its
			// cmp.Ordered constraint).
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "SortedStableFunc")
			if !ok {
				return true
			}
			// The comparator must BE cmp.Compare on the parameters in
			// source order (reuses PS3107's exact matcher): a fresh
			// literal wrapping it, or cmp.Compare itself — anything else
			// is not an ascending slices.Sorted and is never reported.
			if !ps3107BareCompare(pass, call.Args[1]) {
				return true
			}
			// int/string fixable (equal values are bitwise-identical, so
			// dropping the stable tie-order guarantee is unobservable),
			// float and type-parameter advisory — the identical
			// classification (and identical divergence shapes) as
			// PS3006/PS3012.
			elem, fixableElem, why := ps3107Elem(pass, call.Args[1])
			var fix *analysis.SuggestedFix
			if fixableElem {
				fix = ps3012Fix(f, call, sel)
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
		// still spells slices.Sorted through the same qualifier.
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
			// PS3006/PS3012).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "slices.SortedStableFunc with a bare cmp.Compare comparator pays an indirect comparator call per comparison plus the stable sort's merge overhead; slices.Sorted collects and sorts the " + st.elem + " elements with the identical ascending order, the comparison inlined and no stability cost" + st.why,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

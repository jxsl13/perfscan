package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3018 reports a slices.SortedStableFunc whose comparator is nothing but
// strings.Compare on the two parameters in source order — slices.Sorted
// spelled the slow, STABLE way for string elements — and rewrites it to
// slices.Sorted. It completes the Sorted family square: PS3012
// (SortedFunc + cmp.Compare), PS3015 (SortedFunc + strings.Compare) and
// PS3016 (SortedStableFunc + cmp.Compare) all ship; this is the fourth cell,
// SortedStableFunc + strings.Compare, and like PS3016 the rewrite drops the
// comparator indirection AND the stable sort's overhead at once.
var PS3018 = register(&lint.Check{
	ID:       "PS3018",
	Category: "indirect",
	Slug:     "sortedstablefunc-strings-compare-to-slices-sorted",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.SortedStableFunc with a bare strings.Compare(a, b) comparator is slices.Sorted spelled the slow, stable way",
		Text: `slices.SortedStableFunc(seq, func(a, b string) int { return strings.Compare(a, b) })
collects the iter.Seq into a fresh slice and sorts it ascending
byte-lexicographically — exactly what slices.Sorted(seq) does on strings —
but pays TWICE for it. First, every comparison dispatches through the
comparator func value, and inside that closure strings.Compare is itself
another call doing a full three-way compare. Second, SortedStableFunc runs
the STABLE algorithm — insertion-sorted runs merged by a rotation-based
symMerge — which does strictly more work than pdqsort to preserve the
relative order of equal elements. slices.Sorted is a distinct
monomorphized entry point: Collect then unstable pdqsort with the ordered
'<' inlined directly, zero comparator indirection. The rewrite therefore
drops BOTH costs — the union of PS3015 (SortedFunc's comparator
indirection on the strings.Compare spelling) and PS3016's stability share.
Passing strings.Compare itself as the comparator
(slices.SortedStableFunc(seq, strings.Compare)) is the same anti-pattern
minus one layer and is matched too.

The result is BIT-IDENTICAL, and the proof is strictly simpler than
PS3016's: the element type is always string (strings.Compare's parameters
are string, so a comparator built from it only type-checks over string
elements — a defined type with underlying string does not compile without
conversions, which the matcher never accepts). COLLECTION:
slices.Collect(seq) is shared verbatim by both entry points — the seq
expression is kept in place, evaluated once, and consumed identically; the
deleted comparator is only referenced, never called, so no side effect or
evaluation count changes. ORDER: strings.Compare(a, b) < 0 iff a < b
byte-lexicographically, which is precisely cmp.Less on string — the order
slices.Sort is defined by — so every comparison answers the same on both
sides and the non-tie permutation is identical. TIES: stability is the
only remaining difference, and strings that compare equal under
strings.Compare are byte-equal values, interchangeable in every safe-Go
operation — so the stable sort's tie-order guarantee buys nothing
observable and the unstable rewrite returns a byte-for-byte identical
slice. Invalid UTF-8 is irrelevant — both sides order by raw bytes, never
runes. Neither entry point panics on an empty or nil-yielding seq. The
float/NaN advisory carve-outs of the cmp.Compare family cannot arise here
at all.

The match is deliberately EXACT — the same comparator matcher as
PS3009/PS3015. The comparator must be strings.Compare itself, or a func
literal whose whole body is a single return of strings.Compare(p0, p1)
with the two parameters in SOURCE ORDER, resolved by object identity — a
swapped strings.Compare(b, a) is a descending sort and is never matched; a
conversion (strings.Compare(string(a), string(b))), a field selector, a
captured outer variable or any extra computation fails the match silently.
Both strings and slices are resolved with type information — only the
stdlib packages match, never a shadowed local or a same-named method — and
an aliased import matches naturally.

The fix replaces only the SortedStableFunc selector name with Sorted and
deletes the comparator argument: the seq expression is kept VERBATIM in
place (single evaluation preserved) and the package qualifier keeps the
file's alias. An explicit instantiation slices.SortedStableFunc[string](seq,
...) keeps its bracket — slices.Sorted has the same single element type
parameter, and string satisfies its cmp.Ordered constraint. Deleting the
comparator removes the file's strings reference, so when the rewrites
remove the file's LAST strings references the fix also drops the strings
import (alias included); a cgo file or a comment inside the deleted span
keeps the report advisory. The report fires only from go1.23 on
(slices.SortedStableFunc, slices.Sorted and iter all appeared there) — in
practice code containing this pattern already compiles only on go1.23+.`,
		Before: `keys := slices.SortedStableFunc(maps.Keys(m), func(a, b string) int { return strings.Compare(a, b) })`,
		After:  `keys := slices.Sorted(maps.Keys(m))`,
		MeasuredWin: `BenchmarkPS3018 (a 4096-element tie-heavy iter.Seq[string]
of shuffled short keys collected and sorted per op, Apple M2 Pro, go1.26):
~710-840 µs/op before (func literal), ~745-790 µs/op before (bare
strings.Compare value) vs ~370-400 µs/op after — about 2x — identical
allocations either way (240 KB, 18 allocs: the shared slices.Collect
growth). The win stacks the two savings: the indirect comparator call
(plus the strings.Compare hop inside the literal) per comparison that the
monomorphized slices.Sort inlines away (the PS3015 share) and the stable
insertion-run/symMerge machinery that the unstable pdqsort skips entirely
(the PS3016 share) — which is why the delta is far larger than PS3015's
few percent on the same input shape.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3018",
		Doc:  "slices.SortedStableFunc with a bare strings.Compare(a, b) comparator instead of slices.Sorted",
		Run:  runPS3018,
	},
})

func runPS3018(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3012SortedAvailable(pass, f) {
			// slices.Sorted/SortedStableFunc exist only from go1.23 on; below
			// that the advice is moot, so stay silent entirely (same per-file
			// gate policy as PS3012).
			continue
		}
		// Collect first, decide the strings-import edit once per file: every
		// fixable call deletes exactly ONE strings reference (the
		// comparator's strings.Compare selector), and whether the strings
		// import is orphaned depends on ALL of them together (same per-file
		// collection as PS3009/PS3015/PS3016).
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
			// instantiation slices.SortedStableFunc[string] is unwrapped; its
			// bracket survives the fix (slices.Sorted has the same single
			// type parameter, and string satisfies cmp.Ordered).
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "SortedStableFunc")
			if !ok {
				return true
			}
			// The comparator must BE strings.Compare on the parameters in
			// source order (reuses PS3009's exact matcher): a fresh literal
			// wrapping it, or strings.Compare itself — anything else is not
			// an ascending slices.Sorted and is never reported.
			if !ps3009BareCompare(pass, call.Args[1]) {
				return true
			}
			// strings.Compare pins the element type to string, where equal
			// values are byte-identical and dropping the stable tie-order
			// guarantee is unobservable; ps3009Elem's non-string branches are
			// purely defensive.
			elem, fixableElem, why := ps3009Elem(pass, call.Args[1])
			var fix *analysis.SuggestedFix
			if fixableElem {
				// The rewrite itself is PS3012's: only the selector name and
				// the text after the seq argument change.
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
		// Each fixable call deletes exactly one strings reference; when those
		// are ALL of the file's strings references, the rewrites orphan the
		// import and the fix must drop it (the runner never prunes imports
		// itself). The slices import always survives: the rewritten call
		// still spells slices.Sorted through the same qualifier.
		dropStrings := fixable > 0 && pkgRefCount(pass, f, "strings") == fixable
		importEdits, importsOK := ps3009ImportEdits(f, dropStrings)
		if !importsOK {
			// cgo file needing import surgery, or a strings spec we could
			// not locate: keep every report advisory.
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edit (same convention as
			// PS3009/PS3015/PS3016).
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
				Message: "slices.SortedStableFunc with a bare strings.Compare(a, b) comparator pays an indirect comparator call plus a strings.Compare call per comparison, and the stable sort's merge overhead on top; slices.Sorted collects and sorts the " + st.elem + " elements in the identical byte-lexicographic order with the comparison inlined and no stability cost" + st.why,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

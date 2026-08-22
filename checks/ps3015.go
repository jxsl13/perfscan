package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3015 reports a slices.SortedFunc whose comparator is nothing but
// strings.Compare on the two parameters in source order — slices.Sorted
// spelled the slow way for string elements — and rewrites it to
// slices.Sorted. Sibling of PS3012 (the same shape with a cmp.Compare
// comparator, which that matcher deliberately limits itself to) and of
// PS3009 (the strings.Compare spelling of the eager slices.SortFunc family):
// the same "generic Func variant fed a bare three-way compare" anti-pattern,
// on the iterator-collecting sort with the strings-package comparator.
var PS3015 = register(&lint.Check{
	ID:       "PS3015",
	Category: "indirect",
	Slug:     "sortedfunc-strings-compare-to-slices-sorted",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.SortedFunc with a bare strings.Compare(a, b) comparator is slices.Sorted spelled the slow way",
		Text: `slices.SortedFunc(seq, func(a, b string) int { return strings.Compare(a, b) })
collects the iter.Seq into a fresh slice and sorts it ascending
byte-lexicographically — exactly what slices.Sorted(seq) does on strings —
but pays an indirect call through the comparator func value on EVERY
comparison, and inside that closure strings.Compare is itself another call
doing a full three-way compare. Both stdlib functions are literally
Collect-then-sort: SortedFunc runs slices.SortFunc over the collected
slice, Sorted runs slices.Sort — a distinct monomorphized entry point that
inlines the ordered '<' comparison directly into pdqsort. Same collection,
same algorithm, zero comparator indirection. Passing strings.Compare
itself as the comparator (slices.SortedFunc(seq, strings.Compare)) is the
same anti-pattern minus one layer and is matched too.

The result is BIT-IDENTICAL, and the proof is strictly simpler than
PS3012's: the element type is always string (strings.Compare's parameters
are string, so a comparator built from it only type-checks over string
elements — a defined type with underlying string does not compile without
conversions, which the matcher never accepts). COLLECTION:
slices.Collect(seq) is shared verbatim by both entry points — the seq
expression is kept in place, evaluated once, and consumed identically; the
deleted comparator is only referenced, never called, so no side effect or
evaluation count changes. ORDER: strings.Compare(a, b) < 0 iff a < b
byte-lexicographically, which is precisely cmp.Less on string — the order
slices.Sort is defined by — so every comparison the shared pdqsort
template makes answers the same on both sides and the non-tie permutation
is identical. TIES: strings that compare equal are byte-equal and
therefore interchangeable in every safe-Go operation, so the unstable
sort's freedom to arrange ties is unobservable and the returned slice is
byte-for-byte the same. Invalid UTF-8 is irrelevant — both sides order by
raw bytes, never runes. The float/NaN advisory carve-outs of the
cmp.Compare family cannot arise here at all.

The match is deliberately EXACT — the same comparator matcher as PS3009.
The comparator must be strings.Compare itself, or a func literal whose
whole body is a single return of strings.Compare(p0, p1) with the two
parameters in SOURCE ORDER, resolved by object identity — a swapped
strings.Compare(b, a) is a descending sort and is never matched; a
conversion (strings.Compare(string(a), string(b))), a field selector, a
captured outer variable or any extra computation fails the match silently.
Both strings and slices are resolved with type information — only the
stdlib packages match, never a shadowed local or a same-named method — and
an aliased import matches naturally.

The fix replaces only the SortedFunc selector name with Sorted and deletes
the comparator argument: the seq expression is kept VERBATIM in place
(single evaluation preserved) and the package qualifier keeps the file's
alias. An explicit instantiation slices.SortedFunc[string](seq, ...) keeps
its bracket — slices.Sorted has the same single element type parameter,
and string satisfies its cmp.Ordered constraint. Deleting the comparator
removes the file's strings reference, so when the rewrites remove the
file's LAST strings references the fix also drops the strings import
(alias included); a cgo file or a comment inside the deleted span keeps
the report advisory. The report fires only from go1.23 on
(slices.SortedFunc, slices.Sorted and iter all appeared there) — in
practice code containing this pattern already compiles only on go1.23+.`,
		Before: `keys := slices.SortedFunc(maps.Keys(m), func(a, b string) int { return strings.Compare(a, b) })`,
		After:  `keys := slices.Sorted(maps.Keys(m))`,
		MeasuredWin: `BenchmarkPS3015 (a 4096-element tie-heavy iter.Seq[string]
of shuffled short keys collected and sorted per op, Apple M2 Pro, go1.26):
~385-412 µs/op before (func literal), ~405-420 µs/op before (bare
strings.Compare value) vs ~390 µs/op after — a modest few percent —
identical allocations either way (240 KB, 18 allocs: the shared
slices.Collect growth). The delta is smaller than the []int sibling's
(PS3012, ~1.8x) and than the eager PS3009 form's (~1.15x) because the
byte comparison is a runtime cmpstring call on BOTH sides and the shared
per-op collection cost dilutes the sort-only win further; what the
rewrite removes is the indirect comparator call plus the strings.Compare
hop inside the literal, per comparison.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3015",
		Doc:  "slices.SortedFunc with a bare strings.Compare(a, b) comparator instead of slices.Sorted",
		Run:  runPS3015,
	},
})

func runPS3015(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3012SortedAvailable(pass, f) {
			// slices.Sorted/SortedFunc exist only from go1.23 on; below
			// that the advice is moot, so stay silent entirely (same
			// per-file gate policy as PS3012).
			continue
		}
		// Collect first, decide the strings-import edit once per file: every
		// fixable call deletes exactly ONE strings reference (the
		// comparator's strings.Compare selector), and whether the strings
		// import is orphaned depends on ALL of them together (same per-file
		// collection as PS3009/PS3011/PS3012).
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
			// slices.SortedFunc, resolved through type info — never a
			// shadowed slices or a same-named method. An explicit
			// instantiation slices.SortedFunc[string] is unwrapped; its
			// bracket survives the fix (slices.Sorted has the same single
			// type parameter, and string satisfies cmp.Ordered).
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "SortedFunc")
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
			// values are byte-identical and the unstable sort's tie
			// arrangement is unobservable; ps3009Elem's non-string branches
			// are purely defensive.
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
			// PS3009/PS3011/PS3012).
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
				Message: "slices.SortedFunc with a bare strings.Compare(a, b) comparator pays an indirect comparator call plus a strings.Compare call per comparison; slices.Sorted collects and sorts the " + st.elem + " elements in the identical byte-lexicographic order with the comparison inlined" + st.why,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3011 reports a slices.BinarySearchFunc whose comparator is nothing but
// strings.Compare on the two parameters in source order — slices.BinarySearch
// spelled the slow way for string elements — and rewrites it to
// slices.BinarySearch. Sibling of PS3109 (the same shape with a cmp.Compare
// comparator, which that matcher deliberately limits itself to) and of PS3009
// (the strings.Compare spelling of the slices.SortFunc family).
var PS3011 = register(&lint.Check{
	ID:       "PS3011",
	Category: "indirect",
	Slug:     "binarysearchfunc-strings-compare-to-slices-binarysearch",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.BinarySearchFunc with a bare strings.Compare comparator is slices.BinarySearch spelled the slow way",
		Text: `slices.BinarySearchFunc(s, target, func(a, b string) int { return strings.Compare(a, b) })
searches for target exactly as slices.BinarySearch(s, target) does, but pays
an indirect call through the comparator func value on every probe (log n
probes), and inside that closure strings.Compare is itself another call doing
the three-way byte comparison. slices.BinarySearch is a distinct monomorphized
entry point that compares with the ordered '<' operator directly: same probes,
zero comparator indirection. Passing strings.Compare itself as the comparator
(slices.BinarySearchFunc(s, target, strings.Compare)) is the same anti-pattern
minus one layer and is matched too.

The result is BIT-IDENTICAL on every input — sorted or not. Both entry points
run the same textbook loop over the same probe indices; the only question at
each probe is the SIGN of the comparison, and strings.Compare(a, b) < 0 iff
a < b byte-lexicographically — precisely cmp.Less on string, the relation
slices.BinarySearch branches on — while strings.Compare(a, b) == 0 iff the
strings are byte-equal, precisely the == the found-check uses. So every branch
answers the same on both sides and the (index, found) pair is identical, even
on garbage (unsorted) input where the answer is meaningless but still
deterministic. Invalid UTF-8 is irrelevant — both sides order by raw bytes,
never runes — and because strings.Compare's parameters are plain string, the
element type is always string (a defined type with underlying string does not
type-check against the comparator without conversions, which the matcher never
accepts), so the float NaN carve-outs of the cmp.Compare family cannot arise.

The match is deliberately EXACT — the same comparator matcher as PS3009. The
comparator must be strings.Compare itself, or a func literal whose whole body
is a single return of strings.Compare(p0, p1) with the two parameters in
SOURCE ORDER, resolved by object identity — a swapped strings.Compare(b, a) is
a descending search over descending data and is never matched; a conversion, a
field selector, a captured outer variable or any extra computation fails the
match silently. Both strings and slices are resolved with type information —
only the stdlib packages match, never a shadowed local or a same-named method
— and an aliased import matches naturally.

The fix replaces only the BinarySearchFunc selector name with BinarySearch and
deletes the comparator argument: the slice and target expressions are kept
VERBATIM in place (single evaluation preserved) and the package qualifier
keeps the file's alias. An explicit instantiation
slices.BinarySearchFunc[S, E, T] carries THREE type arguments where
BinarySearch has only two type parameters, so those brackets do not transfer
and such a site stays advisory (same restriction as PS3109). Deleting the
comparator removes the file's strings reference, so when the rewrites remove
the file's LAST strings references the fix also drops the strings import
(alias included); a cgo file or a comment inside the deleted span keeps the
report advisory.

The report fires only from go1.21 on (slices.BinarySearch appeared there), the
same gate as PS3104/PS3107/PS3109; in practice code containing this pattern
already compiles only on go1.21+.`,
		Before: `i, ok := slices.BinarySearchFunc(s, target, func(a, b string) int { return strings.Compare(a, b) })`,
		After:  `i, ok := slices.BinarySearch(s, target)`,
		MeasuredWin: `BenchmarkPS3011 (a 4096-element sorted []string of 8-byte
keys sharing a 4-byte prefix, targets cycled across the full range so every
search runs a full log n probes, Apple M2 Pro, go1.26): ~64 ns/op before vs
~56 ns/op after, ~1.13x, 0 allocs either way. The delta is smaller than the
[]int sibling's (PS3109, ~1.8x) because the byte comparison is a runtime
cmpstring call on BOTH sides; what the rewrite removes is the indirect
comparator call plus the strings.Compare hop inside the literal closure, per
probe.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3011",
		Doc:  "slices.BinarySearchFunc with a bare strings.Compare comparator instead of slices.BinarySearch",
		Run:  runPS3011,
	},
})

func runPS3011(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.BinarySearch exists only from go1.21 on (same gate as
			// PS3104/PS3107/PS3109); below that the advice is moot.
			continue
		}
		// Collect first, decide the strings-import edit once per file: every
		// fixable call deletes exactly ONE strings reference (the comparator's
		// strings.Compare selector), and whether the strings import is
		// orphaned depends on ALL of them together (same per-file collection
		// as PS3009/PS3109).
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
			if !ok || len(call.Args) != 3 || call.Ellipsis.IsValid() {
				return true
			}
			// The callee must be the receiver-less package function
			// slices.BinarySearchFunc, resolved through type info — never a
			// shadowed slices or a same-named method. An explicit
			// instantiation slices.BinarySearchFunc[S, E, T] is unwrapped by
			// ps3107PkgFunc to resolve the callee; ps3109ExplicitInst below
			// keeps such a site advisory (its three type arguments do not
			// transfer to BinarySearch's two).
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "BinarySearchFunc")
			if !ok {
				return true
			}
			// The comparator must BE strings.Compare on the parameters in
			// source order (reuses PS3009's exact matcher).
			if !ps3009BareCompare(pass, call.Args[2]) {
				return true
			}
			elem, fixableElem, why := ps3009Elem(pass, call.Args[2])
			var fix *analysis.SuggestedFix
			if fixableElem && !ps3109ExplicitInst(call.Fun) {
				// The rewrite itself is PS3109's: only the selector name and
				// the text after the target argument change.
				fix = ps3109Fix(f, call, sel)
			}
			if fix != nil {
				fixable++
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
		// still spells slices.BinarySearch through the same qualifier.
		dropStrings := fixable > 0 && pkgRefCount(pass, f, "strings") == fixable
		importEdits, importsOK := ps3009ImportEdits(f, dropStrings)
		if !importsOK {
			// cgo file needing import surgery, or a strings spec we could not
			// locate: keep every report advisory.
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edit (same convention as
			// PS3009/PS3109).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			why := st.why
			if why == "" && st.fix == nil {
				why = " (no auto-fix: an explicit instantiation's three type arguments do not transfer to BinarySearch's two, a cgo file, or a comment in the deleted span)"
			}
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "slices.BinarySearchFunc with a bare strings.Compare comparator pays an indirect comparator call plus a strings.Compare call per probe; slices.BinarySearch searches the " + st.elem + " elements with the identical (index, found) result and the comparison inlined" + why,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

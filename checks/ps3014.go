package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3014 reports a slices.IsSortedFunc whose comparator is nothing but
// strings.Compare on the two parameters in source order — slices.IsSorted
// spelled the slow way for string elements — and rewrites it to
// slices.IsSorted. Sibling of PS3010 (the same shape with a cmp.Compare
// comparator, which that matcher deliberately limits itself to) and of
// PS3009/PS3011 (the strings.Compare spelling of the SortFunc and
// BinarySearchFunc families).
var PS3014 = register(&lint.Check{
	ID:       "PS3014",
	Category: "indirect",
	Slug:     "issortedfunc-strings-compare-to-slices-issorted",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.IsSortedFunc with a bare strings.Compare comparator is slices.IsSorted spelled the slow way",
		Text: `slices.IsSortedFunc(s, func(a, b string) int { return strings.Compare(a, b) })
scans s once and reports whether it is ascending — exactly what
slices.IsSorted(s) does — but pays an indirect call through the comparator
func value on EVERY adjacent pair, and inside that closure strings.Compare is
itself another call computing a full three-way byte comparison.
slices.IsSorted is a distinct monomorphized entry point that inlines the
ordered comparison (cmp.Less, a single '<') directly into the scan: same O(n)
single pass, same early return at the first out-of-order pair, zero
comparator indirection. Passing strings.Compare itself as the comparator
(slices.IsSortedFunc(s, strings.Compare)) is the same anti-pattern minus one
layer and is matched too.

The result is BIT-IDENTICAL on every input. slices.IsSorted is defined as the
descending scan 'if cmp.Less(x[i], x[i-1]) return false'; IsSortedFunc is the
same loop with 'cmp(x[i], x[i-1]) < 0'. With cmp = strings.Compare,
strings.Compare(a, b) < 0 holds iff a < b byte-lexicographically — precisely
cmp.Less on string — so every iteration answers identically and the loop
returns the identical bool at the identical point. Invalid UTF-8 is
irrelevant: both sides order by raw bytes, never runes. Because
strings.Compare's parameters are plain string, the element type is always
string (a defined type with underlying string does not type-check against the
comparator without conversions, which the matcher never accepts), so the
float NaN carve-outs of the cmp.Compare family cannot arise. The two
parameters are read once each in both spellings and there is nothing else in
the matched closure — no side effect can be lost.

The match is deliberately EXACT — the same comparator matcher as
PS3009/PS3011. The comparator must be strings.Compare itself, or a func
literal whose whole body is a single return of strings.Compare(p0, p1) with
the two parameters in SOURCE ORDER, resolved by object identity — a swapped
strings.Compare(b, a) asks whether s is DESCENDING and is never matched; a
conversion, a field selector, a captured outer variable or any extra
computation fails the match silently. Both strings and slices are resolved
with type information — only the stdlib packages match, never a shadowed
local or a same-named method — and an aliased import matches naturally.

The fix replaces only the IsSortedFunc selector name with IsSorted and
deletes the comparator argument: the slice expression is kept VERBATIM in
place (single evaluation preserved) and the package qualifier keeps the
file's alias. An explicit instantiation slices.IsSortedFunc[S, E] keeps its
brackets — slices.IsSorted has the same two type parameters, and the matched
strings.Compare already proves E is string, which is cmp.Ordered. Deleting
the comparator removes the file's strings reference, so when the rewrites
remove the file's LAST strings references the fix also drops the strings
import (alias included); a cgo file or a comment inside the deleted span
keeps the report advisory.

The report fires only from go1.21 on (slices.IsSorted and
slices.IsSortedFunc appeared there), the same gate as PS3010/PS3104/PS3107;
in practice code containing this pattern already compiles only on go1.21+.`,
		Before: `ok := slices.IsSortedFunc(s, func(a, b string) int { return strings.Compare(a, b) })`,
		After:  `ok := slices.IsSorted(s)`,
		MeasuredWin: `BenchmarkPS3014 (a sorted 4096-element []string of 8-byte
keys sharing a 4-byte prefix scanned per op — sorted input forces the full
O(n) pass, Apple M2 Pro, go1.26): ~8.14 µs/op before vs ~7.19 µs/op after,
~1.13x, 0 allocs either way. The delta is smaller than the []int sibling's
(PS3010, ~4x) because the byte comparison is a runtime string-compare call on
BOTH sides; what the rewrite removes is the indirect comparator call plus the
strings.Compare hop inside the literal closure, on each adjacent pair.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3014",
		Doc:  "slices.IsSortedFunc with a bare strings.Compare comparator instead of slices.IsSorted",
		Run:  runPS3014,
	},
})

func runPS3014(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.IsSorted exists only from go1.21 on (same gate as
			// PS3010/PS3104/PS3107); below that the advice is moot.
			continue
		}
		// Collect first, decide the strings-import edit once per file: every
		// fixable call deletes exactly ONE strings reference (the
		// comparator's strings.Compare selector), and whether the strings
		// import is orphaned depends on ALL of them together (same per-file
		// collection as PS3009/PS3011).
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
			// slices.IsSortedFunc, resolved through type info — never a
			// shadowed slices or a same-named method. An explicit
			// instantiation slices.IsSortedFunc[S, E] is unwrapped; its
			// brackets survive the fix (slices.IsSorted has the same two
			// type parameters, and the matched strings.Compare already
			// proves E is string, which is cmp.Ordered).
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "IsSortedFunc")
			if !ok {
				return true
			}
			// The comparator must BE strings.Compare on the parameters in
			// source order (reuses PS3009's exact matcher).
			if !ps3009BareCompare(pass, call.Args[1]) {
				return true
			}
			elem, fixableElem, why := ps3009Elem(pass, call.Args[1])
			var fix *analysis.SuggestedFix
			if fixableElem {
				// The rewrite itself is PS3010's: only the selector name and
				// the text after the slice argument change.
				fix = ps3010Fix(f, call, sel)
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
		// Each fixable call deletes exactly one strings reference; when
		// those are ALL of the file's strings references, the rewrites
		// orphan the import and the fix must drop it (the runner never
		// prunes imports itself). The slices import always survives: the
		// rewritten call still spells slices.IsSorted through the same
		// qualifier.
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
			// PS3009/PS3011).
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
				why = " (no auto-fix: a cgo file or a comment in the deleted span)"
			}
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "slices.IsSortedFunc with a bare strings.Compare comparator pays an indirect comparator call plus a strings.Compare call per adjacent pair; slices.IsSorted answers the identical bool over the " + st.elem + " elements with the comparison inlined" + why,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

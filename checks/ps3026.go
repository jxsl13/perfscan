package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3026 reports a slices.SortedFunc whose comparator is a hand-rolled
// three-way if/switch chain (a<b -> negative, a>b -> positive, else 0) —
// slices.Sorted spelled the slow way — and rewrites it to slices.Sorted.
// Sibling of PS3012, which catches the same anti-pattern spelled with
// cmp.Compare on the iterator-collecting sort, and of PS3013, which catches
// this PRE-cmp hand-rolled spelling on the eager slices.SortFunc; PS3026
// closes the remaining cell of that matrix.
var PS3026 = register(&lint.Check{
	ID:       "PS3026",
	Category: "indirect",
	Slug:     "sortedfunc-handrolled-threeway-to-slices-sorted",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.SortedFunc with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) is slices.Sorted spelled the slow way",
		Text: `slices.SortedFunc(seq, func(a, b T) int { if a < b { return -1 };
if a > b { return 1 }; return 0 }) collects the iter.Seq into a fresh
slice and sorts it ascending — exactly what slices.Sorted(seq) does —
but pays for it twice per comparison: the comparator is a func value
invoked through an indirect call on every one of the O(n log n)
comparisons, and its body performs up to TWO relational comparisons
(a<b, then a>b) to synthesize a -1/0/1 sign that the sort only ever
consumes as '<'. Both stdlib functions are literally Collect-then-sort:
SortedFunc runs slices.SortFunc over the collected slice, Sorted runs
slices.Sort — a distinct monomorphized entry point that inlines a SINGLE
ordered '<' (via cmp.Less) directly into pdqsort. Same collection, same
unstable algorithm, zero comparator indirection, one comparison instead
of up to two: the same measured win PS3013 pins for the eager SortFunc
form and PS3012 pins for the cmp.Compare spelling of this very call.

The result is BIT-IDENTICAL for the element types the fix accepts —
exactly PS3012/PS3013's proven policy: the fix is offered only when the
element type's underlying type is an integer or string kind.
slices.Collect(seq) is shared verbatim by both entry points — the seq
expression is kept in place, evaluated once, and consumed identically
(same yields, same count); the deleted comparator is only referenced,
never called, so no side effect or evaluation count changes. For the
sort itself the if-chain returns a value whose SIGN equals
cmp.Compare(a, b) on every pair (a<b -> negative, a>b -> positive,
equal -> 0), and the sort consumes only that sign; slices.Sort is
defined via cmp.Less with cmp.Compare(a, b) < 0 iff cmp.Less(a, b), so
every comparison answers the same on both sides and the permutation is
identical. Because equal integer/string elements are bitwise-identical
values, even the unstable sort's freedom to arrange ties is unobservable
and the returned slice is byte-for-byte identical — and both spellings
return the same empty result on an empty seq (no panic divergence).
'<'/'>' on integers and strings are side-effect-free and
non-overloadable; no String()/Error()/Format() method is ever consulted
by ordering. Named element types (type Priority int) are fixed too.

FLOAT elements are reported ADVISORY only, never auto-fixed — and the
reason is sharper than the usual -0.0/+0.0 and NaN-payload ties an
unstable sort may arrange either way: the hand-rolled chain answers 0
for a NaN against ANYTHING ('<' and '>' are both false), i.e. it is not
even a consistent ordering, while slices.Sorted (via slices.Sort) orders
NaNs first — the two disagree on any input containing a NaN. A
TYPE-PARAMETER element is likewise advisory: its instantiations may
include floats.

The match is deliberately EXACT (the shared PS3013 three-way matcher).
The comparator must be a fresh func literal whose whole body is the
three-way chain over the two BARE parameters, resolved by object
identity, in any of the equivalent spellings: two sequential ifs plus a
trailing return, an if/else-if chain (with the default either a trailing
return or a final else), or an expressionless switch whose two cases
hold the comparisons (the default 0 either as a default clause or a
trailing return). Each condition must be a single '<' or '>' between the
two parameters — a<b and b>a both mean "less" — with the "less" branch
returning a NEGATIVE integer literal and the "greater" branch a POSITIVE
one (magnitude irrelevant: only the sign is consumed), one branch per
direction, and the default returning literal 0. Anything looser stays
silent: a subtraction comparator (return a - b) is explicitly NOT
matched because it can overflow; '<='/'>=' are not the three-way; a
swapped sign pair is a DESCENDING sort; a field selector (a.f < b.f), a
captured variable, a named constant, extra statements, or a named
comparator value all fail the match. Only returns that are integer
literals (with an optional unary sign) match, so deleting the comparator
can never orphan an import or any other reference.

The fix replaces only the SortedFunc selector name with Sorted and
deletes the comparator argument: the seq expression is kept VERBATIM in
place (single evaluation preserved), the package qualifier keeps
whatever alias the file used, and an explicit instantiation
slices.SortedFunc[E](seq, ...) keeps its bracket — slices.Sorted has the
same single element type parameter, and every fixable E (integer/string
underlying) satisfies its cmp.Ordered constraint. A comment anywhere in
the deleted span (the comparator body included) keeps the report
advisory rather than destroy it. The comparator literal references
nothing but its own parameters and integer literals, so no import ever
needs pruning.

The report only fires when the effective language version is at least
go1.23 (slices.SortedFunc, slices.Sorted and iter all appeared there) —
the same gate PS3012 applies; in practice code containing this pattern
already compiles only on go1.23+.`,
		Before: `hi := slices.SortedFunc(seq, func(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
})`,
		After: `hi := slices.Sorted(seq)`,
		MeasuredWin: `BenchmarkPS3026 (a 4096-element scattered iter.Seq[int]
collected and sorted per op, Apple M2 Pro, go1.26): ~208 µs/op before vs
~120 µs/op after — about 1.7x — identical allocations either way
(128 KB, 19 allocs: the shared slices.Collect growth), so the delta is
pure comparison overhead: the indirect comparator call plus the second
relational comparison per element pair that the monomorphized
slices.Sort inlines away — the same win PS3013 measures for the eager
SortFunc form and PS3012 for the cmp.Compare spelling.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3026",
		Doc:  "slices.SortedFunc with a hand-rolled three-way comparator instead of slices.Sorted",
		Run:  runPS3026,
	},
})

func runPS3026(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3012SortedAvailable(pass, f) {
			// slices.Sorted/SortedFunc exist only from go1.23 on; below
			// that the advice is moot, so stay silent entirely (same
			// per-file gate as PS3012).
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			// The callee must be the receiver-less package function
			// slices.SortedFunc, resolved through type info — never a
			// shadowed slices or a same-named method. An explicit
			// instantiation slices.SortedFunc[E] is unwrapped; its bracket
			// survives the fix (slices.Sorted has the same single type
			// parameter, and the fixable element kinds satisfy its
			// cmp.Ordered constraint).
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "SortedFunc")
			if !ok {
				return true
			}
			// The comparator must be a fresh literal that IS the exact
			// hand-rolled three-way (less -> negative literal, greater ->
			// positive literal, default -> literal 0) — the shared PS3013
			// matcher; anything looser is not provably slices.Sorted and is
			// never reported.
			lit, ok := ps2110Unparen(call.Args[1]).(*ast.FuncLit)
			if !ok || !ps3013ThreeWay(pass, lit) {
				return true
			}
			// int/string fixable (equal values are bitwise-identical, so
			// the unstable sort's tie arrangement is unobservable), float
			// and type-parameter advisory — the identical classification
			// (and identical divergence shapes) as PS3013's eager form.
			elem, fixable, why := ps3013Elem(pass, lit)
			order := " collects and sorts the " + elem + " elements with the identical ascending order and a single inlined comparison"
			if why != "" {
				// The advisory reasons (float, type parameter) are exactly
				// the cases where "identical" cannot be claimed — a NaN
				// makes this comparator inconsistent while slices.Sort
				// orders NaNs first — so the message drops the claim.
				order = " collects and sorts the " + elem + " elements ascending with a single inlined comparison"
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "slices.SortedFunc with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) pays an indirect comparator call plus up to two relational comparisons per comparison; slices.Sorted" + order + why,
			}
			if fixable {
				if fix := ps3026Fix(f, call, sel); fix != nil {
					diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
				}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps3026Fix builds the slices.Sorted(seq) rewrite for one call, or nil when a
// comment in the deleted span would be destroyed. Only the SortedFunc
// selector name and the text after the seq argument (the comma, the whole
// comparator and the closing paren) are replaced: the seq expression stays
// untouched in place (text and single evaluation preserved), the package
// qualifier keeps the file's alias, and an explicit instantiation bracket
// survives. The comparator references nothing but its parameters and integer
// literals, so no import edit is ever needed.
func ps3026Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	if ps2111CommentIn(f, call.Args[0].End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + ps3107Qualifier(sel) + "Sorted(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Sorted")},
			{Pos: call.Args[0].End(), End: call.End(), NewText: []byte(")")},
		},
	}
}

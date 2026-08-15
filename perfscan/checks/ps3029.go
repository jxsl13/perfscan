package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3029 reports a slices.SortedStableFunc whose comparator is a hand-rolled
// three-way if/switch chain (a<b -> negative, a>b -> positive, else 0) —
// slices.Sorted spelled the slow, STABLE way — and rewrites it to
// slices.Sorted. It closes the hand-rolled column of the Sorted-family
// matrix: PS3026 catches the same comparator under the unstable SortedFunc,
// PS3017 catches it under the eager SortStableFunc, and PS3016 catches the
// cmp.Compare spelling under this very callee. Like PS3016/PS3017 the
// rewrite drops the comparator indirection AND the stable algorithm's merge
// overhead at once.
var PS3029 = register(&lint.Check{
	ID:       "PS3029",
	Category: "indirect",
	Slug:     "sortedstablefunc-handrolled-threeway-to-slices-sorted",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.SortedStableFunc with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) is slices.Sorted spelled the slow, stable way",
		Text: `slices.SortedStableFunc(seq, func(a, b T) int { if a < b { return -1 };
if a > b { return 1 }; return 0 }) collects the iter.Seq into a fresh
slice and sorts it ascending — exactly what slices.Sorted(seq) does —
but pays THREE separate costs per one of its O(n log n) comparisons.
The comparator is a func value invoked through an indirect call; its
body performs up to TWO relational comparisons (a<b, then a>b) to
synthesize a -1/0/1 sign the sort only ever consumes as a bool; and
SortedStableFunc runs the STABLE algorithm — insertion-sorted runs
merged by a rotation-based symMerge — which does strictly more work
than pdqsort to preserve the relative order of equal elements.
slices.Sorted is a distinct monomorphized entry point: Collect then
unstable pdqsort with a SINGLE ordered '<' (via cmp.Less) inlined
directly. Same collection, zero comparator indirection, one comparison
instead of up to two, no stability overhead — this stacks the
comparator-inlining win PS3026 measures for the SortedFunc spelling on
top of the stable-machinery saving PS3016 measures for the cmp.Compare
spelling of this callee.

The result is BIT-IDENTICAL for the element types the fix accepts —
byte-for-byte the policy PS3016/PS3017/PS3026 ship: the fix is offered
only when the element type's underlying type is an integer or string
kind. COLLECTION: slices.Collect(seq) is shared verbatim by both entry
points — the seq expression is kept in place, evaluated once, and
consumed identically (same yields, same count); the deleted comparator
is only referenced, never called, so no side effect or evaluation count
changes, and both spellings return the same empty (nil) result on an
empty seq. PERMUTATION: the if-chain returns a value whose SIGN equals
cmp.Compare(a, b) on every pair (a<b -> negative, a>b -> positive,
equal -> 0), and the sort consumes only that sign; slices.Sort is
defined via cmp.Less with cmp.Compare(a, b) < 0 iff cmp.Less(a, b), so
every comparison answers the same on both sides and the non-tie
permutation is identical. TIES: stability is the only remaining
difference, and for integer/string elements any two values comparing
equal are indistinguishable (integers bitwise so; equal strings are
interchangeable in every safe-Go operation), so the stable sort's
tie-order guarantee buys nothing observable and the unstable rewrite's
output slice is byte-for-byte identical. '<'/'>' on integers and
strings are side-effect-free and non-overloadable; no
String()/Error()/Format() method is ever consulted by ordering. Named
element types (type Rank int) are fixed too: sorting orders by the
ordered value only.

FLOAT elements are reported ADVISORY only, never auto-fixed — and here
BOTH failure modes of the family bite at once: the hand-rolled chain
answers 0 for a NaN against ANYTHING ('<' and '>' are both false), i.e.
it is not even a consistent ordering, while slices.Sorted (via
slices.Sort) orders NaNs first; and -0.0/+0.0 (and NaNs with distinct
payloads) compare equal but differ in bits — ties SortedStableFunc
contractually KEEPS in their yield order while the unstable rewrite may
arrange them either way. A TYPE-PARAMETER element is likewise advisory:
its instantiations may include floats.

The match is deliberately EXACT (the shared PS3013 three-way matcher).
The comparator must be a fresh func literal whose whole body is the
three-way chain over the two BARE parameters, resolved by object
identity, in any of the equivalent spellings: two sequential ifs plus a
trailing return, an if/else-if chain (with the default either a
trailing return or a final else), or an expressionless switch whose two
cases hold the comparisons (the default 0 either as a default clause or
a trailing return). Each condition must be a single '<' or '>' between
the two parameters — a<b and b>a both mean "less" — with the "less"
branch returning a NEGATIVE integer literal and the "greater" branch a
POSITIVE one (magnitude irrelevant: only the sign is consumed), one
branch per direction, and the default returning literal 0. Anything
looser stays silent: a subtraction comparator (return a - b) is
explicitly NOT matched because it can overflow; '<='/'>=' are not the
three-way; a swapped sign pair is a DESCENDING sort; a field selector
(a.f < b.f), a captured variable, a named constant, extra statements,
or a named comparator value all fail the match. Only returns that are
integer literals (with an optional unary sign) match, so deleting the
comparator can never orphan an import or any other reference.

The fix replaces only the SortedStableFunc selector name with Sorted
and deletes the comparator argument: the seq expression is kept
VERBATIM in place (single evaluation preserved), the package qualifier
keeps whatever alias the file used, and an explicit instantiation
slices.SortedStableFunc[E](seq, ...) keeps its bracket — slices.Sorted
has the same single element type parameter, and every fixable E
(integer/string underlying) satisfies its cmp.Ordered constraint. A
comment anywhere in the deleted span (the comparator body included)
keeps the report advisory rather than destroy it. The comparator
literal references nothing but its own parameters and integer literals,
so no import ever needs pruning.

The report only fires when the effective language version is at least
go1.23 (slices.SortedStableFunc, slices.Sorted and iter all appeared
there) — the same gate PS3012/PS3016/PS3026 apply; in practice code
containing this pattern already compiles only on go1.23+.`,
		Before: `out := slices.SortedStableFunc(seq, func(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
})`,
		After: `out := slices.Sorted(seq)`,
		MeasuredWin: `BenchmarkPS3029 (a 4096-element scattered iter.Seq[int]
collected and sorted per op, Apple M2 Pro, go1.26): ~365 µs/op before vs
~125 µs/op after — about 2.9x — identical allocations either way
(128 KB, 19 allocs: the shared slices.Collect growth), so the delta is
pure sort work. It stacks the family's two savings: the indirect
comparator call plus the second relational comparison per element pair
that the monomorphized slices.Sort inlines away (PS3026's share on the
same input), and the stable insertion-run/symMerge machinery that the
unstable pdqsort skips entirely (PS3016's share).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3029",
		Doc:  "slices.SortedStableFunc with a hand-rolled three-way comparator instead of slices.Sorted",
		Run:  runPS3029,
	},
})

func runPS3029(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3012SortedAvailable(pass, f) {
			// slices.Sorted/SortedStableFunc exist only from go1.23 on;
			// below that the advice is moot, so stay silent entirely (same
			// per-file gate as PS3012/PS3016/PS3026).
			continue
		}
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
			// The comparator must be a fresh literal that IS the exact
			// hand-rolled three-way (less -> negative literal, greater ->
			// positive literal, default -> literal 0) — the shared PS3013
			// matcher; anything looser is not provably slices.Sorted and is
			// never reported. (The bare-cmp.Compare spelling under this
			// callee is PS3016's, the strings.Compare spelling PS3018's.)
			lit, ok := ps2110Unparen(call.Args[1]).(*ast.FuncLit)
			if !ok || !ps3013ThreeWay(pass, lit) {
				return true
			}
			// int/string fixable (equal values are bitwise-identical, so
			// dropping the stable tie-order guarantee is unobservable),
			// float and type-parameter advisory — the identical
			// classification (and identical divergence shapes) as
			// PS3016/PS3017/PS3026.
			elem, fixable, why := ps3013Elem(pass, lit)
			order := " collects and sorts the " + elem + " elements with the identical ascending order, a single inlined comparison and no stability cost"
			if why != "" {
				// The advisory reasons (float, type parameter) are exactly
				// the cases where "identical" cannot be claimed — a NaN
				// makes this comparator inconsistent while slices.Sort
				// orders NaNs first, and the stable sort's tie order is
				// observable — so the message drops the claim.
				order = " collects and sorts the " + elem + " elements ascending with a single inlined comparison and no stability cost"
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "slices.SortedStableFunc with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) pays an indirect comparator call plus up to two relational comparisons per comparison and the stable sort's merge overhead; slices.Sorted" + order + why,
			}
			if fixable {
				// Same rewrite mechanics as PS3026's SortedFunc fix: only
				// the selector name changes and the comparator goes; the
				// literal references nothing but its parameters and integer
				// literals, so no import edit is ever needed.
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

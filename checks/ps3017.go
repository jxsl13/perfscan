package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3017 reports an ascending slices.SortStableFunc whose comparator is a
// hand-rolled three-way if/switch chain (a<b -> negative, a>b -> positive,
// else 0) — slices.Sort spelled the slow, STABLE way — and rewrites it to
// slices.Sort. Sibling of PS3013, which catches the identical hand-rolled
// comparator under the unstable SortFunc, and of PS3006, which catches the
// cmp.Compare spelling under SortStableFunc: PS3017 is the remaining cell
// of that 2x2 — the PRE-cmp hand-rolled spelling that survives in code
// migrated from sort.SliceStable, additionally paying the stable
// algorithm's merge overhead.
var PS3017 = register(&lint.Check{
	ID:       "PS3017",
	Category: "indirect",
	Slug:     "sortstablefunc-handrolled-threeway-to-slices-sort",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.SortStableFunc with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) is slices.Sort spelled the slow, stable way",
		Text: `slices.SortStableFunc(s, func(a, b T) int { if a < b { return -1 };
if a > b { return 1 }; return 0 }) sorts ascending — exactly what
slices.Sort(s) does — but pays THREE separate costs per one of its
O(n log n) comparisons. The comparator is a func value invoked through
an indirect call; its body performs up to TWO relational comparisons
(a<b, then a>b) to synthesize a -1/0/1 sign the sort only ever consumes
as a bool; and SortStableFunc runs the STABLE algorithm —
insertion-sorted runs merged by a rotation-based symMerge — which does
strictly more work than pdqsort to preserve the relative order of equal
elements. slices.Sort is a distinct monomorphized entry point that
inlines a SINGLE ordered '<' (via cmp.Less) directly into pdqsort: same
ascending order, zero comparator indirection, one comparison instead of
up to two, and no stability overhead. This stacks the ~1.6x
comparator-inlining win PS3013 measures for the SortFunc spelling on
top of the stable-machinery saving PS3006 measures for the cmp.Compare
spelling.

The result is BIT-IDENTICAL for the element types the fix accepts —
byte-for-byte the same policy PS3013 and PS3006 ship: the fix is
offered only when the element type's underlying type is an integer or
string kind. For those types the if-chain returns a value whose SIGN
equals cmp.Compare(a, b) on every pair (a<b -> negative, a>b ->
positive, equal -> 0), and the sort consumes only that sign;
slices.Sort is defined via cmp.Less with cmp.Compare(a, b) < 0 iff
cmp.Less(a, b), so both sides answer every comparison the same and
produce the same non-tie permutation. Stability is the only remaining
difference, and for integer/string elements any two values comparing
equal are indistinguishable (integers bitwise so; equal strings are
interchangeable in every safe-Go operation), so the stable sort's
tie-order guarantee buys nothing observable and the unstable rewrite's
output slice is byte-for-byte identical. '<'/'>' on integers and
strings are side-effect-free and non-overloadable; no
String()/Error()/Format() method is ever consulted by ordering. Named
element types (type Celsius int) are fixed too.

FLOAT elements are reported ADVISORY only, never auto-fixed — and here
BOTH failure modes of the family bite at once: the hand-rolled chain
answers 0 for a NaN against ANYTHING ('<' and '>' are both false),
i.e. it is not even a consistent ordering, while slices.Sort orders
NaNs first; and -0.0/+0.0 (and NaNs with distinct payloads) compare
equal but differ in bits, ties SortStableFunc contractually KEEPS in
their original order while the unstable rewrite may arrange them either
way. A TYPE-PARAMETER element is likewise advisory: its instantiations
may include floats.

The match is deliberately EXACT — the same ps3013 matcher. The
comparator must be a fresh func literal whose whole body is the
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

The fix replaces only the SortStableFunc selector name with Sort and
deletes the comparator argument: the target slice expression is kept
VERBATIM in place (single evaluation preserved), the package qualifier
keeps whatever alias the file used, and an explicit instantiation
slices.SortStableFunc[S, E](...) keeps its brackets — slices.Sort has
the same two type parameters, and every fixable E (integer/string
underlying) satisfies its cmp.Ordered constraint. A comment anywhere in
the deleted span (the comparator body included) keeps the report
advisory rather than destroy it. The comparator literal references
nothing but its own parameters and integer literals, so no import ever
needs pruning.

The report only fires when the effective language version is at least
go1.21 (slices.SortStableFunc and slices.Sort appeared there) — the
same gate PS3104/PS3105/PS3107 apply; in practice code containing this
pattern already compiles only on go1.21+.`,
		Before: `slices.SortStableFunc(s, func(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
})`,
		After: `slices.Sort(s)`,
		MeasuredWin: `BenchmarkPS3017 (a shuffled 4096-element []int copied and
sorted per op, Apple M2 Pro, go1.26): 444 µs/op before vs 121 µs/op
after — about 3.7x — 0 allocs either way. The delta stacks the family's
two savings: the indirect comparator call plus the second relational
comparison per element pair that the monomorphized slices.Sort inlines
away (PS3013's ~1.6x share on the same input), and the stable
insertion-run/symMerge machinery that the unstable pdqsort skips
entirely (PS3006's share).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3017",
		Doc:  "slices.SortStableFunc with a hand-rolled three-way comparator instead of slices.Sort",
		Run:  runPS3017,
	},
})

func runPS3017(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.Sort exists only from go1.21 on; below that the
			// advice is moot, so stay silent entirely (same gate as
			// PS3104/PS3105/PS3107).
			continue
		}
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
			// The comparator must be a fresh literal that IS the exact
			// hand-rolled three-way (less -> negative literal, greater ->
			// positive literal, default -> literal 0) — anything looser is
			// not provably slices.Sort and is never reported. (Shared
			// matcher with PS3013; the bare-cmp.Compare spelling under
			// SortStableFunc is PS3006's.)
			lit, ok := ps2110Unparen(call.Args[1]).(*ast.FuncLit)
			if !ok || !ps3013ThreeWay(pass, lit) {
				return true
			}
			elem, fixable, why := ps3013Elem(pass, lit)
			order := " sorts the " + elem + " elements with the identical ascending order, a single inlined comparison and no stability cost"
			if why != "" {
				// The advisory reasons (float, type parameter) are exactly
				// the cases where "identical" cannot be claimed — a NaN
				// makes this comparator inconsistent while slices.Sort
				// orders NaNs first, and the stable sort's tie order is
				// observable — so the message drops the claim.
				order = " sorts the " + elem + " elements ascending with a single inlined comparison and no stability cost"
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "slices.SortStableFunc with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) pays an indirect comparator call plus up to two relational comparisons per comparison and the stable sort's merge overhead; slices.Sort" + order + why,
			}
			if fixable {
				if fix := ps3013Fix(f, call, sel); fix != nil {
					diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
				}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

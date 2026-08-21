package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3028 reports a slices.BinarySearchFunc whose comparator is a hand-rolled
// three-way if/switch chain (a<b -> negative, a>b -> positive, else 0) —
// slices.BinarySearch spelled the slow way — and rewrites it to
// slices.BinarySearch. Sibling of PS3109 (the same shape with a cmp.Compare
// comparator) and PS3011 (the strings.Compare spelling), and the binary-search
// member of the hand-rolled-three-way family PS3013 (SortFunc) and PS3027
// (CompareFunc) cover for the sort and lexicographic-compare consumers.
var PS3028 = register(&lint.Check{
	ID:       "PS3028",
	Category: "indirect",
	Slug:     "binarysearchfunc-handrolled-threeway-to-slices-binarysearch",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.BinarySearchFunc with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) is slices.BinarySearch spelled the slow way",
		Text: `slices.BinarySearchFunc(x, target, func(a, b T) int { if a < b { return -1 };
if a > b { return 1 }; return 0 }) searches for target exactly as
slices.BinarySearch(x, target) does, but pays for it twice on every one of the
log n probes: the comparator is a func value invoked through an indirect call,
and its body performs up to TWO relational comparisons (a<b, then a>b) to
synthesize a -1/0/+1 sign that the search only ever consumes as '< 0' (the
probe branch) and '== 0' (the found check). slices.BinarySearch is a distinct
monomorphized entry point that inlines a SINGLE ordered comparison (cmp.Less)
directly into the probe loop: same probe indices, zero comparator indirection,
one comparison instead of up to two. PS3109 catches this anti-pattern spelled
with cmp.Compare; PS3028 catches the PRE-cmp hand-rolled spelling that
survives in code migrated from sort.Search.

The result is BIT-IDENTICAL for the element types the fix accepts — exactly
PS3013's policy: the fix is offered only when the element type's underlying
type is an integer or string kind. BinarySearchFunc consumes the comparator
result ONLY by sign — cmp(x[h], target) < 0 decides each probe and
cmp(x[i], target) == 0 decides found — so, unlike slices.CompareFunc's
verbatim value propagation (PS3027), the ±magnitude of the hand-rolled
literals is irrelevant and any negative/positive pair matches. For an integer
or string element the chain's sign equals cmp.Compare(a, b) on every pair
(a<b -> negative, a>b -> positive, equal -> 0), and slices.BinarySearch's own
loop branches on cmp.Less(x[h], target) — true iff the chain is negative —
and reports found via x[i] == target — true iff the chain is 0. Every probe
therefore branches identically and the returned (index, found) pair is
byte-identical, even on unsorted (garbage) input where the answer is
meaningless but still deterministic. '<'/'>' on integers and strings are
side-effect-free and non-overloadable; no String()/Error()/Format() method is
ever consulted. Named element types (type Version int) are fixed too: ~int
satisfies BinarySearch's cmp.Ordered constraint. Because the chain compares
its two parameters with '<', the element and target types are necessarily
identical, so the rewrite always type-checks against BinarySearch's
single-type (x []E, target E) signature.

FLOAT elements are reported ADVISORY only, never auto-fixed — the same NaN
caveat PS3013 documents, but here it flips the FOUND bit, not just an
ordering: the chain answers 0 for a NaN against ANYTHING ('<' and '>' are
both false), so BinarySearchFunc stops at the first NaN probe and reports
found=true there (cmp == 0), while slices.BinarySearch orders NaN first via
cmp.Less and reports found only for x[i] == target (or both NaN) — e.g.
searching 1.0 in []float64{NaN} answers (0, true) vs (0, false). This differs
from PS3109, whose bare-cmp.Compare comparator IS BinarySearch's own ordering
and is therefore float-fixable; the hand-rolled chain is not. A
TYPE-PARAMETER element is likewise advisory: its instantiations may include
floats.

The match is deliberately EXACT — the shared PS3013 three-way matcher. The
comparator must be a fresh func literal whose whole body is the ascending
three-way chain over the two BARE parameters, resolved by object identity, in
any of the equivalent spellings: two sequential ifs plus a trailing return,
an if/else-if chain (default as trailing return or final else), or an
expressionless switch (default clause or trailing return). Each condition
must be a single '<' or '>' between the two parameters — a<b and b>a both
mean "less" — with the "less" branch returning a NEGATIVE integer literal
and the "greater" branch a POSITIVE one (magnitude irrelevant: only the sign
is consumed), and the default returning literal 0. The parameter ORDER is
load-bearing and checked by object identity: BinarySearchFunc calls
cmp(element, target), so the chain must order (first, second) ascending — a
swapped sign pair is a DESCENDING search over descending data and is never
matched. Anything looser stays silent: a subtraction comparator (return a-b)
can overflow; '<='/'>=' are not the three-way; a field selector, a captured
variable, a named constant, extra statements, or a named comparator value all
fail the match. Only integer literals (with an optional unary sign) are
accepted as returns, so deleting the comparator can never orphan an import or
any other reference.

The fix replaces only the BinarySearchFunc selector name with BinarySearch
and deletes the comparator argument: the slice and target expressions are
kept VERBATIM in place (single evaluation preserved) and the package
qualifier keeps whatever alias the file used. An explicit instantiation
slices.BinarySearchFunc[S, E, T] carries THREE type arguments where
BinarySearch has only two type parameters, so those brackets do not transfer
and such a site stays advisory (same restriction as PS3109/PS3011). A comment
anywhere in the deleted span (the comparator body included) keeps the report
advisory rather than destroy it. The report fires only from go1.21 on
(slices.BinarySearch and slices.BinarySearchFunc appeared there) — the same
gate as PS3013/PS3109; in practice code containing this pattern already
compiles only on go1.21+.`,
		Before: `i, ok := slices.BinarySearchFunc(nums, target, func(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
})`,
		After: `i, ok := slices.BinarySearch(nums, target)`,
		MeasuredWin: `BenchmarkPS3028 (a 4096-element sorted []int, targets cycled
across the full range so every search runs a full log n probes, Apple M2 Pro,
go1.26): ~50 ns/op before vs ~31 ns/op after, ~1.6x, 0 allocs either way. The
delta is pure comparison overhead — the indirect comparator call plus the up
to two relational comparisons inside the hand-rolled chain, on each probe,
that the monomorphized slices.BinarySearch replaces with one inlined
comparison. Same measured character as PS3109's cmp.Compare spelling of the
identical anti-pattern (~57 vs ~32 ns/op on the same shape).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3028",
		Doc:  "slices.BinarySearchFunc with a hand-rolled three-way comparator instead of slices.BinarySearch",
		Run:  runPS3028,
	},
})

func runPS3028(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.BinarySearch exists only from go1.21 on (same gate as
			// PS3013/PS3104/PS3107/PS3109); below that the advice is moot.
			continue
		}
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
			// The comparator must be a fresh literal that IS the exact
			// hand-rolled ascending three-way (the shared PS3013 matcher:
			// less -> negative literal, greater -> positive literal, default
			// -> literal 0); anything looser is not provably the natural
			// order and is never reported. A bare cmp.Compare comparator is
			// PS3109's territory, a bare strings.Compare one PS3011's.
			lit, ok := ps2110Unparen(call.Args[2]).(*ast.FuncLit)
			if !ok || !ps3013ThreeWay(pass, lit) {
				return true
			}
			elem, fixableElem, why := ps3028Elem(pass, lit)
			result := " searches the " + elem + " elements with the identical (index, found) result and the comparison inlined"
			if why != "" {
				// The advisory-element reasons (float, type parameter,
				// unresolved) are exactly the cases where "identical" cannot
				// be claimed — a NaN makes this comparator answer 0 against
				// anything, flipping the found bit — so the message drops the
				// claim.
				result = " searches the " + elem + " elements with the comparison inlined"
			}
			var fix *analysis.SuggestedFix
			if fixableElem && !ps3109ExplicitInst(call.Fun) {
				// The rewrite itself is PS3109's: only the selector name and
				// the text after the target argument change; the comparator
				// references nothing but its own parameters and integer
				// literals, so no import edit is ever needed.
				fix = ps3109Fix(f, call, sel)
			}
			if fixableElem && fix == nil {
				// The result is still identical for these shape corners; only
				// the mechanical rewrite is withheld.
				why = " (no auto-fix: an explicit instantiation's three type arguments do not transfer to BinarySearch's two, or a comment in the deleted span)"
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "slices.BinarySearchFunc with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) pays an indirect comparator call plus up to two relational comparisons per probe; slices.BinarySearch" + result + why,
			}
			if fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps3028Elem inspects the comparator's element type and classifies the site:
// fixable when the underlying type is an integer or string kind (the chain's
// sign then equals cmp.Compare on every pair, so every probe and the found
// check answer identically), advisory with a reason suffix for floats (the
// chain answers 0 for NaN against anything, so BinarySearchFunc reports
// found=true at a NaN probe while slices.BinarySearch orders NaN first) and
// type parameters (instantiations may include floats). The chain compares its
// two parameters with '<'/'>', so both parameters — BinarySearchFunc's E and
// T — necessarily have the same type and the single-type rewrite type-checks.
func ps3028Elem(pass *analysis.Pass, lit *ast.FuncLit) (elem string, fixable bool, why string) {
	sig, ok := pass.TypesInfo.TypeOf(lit).(*types.Signature)
	if !ok || sig.Params().Len() != 2 {
		return "searched", false, " (no auto-fix: comparator type unresolved)"
	}
	t := sig.Params().At(0).Type()
	elem = types.TypeString(t, types.RelativeTo(pass.Pkg))
	if _, isParam := t.(*types.TypeParam); isParam {
		return elem, false, " (no auto-fix: type-parameter element, instantiations may include floats — this comparator answers 0 for NaN against anything, flipping the found bit, while slices.BinarySearch orders NaN first)"
	}
	b, isBasic := t.Underlying().(*types.Basic)
	if !isBasic {
		return elem, false, " (no auto-fix: element type unresolved)"
	}
	switch {
	case b.Info()&(types.IsInteger|types.IsString) != 0:
		return elem, true, ""
	case b.Info()&types.IsFloat != 0:
		return elem, false, " (no auto-fix: float elements — NaN compares neither < nor >, so this comparator answers 0 for NaN against anything and reports found at a NaN probe while slices.BinarySearch orders NaN first)"
	default:
		return elem, false, " (no auto-fix: element type unresolved)"
	}
}

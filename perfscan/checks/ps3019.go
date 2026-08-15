package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3019 reports a slices.IsSortedFunc whose comparator is a hand-rolled
// three-way if/switch chain (a<b -> negative, a>b -> positive, else 0) —
// slices.IsSorted spelled the slow way — and rewrites it to
// slices.IsSorted. Sibling of PS3010/PS3014, which catch the same
// anti-pattern spelled with cmp.Compare/strings.Compare, and of PS3013,
// which catches this PRE-cmp hand-rolled spelling on SortFunc; the matcher
// is PS3013's, the rewrite PS3010's.
var PS3019 = register(&lint.Check{
	ID:       "PS3019",
	Category: "indirect",
	Slug:     "issortedfunc-handrolled-threeway-to-slices-issorted",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.IsSortedFunc with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) is slices.IsSorted spelled the slow way",
		Text: `slices.IsSortedFunc(s, func(a, b T) int { if a < b { return -1 };
if a > b { return 1 }; return 0 }) scans s once and reports whether it
is ascending — exactly what slices.IsSorted(s) does — but pays for the
answer twice per adjacent pair: the comparator is a func value invoked
through an indirect call on every one of the O(n) pair checks, and its
body performs up to TWO relational comparisons (a<b, then a>b) to
synthesize a -1/0/1 sign that the scan only ever consumes as the bool
'cmp(x[i], x[i-1]) < 0'. slices.IsSorted is a distinct monomorphized
entry point that inlines a SINGLE ordered comparison (cmp.Less) directly
into the scan: same O(n) single pass, same early return at the first
out-of-order pair, zero comparator indirection, one comparison instead
of up to two.

The returned bool is BIT-IDENTICAL for the element types the fix
accepts — exactly PS3013's policy: the fix is offered only when the
element type's underlying type is an integer or string kind. For those
types the if-chain returns a negative value iff a < b, so
'cmp(a, b) < 0' — the predicate IsSortedFunc evaluates per adjacent
pair — equals 'a < b', which is precisely the cmp.Less predicate
slices.IsSorted's descending scan 'if cmp.Less(x[i], x[i-1]) return
false' branches on. Every iteration answers identically, so the loop
returns the identical bool at the identical point. '<'/'>' on integers
and strings are side-effect-free and non-overloadable; no
String()/Error()/Format() method is ever consulted. Named element types
(type Celsius int) are fixed too. Strings order by raw bytes on both
sides; invalid UTF-8 is irrelevant.

FLOAT elements are reported ADVISORY only, never auto-fixed — and
unlike the cmp.Compare spelling (PS3010, where floats ARE fixable) the
bool itself can differ: a NaN compares neither '<' nor '>', so the
hand-rolled chain answers 0 (equal) for a NaN against ANYTHING and the
scan sails past it, while slices.IsSorted orders NaNs first —
IsSortedFunc([1.0, NaN], chain) is true, slices.IsSorted([1.0, NaN]) is
false. A TYPE-PARAMETER element is likewise advisory: its
instantiations may include floats.

The match is deliberately EXACT — the shared PS3013 matcher. The
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
looser stays silent: a subtraction comparator (return a - b) can
overflow; a swapped sign pair asks whether s is DESCENDING;
'<='/'>=' are not the three-way; a field selector (a.f < b.f), a
captured variable, a named constant, extra statements, or a named
comparator value all fail the match. Only returns that are integer
literals (with an optional unary sign) match, so deleting the
comparator can never orphan an import or any other reference.

The fix replaces only the IsSortedFunc selector name with IsSorted and
deletes the comparator argument: the slice expression is kept VERBATIM
in place (single evaluation preserved), the package qualifier keeps
whatever alias the file used, and an explicit instantiation
slices.IsSortedFunc[S, E](...) keeps its brackets — slices.IsSorted has
the same two type parameters, and every fixable E (integer/string
underlying) satisfies its cmp.Ordered constraint. A comment anywhere in
the deleted span (the comparator body included) keeps the report
advisory rather than destroy it. The comparator literal references
nothing but its own parameters and integer literals, so no import ever
needs pruning.

The report only fires when the effective language version is at least
go1.21 (slices.IsSortedFunc and slices.IsSorted appeared there) — the
same gate PS3010/PS3013/PS3104/PS3107 apply; in practice code
containing this pattern already compiles only on go1.21+.`,
		Before: `ok := slices.IsSortedFunc(s, func(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
})`,
		After: `ok := slices.IsSorted(s)`,
		MeasuredWin: `BenchmarkPS3019 (a sorted 4096-element []int scanned per op —
sorted input forces the full O(n) pass, Apple M2 Pro, go1.26): ~5.0 µs/op
before vs ~1.3 µs/op after, ~3.8x, 0 allocs either way. The delta is pure
comparison overhead — the indirect comparator call plus the up-to-two
relational comparisons inside the literal on each adjacent pair, which the
monomorphized slices.IsSorted inlines into a direct '<' scan. Same measured
character as PS3010's cmp.Compare spelling of the identical anti-pattern.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3019",
		Doc:  "slices.IsSortedFunc with a hand-rolled three-way comparator instead of slices.IsSorted",
		Run:  runPS3019,
	},
})

func runPS3019(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.IsSorted exists only from go1.21 on; below that the
			// advice is moot, so stay silent entirely (same gate as
			// PS3010/PS3013/PS3104/PS3107).
			continue
		}
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
			// type parameters).
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "IsSortedFunc")
			if !ok {
				return true
			}
			// The comparator must be a fresh literal that IS the exact
			// hand-rolled three-way (less -> negative literal, greater ->
			// positive literal, default -> literal 0) — anything looser is
			// not provably slices.IsSorted and is never reported. Shared
			// matcher with PS3013.
			lit, ok := ps2110Unparen(call.Args[1]).(*ast.FuncLit)
			if !ok || !ps3013ThreeWay(pass, lit) {
				return true
			}
			elem, fixable, why := ps3019Elem(pass, lit)
			answer := " answers the identical bool over the " + elem + " elements with a single inlined comparison"
			if why != "" {
				// The advisory reasons (float, type parameter) are exactly
				// the cases where "identical" cannot be claimed — a NaN
				// makes this comparator call NaN equal to everything while
				// slices.IsSorted orders NaNs first — so the message drops
				// the claim.
				answer = " scans the " + elem + " elements with a single inlined comparison"
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "slices.IsSortedFunc with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) pays an indirect comparator call plus up to two relational comparisons per adjacent pair; slices.IsSorted" + answer + why,
			}
			if fixable {
				// The rewrite itself is PS3010's: only the selector name and
				// the text after the slice argument change; the comparator
				// references nothing but its parameters and integer
				// literals, so no import edit is ever needed.
				if fix := ps3010Fix(f, call, sel); fix != nil {
					diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
				}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps3019Elem inspects the comparator's element type and classifies the
// site: fixable when the underlying type is an integer or string kind (the
// chain's 'cmp(a,b) < 0' equals 'a < b' equals cmp.Less, so the scanned
// bool is identical), advisory with a reason suffix for floats (a NaN
// compares neither '<' nor '>', so the chain calls NaN equal to everything
// and the scan can answer true where slices.IsSorted, which orders NaNs
// first, answers false) and type parameters (instantiations may include
// floats). Same policy as ps3013Elem, with the IsSorted-shaped reasons.
func ps3019Elem(pass *analysis.Pass, lit *ast.FuncLit) (elem string, fixable bool, why string) {
	sig, ok := pass.TypesInfo.TypeOf(lit).(*types.Signature)
	if !ok || sig.Params().Len() != 2 {
		return "", false, " (no auto-fix: comparator type unresolved)"
	}
	t := sig.Params().At(0).Type()
	elem = types.TypeString(t, types.RelativeTo(pass.Pkg))
	if _, isParam := t.(*types.TypeParam); isParam {
		return elem, false, " (no auto-fix: type-parameter element, instantiations may include floats — a NaN makes this comparator call NaN equal to everything while slices.IsSorted orders NaNs first, so the bool can differ)"
	}
	b, isBasic := t.Underlying().(*types.Basic)
	if !isBasic {
		return elem, false, " (no auto-fix: element type unresolved)"
	}
	switch {
	case b.Info()&(types.IsInteger|types.IsString) != 0:
		return elem, true, ""
	case b.Info()&types.IsFloat != 0:
		return elem, false, " (no auto-fix: float elements — a NaN compares neither < nor >, so this comparator calls NaN equal to everything and the scan can answer true where slices.IsSorted, which orders NaNs first, answers false)"
	default:
		return elem, false, " (no auto-fix: element type unresolved)"
	}
}

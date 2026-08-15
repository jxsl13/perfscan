package checks

import (
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3027 reports a slices.CompareFunc whose comparator is a hand-rolled
// three-way if/switch chain returning EXACTLY -1/+1/0 (a<b -> -1, a>b -> +1,
// else 0) — slices.Compare spelled the slow way — and rewrites it to the
// direct form. Sibling of PS3023 (CompareFunc with a bare cmp.Compare
// comparator): the same anti-pattern in the PRE-cmp hand-rolled spelling
// PS3013/PS3017/PS3019/PS3022 catch for the sort and extremum families.
var PS3027 = register(&lint.Check{
	ID:       "PS3027",
	Category: "indirect",
	Slug:     "comparefunc-handrolled-threeway-to-slices-compare",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.CompareFunc with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) is slices.Compare spelled the slow way",
		Text: `slices.CompareFunc(s1, s2, func(a, b T) int { if a < b { return -1 };
if a > b { return 1 }; return 0 }) compares the two slices lexicographically —
exactly what slices.Compare(s1, s2) does — but pays for it twice per element
pair: the comparator is a func value invoked through an indirect call at every
position up to min(len(s1), len(s2)), and its body performs up to TWO
relational comparisons (a<b, then a>b) to synthesize the -1/0/+1 three-way.
slices.Compare is a distinct monomorphized entry point whose body IS the same
loop with the comparator fixed to an inlined cmp.Compare(s1[i], s2[i]): same
early exit, same ±1 length tie-breaks, zero comparator indirection. PS3023
catches this anti-pattern spelled with cmp.Compare; PS3027 catches the PRE-cmp
hand-rolled spelling that survives in code migrated from manual loops.

The magnitude guard is STRICTER than the sort/extremum siblings, because the
consumer is stricter: sort.Slice and slices.MaxFunc read only the SIGN of the
comparator's result, so PS3013/PS3022 accept any negative/positive literal
magnitude — but slices.CompareFunc returns the comparator's first non-zero
result VERBATIM. A ladder returning -2/+3 makes CompareFunc yield -2/+3 where
slices.Compare yields cmp.Compare's canonical -1/+1 — the returned VALUE, not
just its sign, would change. PS3027 therefore matches only a ladder whose
"less" arm returns exactly -1 and whose "greater" arm returns exactly +1
(checked on the type-checked constant value, so -(1) and +1 count and -2 never
slips through); any other magnitude keeps the report advisory.

With that guard, the fix is offered when the element type's underlying type is
an INTEGER kind: each per-position ladder result then equals
cmp.Compare(a, b) exactly (integers admit no NaN, so cmp.Compare's NaN
prelude is dead and its -1/+1/0 answers are the ladder's), the trailing
length comparison is byte-identical in both entry points, and the comparator
references only its two bare parameters and integer literals — no side
effects, no differing evaluation counts, no import to prune. Named element
types (type Rank int) are fixed too: ~int satisfies slices.Compare's
cmp.Ordered constraint and no method of the element is ever consulted.

FLOAT elements are reported ADVISORY only, never auto-fixed: the ladder's
else branch answers 0 for a NaN against ANYTHING ('<' and '>' are both
false), so CompareFunc treats every NaN position as a tie and scans on, while
slices.Compare runs cmp.Compare per pair, which orders NaN FIRST — the two
disagree on any slice containing NaN (e.g. []float64{NaN} vs []float64{1}: 0
after the length tie-break vs -1). STRING elements are bit-identical (strings
admit no NaN, so the ladder is cmp.Compare exactly) but stay advisory too:
slices.Compare's generic cmp.Compare instantiation performs two isNaN
self-comparisons per pair that the devirtualized hand-rolled ladder omits —
measured parity at best, no win to buy — matching the family's string
carve-outs (PS3022). A TYPE-PARAMETER element is likewise advisory: its
instantiations may include floats.

Two shape corners also keep the report advisory, exactly as in PS3023. MIXED
SLICE TYPES: slices.CompareFunc takes two independently-typed slices (S1,
S2), while slices.Compare takes one S for both — comparing a []int against a
named IntSlice compiles only in the Func spelling, so the fix requires the
two slice arguments to have IDENTICAL types. EXPLICIT INSTANTIATION:
slices.CompareFunc[S1, S2, E1, E2](...) has four type arguments where
slices.Compare has two, so the brackets cannot survive the rewrite and the
site is left to the human.

The match is deliberately EXACT (the shared PS3013 three-way matcher plus the
unit-magnitude tightening): the comparator must be a fresh func literal whose
whole body is the ascending three-way chain over the two BARE parameters,
resolved by object identity, in any of the equivalent spellings — two
sequential ifs plus a trailing return, an if/else-if chain (default as
trailing return or final else), or an expressionless switch (default clause
or trailing return). Each condition must be a single '<' or '>' between the
two parameters (a<b and b>a both mean "less"), the "less" arm returning
exactly -1, the "greater" arm exactly +1, and the default exactly literal 0.
Anything looser stays silent: a subtraction comparator (return a - b) can
overflow; a swapped sign pair is the REVERSED comparison; '<='/'>=' are not
the three-way; a field selector, a captured variable, a named constant, extra
statements, or a named comparator value all fail the match. Only integer
literals (with an optional unary sign) are accepted as returns, so deleting
the comparator can never orphan an import or any other reference.

The fix replaces only the CompareFunc selector name with Compare and deletes
the comparator argument: both slice expressions are kept VERBATIM in place
(single evaluation, source order preserved) and the package qualifier keeps
whatever alias the file used. A comment anywhere in the deleted span (the
comparator body included) keeps the report advisory rather than destroy it.
Fires only from go1.21 on (slices.Compare and slices.CompareFunc appeared
there) — the same gate as PS3013/PS3022/PS3023; in practice code containing
this pattern already compiles only on go1.21+.`,
		Before: `d := slices.CompareFunc(s1, s2, func(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
})`,
		After: `d := slices.Compare(s1, s2)`,
		MeasuredWin: `BenchmarkPS3027 (two 4096-element []int slices differing only
in the last element — a full-length lexicographic scan — per op, Apple M2
Pro, gc 1.26): ~3.9 µs/op before vs ~3.9 µs/op after — parity, 0 allocs
either way. gc inlines slices.CompareFunc and devirtualizes the fresh
comparator literal, so both become the same direct-comparison scan; the win
is source-level robustness (the branch ladder goes away and slices.Compare
cannot fall off the devirtualization path a hoisted or grown callback can)
plus a real per-pair indirect call and second relational comparison removed
on toolchains without closure devirtualization (gccgo, tinygo, older gc) —
the same measured character as PS3023 and the PS3013/PS3022 siblings.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3027",
		Doc:  "slices.CompareFunc with a hand-rolled three-way comparator instead of slices.Compare",
		Run:  runPS3027,
	},
})

func runPS3027(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			continue // slices.Compare exists only from go1.21 on (same gate as PS3023).
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 || call.Ellipsis.IsValid() {
				return true
			}
			// The callee must be the receiver-less package function
			// slices.CompareFunc, resolved through type info — never a
			// shadowed slices or a same-named method. An explicit
			// instantiation is unwrapped for matching but keeps the site
			// advisory (four type parameters vs Compare's two).
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "CompareFunc")
			if !ok {
				return true
			}
			// The comparator must be a fresh literal that IS the exact
			// hand-rolled ascending three-way (the shared PS3013 matcher);
			// anything looser is not provably the natural order and is never
			// reported. A bare cmp.Compare comparator is PS3023's territory.
			lit, ok := ps2110Unparen(call.Args[2]).(*ast.FuncLit)
			if !ok || !ps3013ThreeWay(pass, lit) {
				return true
			}
			elem, fixable, identical, why := ps3027Site(pass, call, lit)
			result := " compares the " + elem + " elements with the identical lexicographic result and the comparison inlined"
			if !identical {
				// The value-changing advisory reasons (non-unit magnitudes,
				// floats, type parameters) are exactly the cases where
				// "identical" cannot be claimed — the message drops the claim.
				result = " compares the " + elem + " elements with the comparison inlined"
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "slices.CompareFunc with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) pays an indirect comparator call plus up to two relational comparisons per element pair; slices.Compare" + result + why,
			}
			if fixable {
				if fix := ps3027Fix(f, call, sel); fix != nil {
					diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
				}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps3027Site classifies one matched call: fixable when the rewrite provably
// typechecks AND is byte-identical; identical reports whether the message may
// still claim an identical result (true for the shape-corner advisories,
// false when the returned value itself could change). The guards, in order:
// the ladder's non-zero arms must return EXACTLY ±1 (slices.CompareFunc
// returns the comparator's first non-zero result VERBATIM, so any other
// magnitude changes the returned value, not just its spelling — the
// relaxation the sign-only sort/extremum consumers allow is unsound here);
// the element type must be an integer kind (no NaN, so the ladder IS
// cmp.Compare; floats diverge on NaN, strings are bit-identical but a
// no-win rewrite kept advisory like PS3022, type parameters may hide
// floats); the call must not be explicitly instantiated (CompareFunc has
// four type parameters, Compare two); and the two slice arguments must have
// IDENTICAL types (CompareFunc takes independent S1/S2, Compare one S).
func ps3027Site(pass *analysis.Pass, call *ast.CallExpr, lit *ast.FuncLit) (elem string, fixable, identical bool, why string) {
	sig, ok := pass.TypesInfo.TypeOf(lit).(*types.Signature)
	if !ok || sig.Params().Len() != 2 {
		return "", false, false, " (no auto-fix: comparator type unresolved)"
	}
	t := sig.Params().At(0).Type()
	elem = types.TypeString(t, types.RelativeTo(pass.Pkg))
	if !ps3027UnitMagnitudes(pass, lit) {
		return elem, false, false, " (no auto-fix: slices.CompareFunc returns the comparator's first non-zero result verbatim, so this ladder's non-±1 magnitudes reach the caller while slices.Compare would return the canonical -1/+1 — the returned value, not just its sign, would change)"
	}
	if _, isParam := t.(*types.TypeParam); isParam {
		return elem, false, false, " (no auto-fix: type-parameter element, instantiations may include floats whose NaN handling differs)"
	}
	b, isBasic := t.Underlying().(*types.Basic)
	if !isBasic {
		return elem, false, false, " (no auto-fix: element type unresolved)"
	}
	switch {
	case b.Info()&types.IsInteger != 0:
		// Fixable modulo the shape guards below.
	case b.Info()&types.IsString != 0:
		return elem, false, true, " (no auto-fix: bit-identical for strings, but slices.Compare's generic cmp.Compare instantiation adds two isNaN self-comparisons per pair that this devirtualized ladder omits — measured parity at best, no win to buy, matching the family's string carve-outs)"
	case b.Info()&types.IsFloat != 0:
		return elem, false, false, " (no auto-fix: slices.Compare orders NaN first via cmp.Compare while this comparator answers 0 for NaN against anything, so they differ on a NaN element)"
	default:
		return elem, false, false, " (no auto-fix: element type unresolved)"
	}
	switch ps2110Unparen(call.Fun).(type) {
	case *ast.IndexExpr, *ast.IndexListExpr:
		return elem, false, true, " (no auto-fix: explicit instantiation — slices.CompareFunc has four type parameters, slices.Compare two, so the brackets cannot survive the rewrite)"
	}
	t1, t2 := pass.TypesInfo.TypeOf(call.Args[0]), pass.TypesInfo.TypeOf(call.Args[1])
	if t1 == nil || t2 == nil {
		return elem, false, true, " (no auto-fix: slice types unresolved)"
	}
	if !types.Identical(t1, t2) {
		return elem, false, true, " (no auto-fix: the slice types " + types.TypeString(t1, types.RelativeTo(pass.Pkg)) + " and " + types.TypeString(t2, types.RelativeTo(pass.Pkg)) + " differ — slices.Compare takes one slice type for both sides)"
	}
	return elem, true, true, ""
}

// ps3027UnitMagnitudes reports whether every conditional arm of the (already
// PS3013-matched) three-way ladder returns EXACTLY ±1, read from the
// type-checked constant value — so -(1) and +1 count while -2, 3 or an
// overflowing spelling never slip through. ps3013ThreeWay already proved each
// arm's SIGN matches its direction (less -> negative, greater -> positive)
// and the default is literal 0, so unit magnitude here pins the arms to
// exactly -1 (less) and +1 (greater): each per-position result is
// cmp.Compare(a, b) verbatim, which is what slices.CompareFunc's
// value-propagating contract requires for the rewrite.
func ps3027UnitMagnitudes(pass *analysis.Pass, lit *ast.FuncLit) bool {
	arms, _ := ps3013Arms(lit.Body.List)
	if len(arms) != 2 {
		return false
	}
	for _, arm := range arms {
		tv, ok := pass.TypesInfo.Types[arm.ret]
		if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
			return false
		}
		v, exact := constant.Int64Val(tv.Value)
		if !exact || (v != 1 && v != -1) {
			return false
		}
	}
	return true
}

// ps3027Fix builds the slices.Compare(s1, s2) rewrite for one call, or nil
// when the comment guard fails and the report must stay advisory. Only the
// CompareFunc selector name and the text after the second slice argument (the
// comma, the whole comparator and the closing paren) are replaced: both slice
// expressions stay untouched in place (text and single evaluation preserved)
// and the package qualifier keeps the file's alias. The comparator references
// nothing but its parameters and integer literals, so no import edit is ever
// needed. A comment in the deleted span (the comparator body included) would
// be silently destroyed — advisory then.
func ps3027Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	if ps2111CommentIn(f, call.Args[1].End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + ps3107Qualifier(sel) + "Compare(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Compare")},
			{Pos: call.Args[1].End(), End: call.End(), NewText: []byte(")")},
		},
	}
}

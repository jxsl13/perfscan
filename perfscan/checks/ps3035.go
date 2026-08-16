package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3035 reports a slices.MaxFunc / slices.MinFunc whose comparator is a
// hand-rolled three-way if/switch chain with the SIGNS SWAPPED (a>b ->
// negative, a<b -> positive) — the comparator induces the REVERSED order, so
// the call computes the OPPOSITE extremum: slices.Min / slices.Max spelled the
// slow way — and rewrites it to the direct form. Completes the square with
// PS3020 (swapped cmp.Compare(b, a) -> opposite extremum) and PS3022
// (source-order hand-rolled ladder -> same extremum): this is the PRE-cmp
// hand-rolled spelling of PS3020's reversed order, which neither sibling
// matches (PS3022's matcher rejects a swapped sign pair by design).
var PS3035 = register(&lint.Check{
	ID:       "PS3035",
	Category: "indirect",
	Slug:     "maxfunc-swapped-handrolled-threeway-to-slices-min",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.MaxFunc/MinFunc with a swapped hand-rolled three-way comparator (a>b/-1, a<b/1) is the opposite extremum — slices.Min/Max spelled the slow way",
		Text: `slices.MaxFunc(s, func(a, b T) int { if a > b { return -1 };
if a < b { return 1 }; return 0 }) walks s once and keeps the element that is
maximal under the REVERSED order — which is exactly the minimum under the
natural order, i.e. what slices.Min(s) does — but pays for it twice per
element: the comparator is a func value invoked through an indirect call on
every element, and its body performs up to TWO relational comparisons (a>b,
then a<b) to synthesize a -1/0/1 sign that the scan only ever consumes as
"greater than zero". slices.Min is a distinct monomorphized entry point that
folds the comparison into the inlined builtin min: same single pass, zero
comparator indirection, one comparison per element. slices.MinFunc with the
swapped ladder is the symmetric case and becomes slices.Max. PS3022 catches
the SOURCE-ORDER ladder (MaxFunc -> Max) and PS3020 the swapped cmp.Compare
spelling; this check completes the square for the reversed hand-rolled
spelling, which otherwise stays on the slow path forever.

The fix is offered only when the element type's underlying type is an INTEGER
kind — exactly PS3020/PS3022's policy. The swapped ladder returns a NEGATIVE
value iff a>b, so the predicate MaxFunc branches on (cmp(new, best) > 0)
equals new < best: MaxFunc under the reversed order updates its candidate
exactly when the new element is STRICTLY smaller under the natural order and
keeps the EARLIER of any tie — precisely the scan slices.Min performs (the
builtin min also keeps the earlier operand of a tie). For integers any two
elements that compare equal are bitwise-identical values, so tie selection is
unobservable either way and the result is byte-for-byte identical. Both sides
panic on an empty slice (both index s[0] up front). Named element types
(type Priority int) are fixed too; '<'/'>' compare the underlying value and
overflow is impossible (the ladder is relational, never a subtraction).

STRING elements are bit-identical too but reported ADVISORY only, never
auto-fixed — the family's policy verbatim: slices.Min/Max on a []string fold
via an outlined runtime.strmin/strmax call per element, which benchmarks
~10-25% SLOWER on gc than the devirtualized hand-rolled loop, so the auto-fix
would trade a recognizable idiom for provably slower code and the human
decides.

FLOAT elements are reported ADVISORY only, never auto-fixed: the ladder's
default arm answers 0 for a NaN against ANYTHING ('>' and '<' are both
false), so the Func scan never selects a NaN unless one happens to sit first,
and it treats -0.0 and +0.0 as a tie (keeping the earlier) — while
slices.Min/Max are defined via the builtin min/max, which PROPAGATE NaN (any
NaN forces a NaN result) and order -0.0 below +0.0 by value. The two disagree
on any slice containing a NaN or mixed-sign zeros. A TYPE-PARAMETER element
is likewise advisory: its instantiations may include floats.

The match is deliberately EXACT — the sign-mirrored PS3013 three-way matcher:
the comparator must be a fresh func literal whose whole body is the
DESCENDING three-way chain over the two BARE parameters, resolved by object
identity, in any of the equivalent spellings — two sequential ifs plus a
trailing return, an if/else-if chain (default as trailing return or final
else), or an expressionless switch (default clause or trailing return). Each
condition must be a single '<' or '>' between the two parameters (a>b and
b<a both mean "greater"), the "greater" branch returning a NEGATIVE integer
literal and the "less" branch a POSITIVE one (magnitude irrelevant: only the
sign is consumed), and the default returning literal 0. Anything looser stays
silent: the source-order sign pair is PS3022's case and is never matched
here; a subtraction comparator (return b - a) can overflow; '<='/'>=' are
not the three-way; a field selector, a captured variable, a named constant,
extra statements, or a named comparator value all fail the match. Only
integer literals (with an optional unary sign) are accepted as returns, so
deleting the comparator can never orphan an import or any other reference.

The fix replaces the MaxFunc/MinFunc selector name with the OPPOSITE base
name (MaxFunc -> Min, MinFunc -> Max) and deletes the comparator argument:
the target slice expression is kept VERBATIM in place (single evaluation
preserved), the package qualifier keeps whatever alias the file used, and an
explicit instantiation slices.MaxFunc[S, E](...) keeps its brackets —
slices.Min has the same two type parameters, and every fixable E (integer
underlying) satisfies its cmp.Ordered constraint. A comment anywhere in the
deleted span (the comparator body included) keeps the report advisory rather
than destroy it. The comparator literal references nothing but its own
parameters and integer literals, so no import ever needs pruning.

The report only fires when the effective language version is at least go1.21
(slices.MaxFunc, slices.Max and slices.Min appeared there) — the same gate
PS3013/PS3020/PS3022 apply; in practice code containing this pattern already
compiles only on go1.21+.`,
		Before: `lo := slices.MaxFunc(xs, func(a, b int) int {
	if a > b {
		return -1
	}
	if a < b {
		return 1
	}
	return 0
})`,
		After: `lo := slices.Min(xs)`,
		MeasuredWin: `BenchmarkPS3035 (a scattered 4096-element []int scanned
per op, Apple M2 Pro, gc 1.26): ~2.6 µs/op before vs ~2.6 µs/op after —
parity, 0 allocs either way — gc inlines slices.MaxFunc and devirtualizes
the fresh literal, so both become the same direct-comparison scan. The win
is source-level robustness (the branch ladder, its sign scaffolding and the
mental order-reversal go away, and slices.Min cannot fall off the
devirtualization path a hoisted or grown callback can) plus a real
per-element indirect call and second relational comparison removed on
toolchains without closure devirtualization (gccgo, tinygo, older gc) — the
same measured character as PS3020 and PS3022.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3035",
		Doc:  "slices.MaxFunc/MinFunc with a swapped hand-rolled three-way comparator instead of slices.Min/Max",
		Run:  runPS3035,
	},
})

func runPS3035(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			continue // slices.Max/Min exist only from go1.21 on (same gate as PS3020/PS3022).
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			// The callee must be the receiver-less package function
			// slices.MaxFunc or slices.MinFunc, resolved through type info —
			// never a shadowed slices or a same-named method. An explicit
			// instantiation slices.MaxFunc[S, E] is unwrapped; its brackets
			// survive the fix (slices.Min/Max have the same two type
			// parameters). The swapped comparator reverses the order, so the
			// rewrite target is the OPPOSITE base name (ps3020Base).
			var sel *ast.SelectorExpr
			var names struct{ base, extremum string }
			for funcName, opposite := range ps3020Base {
				if s, isIt := ps3107PkgFunc(pass, call.Fun, "slices", funcName); isIt {
					sel, names = s, opposite
					break
				}
			}
			if sel == nil {
				return true
			}
			// The comparator must be a fresh literal that IS the exact
			// swapped hand-rolled three-way (greater -> negative literal,
			// less -> positive literal, default -> literal 0) — the
			// sign-mirror of the shared PS3013 matcher; the source-order
			// ladder is PS3022's case and anything looser is not provably
			// the reversed natural order.
			lit, ok := ps2110Unparen(call.Args[1]).(*ast.FuncLit)
			if !ok || !ps3035ThreeWayDesc(pass, lit) {
				return true
			}
			elem, fixable, why := ps3035Elem(pass, lit)
			result := " computes the identical " + elem + " " + names.extremum + " with the comparison inlined"
			if why != "" {
				// The advisory reasons are exactly the cases where
				// "identical" cannot be claimed (or, for strings, where the
				// rewrite is a measured regression) — the message drops the
				// claim accordingly.
				result = " computes the " + elem + " " + names.extremum + " with the comparison inlined"
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "slices." + sel.Sel.Name + " with a swapped hand-rolled three-way comparator (a>b/-1, a<b/1) selects the " + names.extremum + " through an indirect comparator call plus up to two relational comparisons per element; slices." + names.base + result + why,
			}
			if fixable {
				if fix := ps3035Fix(f, call, sel, names.base); fix != nil {
					diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
				}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps3035ThreeWayDesc reports whether lit's whole body is EXACTLY the
// DESCENDING hand-rolled three-way over its two parameters: one "greater"
// branch (a>b or b<a) returning a negative integer literal, one "less" branch
// (a<b or b>a) returning a positive one, and a default returning literal 0 —
// in the if-chain, if/else-if, or expressionless-switch spelling (the shared
// PS3013 decomposition with the sign requirement mirrored). Everything else —
// the source-order sign pair (PS3022's ascending ladder), '<='/'>=', a
// subtraction, selectors, captured variables, named constants, extra
// statements — fails the match.
func ps3035ThreeWayDesc(pass *analysis.Pass, lit *ast.FuncLit) bool {
	// Exactly two named parameters, in source order across the fields
	// (func(a, b T) and func(a T, b T) both). A blank _ parameter cannot be
	// compared in the body, so requiring resolvable names loses nothing.
	var params []*types.Var
	for _, field := range lit.Type.Params.List {
		for _, name := range field.Names {
			if name.Name == "_" {
				return false
			}
			v, isVar := pass.TypesInfo.Defs[name].(*types.Var)
			if !isVar {
				return false
			}
			params = append(params, v)
		}
	}
	if len(params) != 2 {
		return false
	}
	arms, zeroDefault := ps3013Arms(lit.Body.List)
	if len(arms) != 2 || !zeroDefault {
		return false
	}
	var haveLess, haveGreater bool
	for _, arm := range arms {
		dir, ok := ps3013Direction(pass, arm.cond, params)
		if !ok {
			return false
		}
		// The branch's returned sign must be the MIRROR of its direction:
		// less -> positive, greater -> negative. The matching pair is
		// PS3022's ascending ladder and never matched here.
		switch dir {
		case ps3013Less:
			if haveLess || !ps3013IntLit(pass, arm.ret, +1) {
				return false
			}
			haveLess = true
		case ps3013Greater:
			if haveGreater || !ps3013IntLit(pass, arm.ret, -1) {
				return false
			}
			haveGreater = true
		}
	}
	return haveLess && haveGreater
}

// ps3035Elem classifies the extremum's element type (the comparator's first
// parameter type): fixable only for an integer kind — the swapped ladder's
// sign equals cmp.Compare(b, a) on every pair, equal values are
// bitwise-identical (tie selection unobservable) and the builtin min/max
// inlines to a CMP/CSEL at least as fast, so slices.Min/Max match
// byte-for-byte. String is bit-identical too but stays ADVISORY:
// slices.Min/Max on a string slice fold via an outlined runtime.strmin/strmax
// call per element, ~10-25% slower on gc than the devirtualized hand-rolled
// loop (the family's measurement). Floats are advisory (the ladder answers 0
// for NaN against anything and calls -0.0/+0.0 a tie, while slices.Min/Max
// PROPAGATE NaN and order signed zeros via the builtin min/max) and so are
// type parameters (instantiations may include floats).
func ps3035Elem(pass *analysis.Pass, lit *ast.FuncLit) (elem string, fixable bool, why string) {
	sig, ok := pass.TypesInfo.TypeOf(lit).(*types.Signature)
	if !ok || sig.Params().Len() != 2 {
		return "", false, " (no auto-fix: comparator type unresolved)"
	}
	t := sig.Params().At(0).Type()
	elem = types.TypeString(t, types.RelativeTo(pass.Pkg))
	if _, isParam := t.(*types.TypeParam); isParam {
		return elem, false, " (no auto-fix: type-parameter element, instantiations may include floats whose NaN and signed-zero handling differ)"
	}
	b, isBasic := t.Underlying().(*types.Basic)
	if !isBasic {
		return elem, false, " (no auto-fix: element type unresolved)"
	}
	switch {
	case b.Info()&types.IsInteger != 0:
		return elem, true, ""
	case b.Info()&types.IsString != 0:
		return elem, false, " (no auto-fix: slices.Min/Max on a string slice fold to an outlined runtime.strmin/strmax call per element, ~10-25% slower on gc than the devirtualized hand-rolled loop; bit-identical but a measured perf regression)"
	case b.Info()&types.IsFloat != 0:
		return elem, false, " (no auto-fix: slices.Min/Max propagate NaN and order signed zeros via the builtin min/max while this comparator answers 0 for NaN against anything and calls -0.0/+0.0 a tie, so they differ on a NaN or signed-zero element)"
	default:
		return elem, false, " (no auto-fix: element type unresolved)"
	}
}

// ps3035Fix builds the slices.Min(s) / slices.Max(s) rewrite for one call, or
// nil when a guard fails and the report must stay advisory. Only the
// MaxFunc/MinFunc selector name (replaced with the OPPOSITE base) and the
// text after the slice argument are touched: the slice expression stays
// untouched in place (text and single evaluation preserved), the package
// qualifier keeps the file's alias, and explicit instantiation brackets
// survive — slices.Min/Max take the same two type parameters. The comparator
// references nothing but its parameters and integer literals, so no import
// edit is ever needed.
func ps3035Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr, base string) *analysis.SuggestedFix {
	// The span from the slice argument's end to the call's end — the comma,
	// the whole comparator (its body comments included) and the closing
	// parenthesis — is deleted; a comment there would be silently destroyed —
	// advisory then. (The other replaced span is the MaxFunc/MinFunc
	// identifier itself, which cannot contain a comment.)
	if ps2111CommentIn(f, call.Args[0].End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + ps3107Qualifier(sel) + base + "(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte(base)},
			{Pos: call.Args[0].End(), End: call.End(), NewText: []byte(")")},
		},
	}
}

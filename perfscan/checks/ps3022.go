package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3022 reports a slices.MaxFunc / slices.MinFunc whose comparator is a
// hand-rolled three-way if/switch chain (a<b -> negative, a>b -> positive,
// else 0) — slices.Max / slices.Min spelled the slow way — and rewrites it to
// the direct form. Sibling of PS3111 (MaxFunc/MinFunc with a bare cmp.Compare
// comparator): the same extremum anti-pattern in the PRE-cmp hand-rolled
// spelling PS3013/PS3017/PS3019 catch for the sort family.
var PS3022 = register(&lint.Check{
	ID:       "PS3022",
	Category: "indirect",
	Slug:     "maxfunc-handrolled-threeway-to-slices-max",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.MaxFunc/MinFunc with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) is slices.Max/Min spelled the slow way",
		Text: `slices.MaxFunc(s, func(a, b T) int { if a < b { return -1 };
if a > b { return 1 }; return 0 }) walks s once and keeps the greatest
element — exactly what slices.Max(s) does — but pays for it twice per
element: the comparator is a func value invoked through an indirect call
on every element, and its body performs up to TWO relational comparisons
(a<b, then a>b) to synthesize a -1/0/1 sign that the scan only ever
consumes as "greater than zero". slices.Max is a distinct monomorphized
entry point that folds the comparison into the inlined builtin max: same
single pass, zero comparator indirection, one comparison per element.
slices.MinFunc/slices.Min are the symmetric pair. PS3111 catches this
anti-pattern spelled with cmp.Compare; PS3022 catches the PRE-cmp
hand-rolled spelling that survives in code migrated from manual loops.

The fix is offered only when the element type's underlying type is an
INTEGER kind — exactly PS3111's policy, because for integers the
hand-rolled ladder induces the natural order byte-identically to
cmp.Compare and the site collapses to PS3111's already-proven integer
case: the ladder returns a value whose SIGN equals cmp.Compare(a, b) on
every pair, MaxFunc/MinFunc consume only that sign, any two elements
that compare equal are bitwise-identical values (so which of a tie the
scan keeps is unobservable), and both spellings panic on an empty slice.
Named element types (type Priority int) are fixed too.

STRING elements are bit-identical too but reported ADVISORY only, never
auto-fixed: slices.Max/Min on a []string fold via an outlined
runtime.strmax/strmin call per element, which benchmarks ~10-25% SLOWER
on gc than the devirtualized hand-rolled loop (the same measurement
behind PS3111's string policy). Auto-fixing would trade recognizable
code for provably slower code, so the human decides.

FLOAT elements are reported ADVISORY only, never auto-fixed — and the
divergence is even sharper than in PS3111: the ladder's else branch
answers 0 for a NaN against ANYTHING ('<' and '>' are both false), so
MaxFunc never replaces the running extremum on a NaN comparison — the
result depends on whether a NaN happens to sit first — while slices.Max
is defined via the builtin max, which PROPAGATES NaN (any NaN forces a
NaN result). The two disagree on any slice containing NaN. A
TYPE-PARAMETER element is likewise advisory: its instantiations may
include floats.

The match is deliberately EXACT (the shared PS3013 three-way matcher):
the comparator must be a fresh func literal whose whole body is the
ascending three-way chain over the two BARE parameters, resolved by
object identity, in any of the equivalent spellings — two sequential ifs
plus a trailing return, an if/else-if chain (default as trailing return
or final else), or an expressionless switch (default clause or trailing
return). Each condition must be a single '<' or '>' between the two
parameters (a<b and b>a both mean "less"), the "less" branch returning a
NEGATIVE integer literal and the "greater" branch a POSITIVE one
(magnitude irrelevant: only the sign is consumed), and the default
returning literal 0. Anything looser stays silent: a subtraction
comparator (return a - b) can overflow; a swapped sign pair is the
OPPOSITE extremum; '<='/'>=' are not the three-way; a field selector, a
captured variable, a named constant, extra statements, or a named
comparator value all fail the match. Only integer literals (with an
optional unary sign) are accepted as returns, so deleting the comparator
can never orphan an import or any other reference.

The fix replaces only the MaxFunc/MinFunc selector name with Max/Min and
deletes the comparator argument: the target slice expression is kept
VERBATIM in place (single evaluation preserved), the package qualifier
keeps whatever alias the file used, and an explicit instantiation
slices.MaxFunc[S, E](...) keeps its brackets — slices.Max has the same
two type parameters, and every fixable E (integer underlying) satisfies
its cmp.Ordered constraint. A comment anywhere in the deleted span (the
comparator body included) keeps the report advisory rather than destroy
it. The comparator literal references nothing but its own parameters and
integer literals, so no import ever needs pruning.

The report only fires when the effective language version is at least
go1.21 (slices.MaxFunc and slices.Max appeared there) — the same gate
PS3013/PS3104/PS3107/PS3111 apply; in practice code containing this
pattern already compiles only on go1.21+.`,
		Before: `hi := slices.MaxFunc(xs, func(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
})`,
		After: `hi := slices.Max(xs)`,
		MeasuredWin: `BenchmarkPS3022 (a scattered 4096-element []int scanned
per op, Apple M2 Pro, gc 1.26): ~2.60 µs/op before vs ~2.56 µs/op after
— parity, 0 allocs either way — gc
inlines slices.MaxFunc and devirtualizes the fresh literal, so both
become the same direct-comparison scan. The win is source-level
robustness (the branch ladder goes away and slices.Max cannot fall off
the devirtualization path a hoisted or grown callback can) plus a real
per-element indirect call and second relational comparison removed on
toolchains without closure devirtualization (gccgo, tinygo, older gc) —
the same measured character as PS3111 and the PS3013 sort sibling.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3022",
		Doc:  "slices.MaxFunc/MinFunc with a hand-rolled three-way comparator instead of slices.Max/Min",
		Run:  runPS3022,
	},
})

func runPS3022(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			continue // slices.Max/Min exist only from go1.21 on (same gate as PS3111).
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
			// survive the fix (slices.Max has the same two type parameters).
			var sel *ast.SelectorExpr
			var base string
			for funcName, baseName := range ps3111Base {
				if s, isIt := ps3107PkgFunc(pass, call.Fun, "slices", funcName); isIt {
					sel, base = s, baseName
					break
				}
			}
			if sel == nil {
				return true
			}
			// The comparator must be a fresh literal that IS the exact
			// hand-rolled ascending three-way (less -> negative literal,
			// greater -> positive literal, default -> literal 0) — the shared
			// PS3013 matcher; anything looser is not provably the natural
			// order and is never reported.
			lit, ok := ps2110Unparen(call.Args[1]).(*ast.FuncLit)
			if !ok || !ps3013ThreeWay(pass, lit) {
				return true
			}
			elem, fixable, why := ps3022Elem(pass, lit)
			result := " computes the extremal " + elem + " value with the identical result and the comparison inlined"
			if why != "" {
				// The advisory reasons are exactly the cases where
				// "identical" cannot be claimed (or, for strings, where the
				// rewrite is a measured regression) — the message drops or
				// qualifies the claim accordingly.
				result = " computes the extremal " + elem + " value with the comparison inlined"
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "slices." + base + "Func with a hand-rolled three-way comparator (a<b/a>b/-1/1/0) pays an indirect comparator call plus up to two relational comparisons per element; slices." + base + result + why,
			}
			if fixable {
				if fix := ps3022Fix(f, call, sel, base); fix != nil {
					diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
				}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps3022Elem classifies the extremum's element type (the comparator's first
// parameter type): fixable only for an integer kind — there the hand-rolled
// ladder induces the natural order byte-identically to cmp.Compare, equal
// values are bitwise-identical (tie selection unobservable) and the builtin
// max/min inlines to a CMP/CSEL at least as fast. String is bit-identical too
// but stays ADVISORY: slices.Max/Min on a string slice fold via an outlined
// runtime.strmax/strmin call per element, ~10-25% slower on gc than the
// devirtualized hand-rolled loop (PS3111's measurement). Floats are advisory
// (the ladder answers 0 for NaN against anything, so the Func scan's result
// depends on the NaN's position, while slices.Max/Min PROPAGATE NaN via the
// builtin max/min) and so are type parameters (instantiations may include
// floats).
func ps3022Elem(pass *analysis.Pass, lit *ast.FuncLit) (elem string, fixable bool, why string) {
	sig, ok := pass.TypesInfo.TypeOf(lit).(*types.Signature)
	if !ok || sig.Params().Len() != 2 {
		return "", false, " (no auto-fix: comparator type unresolved)"
	}
	t := sig.Params().At(0).Type()
	elem = types.TypeString(t, types.RelativeTo(pass.Pkg))
	if _, isParam := t.(*types.TypeParam); isParam {
		return elem, false, " (no auto-fix: type-parameter element, instantiations may include floats whose NaN handling differs)"
	}
	b, isBasic := t.Underlying().(*types.Basic)
	if !isBasic {
		return elem, false, " (no auto-fix: element type unresolved)"
	}
	switch {
	case b.Info()&types.IsInteger != 0:
		return elem, true, ""
	case b.Info()&types.IsString != 0:
		return elem, false, " (no auto-fix: slices.Max/Min on a string slice fold to an outlined runtime.strmax/strmin call per element, ~10-25% slower on gc than the devirtualized hand-rolled loop; bit-identical but a measured perf regression)"
	case b.Info()&types.IsFloat != 0:
		return elem, false, " (no auto-fix: slices.Max/Min propagate NaN via the builtin max/min while this comparator answers 0 for NaN against anything, so they differ on a NaN element)"
	default:
		return elem, false, " (no auto-fix: element type unresolved)"
	}
}

// ps3022Fix builds the slices.Max(s) / slices.Min(s) rewrite for one call, or
// nil when a guard fails and the report must stay advisory. Only the
// MaxFunc/MinFunc selector name and the text after the slice argument are
// replaced: the slice expression stays untouched in place (text and single
// evaluation preserved), the package qualifier keeps the file's alias, and
// explicit instantiation brackets survive — slices.Max/Min take the same two
// type parameters. The comparator references nothing but its parameters and
// integer literals, so no import edit is ever needed.
func ps3022Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr, base string) *analysis.SuggestedFix {
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

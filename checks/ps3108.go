package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3108 reports a slices.CompactFunc whose equality func is nothing but a
// bare x == y on its two parameters — slices.Compact spelled with a
// per-element indirect call — and rewrites it to slices.Compact. Sibling of
// PS1008 (slices.EqualFunc with a bare == closure) and PS3107 (slices.SortFunc
// with a bare cmp.Compare comparator): the same "generic Func variant fed the
// trivial callback" anti-pattern, on in-place de-duplication.
var PS3108 = register(&lint.Check{
	ID:       "PS3108",
	Category: "indirect",
	Slug:     "compactfunc-eq-closure-to-slices-compact",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.CompactFunc with a bare x == y equality closure is slices.Compact spelled with a per-element indirect call",
		Text: `slices.CompactFunc(s, func(x, y T) bool { return x == y })
replaces consecutive runs of equal elements with a single copy — exactly
what slices.Compact(s) does — but routes every adjacent pair through the
supplied func value, nominally an indirect call per pair where
slices.Compact compares the elements directly.

The result is BIT-IDENTICAL on every input. Both walk the slice once,
keep the first element of each run, compare each later element against the
kept one and drop it on equality, overwrite the retained prefix in place,
and return the slice truncated to that prefix (nil and empty return
unchanged in both). == is side-effect-free, symmetric and
non-overloadable, so a closure spelled return y == x is matched too.
Floats keep their exact semantics — NaN == NaN is false so no run of NaNs
is ever collapsed, and -0.0 == +0.0 is true so an adjacent ±0.0 pair
collapses — on both sides. Interface elements behave identically too: both
compare dynamic types first and panic with the same runtime error only
when an adjacent pair's identical dynamic type is uncomparable. The slice
argument is kept verbatim in place, evaluated once on both sides.

The match is deliberately EXACT, so the rewrite always compiles and never
changes behavior. The equality func must be a fresh func literal whose
whole body is a single return x == y (or return y == x) with the two
parameters — named, non-blank, resolved by object identity — as the bare
operands: a field selector, a captured variable, an extra statement, or
any other operator fails the match silently (the shared ps1008 matcher).
Because x == y type-checks only when the element type E is comparable,
matching the closure already proves slices.Compact's comparable
constraint is satisfiable; a spec-comparable-but-not-strictly type (an
interface, or a struct/array containing one) satisfies the comparable
CONSTRAINT only from go1.20 on, so those sites are fixed only when the
effective language version allows it and stay advisory below it. slices is
resolved with type information — a shadowed local or a same-named method
never matches — and an aliased import keeps its alias. The closure
references only its own two parameters, so deleting it can never orphan an
import.`,
		Before: `s = slices.CompactFunc(s, func(a, b int) bool { return a == b })`,
		After:  `s = slices.Compact(s)`,
		MeasuredWin: `BenchmarkPS3108 (a 4096-element []int with every element
distinct — the worst case, no run collapsed — Apple M2 Pro, gc 1.26,
-count=5): ~4.95 µs/op before vs ~1.26 µs/op after, ~3.9x faster, 0 allocs
either way. Unlike slices.EqualFunc (PS1008, which gc devirtualizes to
parity), slices.CompactFunc's literal closure is NOT devirtualized here,
so every adjacent pair pays a real indirect call that slices.Compact's
direct == avoids — a genuine gc speedup, not merely source-level
robustness. The rewrite also cannot fall off any future devirtualization
path the way a hoisted or grown callback can.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3108",
		Doc:  "slices.CompactFunc with a bare x == y equality closure instead of slices.Compact",
		Run:  runPS3108,
	},
})

func runPS3108(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			// The callee must be the receiver-less package function
			// slices.CompactFunc, resolved through type info.
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "CompactFunc")
			if !ok {
				return true
			}
			// The equality func must BE x == y on the two parameters (in either
			// order — == is symmetric). Reuses PS1008's exact-shape matcher.
			if !ps1008BareEq(pass, call.Args[1]) {
				return true
			}
			elem, fixable, why := ps3108Classify(pass, f, call)
			if elem == "" {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "slices.CompactFunc with a bare x == y equality closure pays an indirect call per adjacent pair; slices.Compact de-duplicates the " + elem + " elements with the identical result and the comparison inlined" + why,
			}
			if fixable {
				if fix := ps3108Fix(f, call, sel); fix != nil {
					diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
				}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps3108Classify inspects CompactFunc's single slice argument and decides the
// report: silent (elem == "") when the operand is not a resolvable slice,
// advisory with a reason suffix when the rewrite is not guaranteed to compile
// (a not-strictly-comparable element below go1.20), and fixable otherwise.
// Because the matched closure's x == y already type-checked, the element type
// is necessarily comparable — the only open question is whether it satisfies
// the comparable CONSTRAINT on the effective language version.
func ps3108Classify(pass *analysis.Pass, f *ast.File, call *ast.CallExpr) (elem string, fixable bool, why string) {
	t := pass.TypesInfo.TypeOf(call.Args[0])
	if t == nil {
		return "", false, ""
	}
	s, ok := t.Underlying().(*types.Slice)
	if !ok {
		// A type-parameter-typed operand (S ~[]E) or an unresolved type.
		return "", false, ""
	}
	e := s.Elem()
	elem = types.TypeString(e, types.RelativeTo(pass.Pkg))
	if tp, isParam := types.Unalias(e).(*types.TypeParam); isParam {
		// x == y on type-parameter operands type-checks only when the
		// constraint's type set is strictly comparable — exactly what
		// satisfying Compact's E comparable needs, on every generics version.
		if iface, isIface := tp.Underlying().(*types.Interface); isIface && iface.IsComparable() {
			return elem, true, ""
		}
		return elem, false, " (no auto-fix: the type parameter's constraint was not proven comparable)"
	}
	if !types.Comparable(e) {
		// Defensive: x == y on the elements should not have type-checked.
		return "", false, ""
	}
	if ps1008StrictlyComparable(e) {
		return elem, true, ""
	}
	// Spec-comparable but not strictly (an interface, or a struct/array
	// containing one): satisfies the comparable constraint only from go1.20 on.
	// Behavior is identical either way; the gate is purely "does it compile".
	if ps1008ComparableIfaceAvailable(pass, f) {
		return elem, true, ""
	}
	return elem, false, " (no auto-fix: an interface element satisfies comparable only from go1.20 on)"
}

// ps3108Fix builds the slices.Compact(s) rewrite: the CompactFunc selector name
// becomes Compact and the text after the slice argument (the comma, the closure
// and the closing paren) becomes ")". The slice expression stays untouched in
// place (text and single evaluation preserved) and the package qualifier keeps
// the file's alias. The closure references only its own two parameters, so no
// import can be orphaned. A comment in the deleted span would be destroyed —
// advisory then.
func ps3108Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	if ps2111CommentIn(f, call.Args[0].End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + ps3107Qualifier(sel) + "Compact(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Compact")},
			{Pos: call.Args[0].End(), End: call.End(), NewText: []byte(")")},
		},
	}
}

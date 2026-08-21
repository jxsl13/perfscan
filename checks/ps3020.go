package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3020 reports a slices.MaxFunc / slices.MinFunc whose comparator is nothing
// but cmp.Compare on the two parameters in SWAPPED order — the OPPOSITE
// extremum (slices.Min / slices.Max) spelled the slow way — and rewrites it to
// the direct form. Sibling of PS3111, which handles the source-order
// cmp.Compare(a, b) comparator (the SAME extremum); the swapped
// cmp.Compare(b, a) reverses the order, so MaxFunc selects the minimum and
// MinFunc the maximum, and PS3111 deliberately never matches it.
var PS3020 = register(&lint.Check{
	ID:       "PS3020",
	Category: "indirect",
	Slug:     "maxfunc-swapped-cmp-compare-to-slices-min",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.MaxFunc/MinFunc with a swapped cmp.Compare(b, a) comparator is the opposite extremum — slices.Min/Max spelled the slow way",
		Text: `slices.MaxFunc(s, func(a, b T) int { return cmp.Compare(b, a) })
walks s once and keeps the element that is maximal under the REVERSED order —
which is exactly the minimum under the natural order, i.e. what slices.Min(s)
does — but pays an indirect call through the comparator func value on EVERY
element, and that closure itself calls cmp.Compare, a second call layer.
slices.Min is a distinct monomorphized entry point that inlines the ordered
comparison (via the builtin min). slices.MinFunc with the swapped comparator is
the symmetric case and becomes slices.Max. PS3111 handles the source-order
cmp.Compare(a, b) comparator (MaxFunc -> Max); this check completes the square
for the reversed spelling, which otherwise stays on the slow path forever.

The fix is offered only when the element type's underlying type is an INTEGER
kind (PS3111's exact policy). cmp.Compare(b, a) == -cmp.Compare(a, b), so
MaxFunc under the reversed order updates its candidate exactly when the new
element is STRICTLY smaller under the natural order and keeps the EARLIER of
any tie — precisely the scan slices.Min performs (the builtin min also keeps
the earlier operand of a tie). For integers any two elements that compare equal
are bitwise-identical values, so tie selection is unobservable either way and
the result is byte-for-byte identical. Both sides panic on an empty slice
(both index s[0] up front). Named element types (type Priority int) are fixed
too. A bare cmp.Compare passed as the comparator value cannot be "swapped" —
it is always the source order and belongs to PS3111 — so only the func-literal
shape is matched here.

STRING elements are bit-identical too but reported ADVISORY only, never
auto-fixed — PS3111's policy verbatim: slices.Min/Max on a []string fold via an
outlined runtime.strmin/strmax call per element, which benchmarks ~10-25%
SLOWER on gc than the devirtualized comparator loop, so the auto-fix would
trade a recognizable idiom for provably slower code and the human decides.

FLOAT elements are reported ADVISORY only, never auto-fixed. slices.Min/Max
are defined via the builtin min/max, which PROPAGATE NaN (any NaN forces a NaN
result) and order -0.0 below +0.0 by value, while the swapped cmp.Compare scan
orders NaN first and treats -0.0 and +0.0 as equal (keeping the earlier): a
MinFunc(cmp.Compare(b, a)) scan never selects a NaN but slices.Max returns one,
so the two can disagree outright. A TYPE-PARAMETER element is likewise advisory
(its instantiations may include floats).

The match is deliberately EXACT (the mirror of the shared PS3107 matcher): the
comparator must be a func literal whose whole body is a single
return cmp.Compare(p1, p0) with the two parameters in REVERSED order, resolved
by object identity — a source-order cmp.Compare(a, b) is PS3111's case and is
never matched here; a selector suffix, a captured variable, or any extra
computation fails the match silently. Both cmp and slices are resolved with
type information; an aliased import matches naturally. The fix replaces the
Func selector with the OPPOSITE base name (MaxFunc -> Min, MinFunc -> Max) and
deletes the comparator; the target slice is kept verbatim, and deleting the
comparator drops the file's cmp import when it removes the last cmp reference
(a cgo file or a comment in the deleted span keeps the report advisory). Fires
only from go1.21 on (slices.Max, slices.Min and cmp.Compare all appeared
there).`,
		Before: `lo := slices.MaxFunc(xs, func(a, b int) int { return cmp.Compare(b, a) })`,
		After:  `lo := slices.Min(xs)`,
		MeasuredWin: `BenchmarkPS3020 (a scattered 4096-element []int scanned per
op, Apple M2 Pro, gc 1.26): ~2.5 µs/op before vs ~2.5 µs/op after — parity, 0
allocs either way. gc inlines slices.MaxFunc and devirtualizes the literal
closure, so both become the same direct-comparison scan; the win is source-level
robustness (the closure, its cmp.Compare scaffolding and the mental
order-reversal go away, and slices.Min cannot fall off the devirtualization path
a hoisted or grown callback can), plus a real per-element indirect call removed
on toolchains without closure devirtualization (gccgo, tinygo, older gc) — the
same class as PS1008/PS3110/PS3111.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3020",
		Doc:  "slices.MaxFunc/MinFunc with a swapped cmp.Compare(b, a) comparator instead of slices.Min/Max",
		Run:  runPS3020,
	},
})

// ps3020Base maps the matched Func variant to the OPPOSITE direct spelling
// (the swapped comparator reverses the order) plus the extremum word for the
// message.
var ps3020Base = map[string]struct{ base, extremum string }{
	"MaxFunc": {"Min", "minimum"},
	"MinFunc": {"Max", "maximum"},
}

func runPS3020(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			continue // slices.Max/Min exist only from go1.21 on (same gate as PS3111).
		}
		type site struct {
			call     *ast.CallExpr
			fn       string
			base     string
			extremum string
			elem     string
			why      string
			fix      *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			names, wanted := ps3020Base[sel.Sel.Name]
			if !wanted {
				return true
			}
			s, ok := ps3107PkgFunc(pass, call.Fun, "slices", sel.Sel.Name)
			if !ok {
				return true
			}
			// The comparator must BE cmp.Compare on the parameters in
			// REVERSED order (the mirror of PS3111's source-order matcher).
			if !ps3020SwappedCompare(pass, call.Args[1]) {
				return true
			}
			// Integer fixable (equal values are bitwise-identical, ties
			// unobservable), string advisory (measured regression), float and
			// type-parameter advisory (NaN/signed-zero divergence).
			elem, fixableElem, why := ps3020Elem(pass, call.Args[1])
			var fix *analysis.SuggestedFix
			if fixableElem {
				fix = ps3020Fix(f, call, s, names.base)
				if fix != nil {
					fixable++
				}
			}
			sites = append(sites, site{call, sel.Sel.Name, names.base, names.extremum, elem, why, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		dropCmp := fixable > 0 && pkgRefCount(pass, f, "cmp") == fixable
		importEdits, importsOK := ps3107ImportEdits(f, dropCmp)
		if !importsOK {
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "slices." + st.fn + " with a swapped cmp.Compare(b, a) comparator selects the " + st.extremum + " through an indirect comparator call per element; slices." + st.base + " computes the identical " + st.elem + " result with the comparison inlined" + st.why,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps3020SwappedCompare reports whether the comparator argument is EXACTLY the
// descending cmp.Compare order: a func literal whose whole body is a single
// `return cmp.Compare(p1, p0)` with the two parameters in REVERSED order,
// resolved by object identity. The source-order (p0, p1) — ascending, PS3111's
// case — a selector suffix, a captured variable, or any extra computation
// fails the match. A bare cmp.Compare func value is always the source order
// and therefore never matches here.
func ps3020SwappedCompare(pass *analysis.Pass, comparator ast.Expr) bool {
	lit, ok := ps2110Unparen(comparator).(*ast.FuncLit)
	if !ok {
		return false
	}
	// Exactly two named parameters, in source order across the fields
	// (func(a, b T) and func(a T, b T) both). A blank _ parameter cannot
	// be referenced in the body, so requiring resolvable names loses
	// nothing. (Same collection as ps3107BareCompare.)
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
	// The whole body is a single `return cmp.Compare(p1, p0)`.
	if len(lit.Body.List) != 1 {
		return false
	}
	ret, ok := lit.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	inner, ok := ps2110Unparen(ret.Results[0]).(*ast.CallExpr)
	if !ok || len(inner.Args) != 2 || inner.Ellipsis.IsValid() {
		return false
	}
	if _, ok := ps3107PkgFunc(pass, inner.Fun, "cmp", "Compare"); !ok {
		return false
	}
	// Reversed order: the first cmp.Compare argument is the SECOND
	// parameter, the second the FIRST.
	for i, want := range []*types.Var{params[1], params[0]} {
		id, isIdent := ps2110Unparen(inner.Args[i]).(*ast.Ident)
		if !isIdent || pass.TypesInfo.Uses[id] != want {
			return false
		}
	}
	return true
}

// ps3020Elem classifies the extremum's element type (the comparator's first
// parameter type): fixable only for an integer kind — equal values are
// bitwise-identical, so tie selection is unobservable, the swapped-order scan
// and the builtin min/max pick the same value, and slices.Min/Max match
// byte-for-byte. String is bit-identical too but stays ADVISORY (slices.Min/
// Max on a string slice fold via an outlined runtime.strmin/strmax call per
// element, a measured ~10-25% regression on gc — PS3111's policy). Floats are
// advisory (the builtin min/max propagate NaN and order signed zeros by value,
// the swapped cmp.Compare scan does neither) and so are type parameters
// (instantiations may include floats).
func ps3020Elem(pass *analysis.Pass, comparator ast.Expr) (elem string, fixable bool, why string) {
	sig, ok := pass.TypesInfo.TypeOf(comparator).(*types.Signature)
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
		return elem, false, " (no auto-fix: slices.Min/Max on a string slice fold to an outlined runtime.strmin/strmax call per element, ~10-25% slower on gc than the devirtualized comparator loop; bit-identical but a measured perf regression)"
	case b.Info()&types.IsFloat != 0:
		return elem, false, " (no auto-fix: slices.Min/Max propagate NaN and order signed zeros via the builtin min/max while the swapped cmp.Compare scan orders NaN first and treats -0.0/+0.0 as a tie, so they differ on a NaN or signed-zero element)"
	default:
		return elem, false, " (no auto-fix: element type unresolved)"
	}
}

// ps3020Fix builds the slices.Min(s) / slices.Max(s) rewrite: the Func
// selector becomes the OPPOSITE base name and the text after the slice
// argument (the comma, the comparator and the closing paren) becomes ")". The
// slice stays verbatim in place and the package qualifier keeps its alias. The
// cmp-import edit is appended later, once per file. A comment in the deleted
// span keeps the report advisory.
func ps3020Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr, base string) *analysis.SuggestedFix {
	if ps2111CommentIn(f, call.Args[0].End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + ps3107Qualifier(sel) + base,
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte(base)},
			{Pos: call.Args[0].End(), End: call.End(), NewText: []byte(")")},
		},
	}
}

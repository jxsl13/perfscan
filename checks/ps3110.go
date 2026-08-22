package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3110 reports a slices.IndexFunc / slices.ContainsFunc whose predicate is
// nothing but a bare x == target on the single parameter — slices.Index /
// slices.Contains spelled with a per-element indirect call — and rewrites it to
// the direct form. Sibling of PS1008 (EqualFunc), PS3107 (SortFunc), PS3108
// (CompactFunc) and PS3109 (BinarySearchFunc): the same "generic Func variant
// fed the trivial callback" anti-pattern, on linear search.
var PS3110 = register(&lint.Check{
	ID:       "PS3110",
	Category: "indirect",
	Slug:     "indexfunc-eq-closure-to-slices-index",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.IndexFunc/ContainsFunc with a bare x == target closure is slices.Index/Contains spelled with a per-element indirect call",
		Text: `slices.IndexFunc(s, func(x T) bool { return x == target }) answers
exactly what slices.Index(s, target) answers, but routes every element through
the supplied func value — nominally an indirect call per element, where
slices.Index compares s[i] == target directly. The same holds for
slices.ContainsFunc(s, func(x T) bool { return x == target }) and
slices.Contains(s, target) (Contains is Index(s, target) >= 0).

The result is BIT-IDENTICAL. Both scan left to right and stop at the first
element that compares equal; == is side-effect-free, symmetric and
non-overloadable, so a closure spelled return target == x is matched too. Floats
keep their exact semantics — NaN != NaN makes both walk past a NaN, -0.0 == +0.0
matches on both sides — and interface elements panic identically only on an
uncomparable identical dynamic type. The searched slice is kept verbatim in place.

The match is deliberately EXACT, so the rewrite always compiles and never changes
behavior:
  - the predicate is a fresh func literal with ONE named, non-blank parameter
    whose whole body is a single return x == target (or target == x), with x
    resolved by object identity;
  - target is the OTHER operand and does NOT reference x;
  - target is CALL-FREE (no function call, conversion or channel receive). The
    closure evaluates target once PER ELEMENT while slices.Index evaluates it
    once, so a target with a side effect or a per-call-varying value would change
    behavior; a variable, field, index, literal or arithmetic over them reads the
    same value every time and is safe.

Because x == target type-checks only when the element type E is comparable,
matching the closure already proves slices.Index's E comparable constraint; a
spec-comparable-but-not-strictly element (an interface, or a struct/array
containing one) satisfies the comparable CONSTRAINT only from go1.20 on, so those
sites are fixed only when the effective language version allows it. slices is
resolved with type information — a shadowed local or a same-named method never
matches — and an aliased import keeps its alias. The predicate references only x
and the (kept) target, so deleting it can never orphan an import.`,
		Before: `i := slices.IndexFunc(users, func(u string) bool { return u == name })`,
		After:  `i := slices.Index(users, name)`,
		MeasuredWin: `Parity on current gc (it inlines slices.IndexFunc and
devirtualizes the literal closure, so both become the identical direct-comparison
scan), 0 allocs either way — the win is source-level robustness (the closure and
its call-site scaffolding go away, and slices.Index cannot fall off the
devirtualization path a hoisted or grown callback can) plus a real per-element
indirect call removed on toolchains without closure devirtualization (gccgo,
tinygo, older gc), the same class as PS1008/PS3108.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3110",
		Doc:  "slices.IndexFunc/ContainsFunc with a bare x == target closure instead of slices.Index/Contains",
		Run:  runPS3110,
	},
})

// ps3110Base maps the matched Func variant to its direct spelling.
var ps3110Base = map[string]string{"IndexFunc": "Index", "ContainsFunc": "Contains"}

func runPS3110(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			fnName := sel.Sel.Name
			base, wanted := ps3110Base[fnName]
			if !wanted {
				return true
			}
			// Pin the callee to the package-level slices.IndexFunc/ContainsFunc.
			s, ok := ps3107PkgFunc(pass, call.Fun, "slices", fnName)
			if !ok {
				return true
			}
			target, ok := ps3110BareEqTarget(pass, call.Args[1])
			if !ok {
				return true
			}
			elem, fixable, why := ps3110Classify(pass, f, call)
			if elem == "" {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "slices." + fnName + " with a bare x == target closure pays an indirect call per element; slices." + base + " compares the " + elem + " elements directly with the identical result" + why,
			}
			if fixable {
				if fix := ps3110Fix(f, call, s, base, target); fix != nil {
					diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
				}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps3110BareEqTarget reports whether pred is a fresh one-parameter func literal
// whose whole body is a single `return x == target` (or `target == x`), where x
// is the parameter (by object identity), target is the OTHER operand, target
// does NOT reference x, and target is CALL-FREE (so its once-per-element
// evaluation in the closure is behavior-equivalent to slices.Index's single
// evaluation). It returns the target expression.
func ps3110BareEqTarget(pass *analysis.Pass, pred ast.Expr) (ast.Expr, bool) {
	lit, ok := ps2110Unparen(pred).(*ast.FuncLit)
	if !ok {
		return nil, false
	}
	// Exactly one named, non-blank parameter.
	var param *types.Var
	count := 0
	for _, field := range lit.Type.Params.List {
		for _, name := range field.Names {
			count++
			if name.Name == "_" {
				return nil, false
			}
			v, isVar := pass.TypesInfo.Defs[name].(*types.Var)
			if !isVar {
				return nil, false
			}
			param = v
		}
	}
	if count != 1 || param == nil {
		return nil, false
	}
	// The whole body is a single `return <ident> == <expr>` / `<expr> == <ident>`.
	if len(lit.Body.List) != 1 {
		return nil, false
	}
	ret, ok := lit.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return nil, false
	}
	bin, ok := ps2110Unparen(ret.Results[0]).(*ast.BinaryExpr)
	if !ok || bin.Op != token.EQL {
		return nil, false
	}
	// Exactly one operand is the parameter x; the other is the target.
	xIsLeft := ps3110IsParam(pass, bin.X, param)
	xIsRight := ps3110IsParam(pass, bin.Y, param)
	if xIsLeft == xIsRight {
		return nil, false // both or neither is x
	}
	target := bin.Y
	if xIsRight {
		target = bin.X
	}
	// The target must not reference x and must be call-free.
	if ps3110RefsParam(pass, target, param) || !ps3110CallFree(target) {
		return nil, false
	}
	return target, true
}

// ps3110IsParam reports whether e is exactly the identifier of param.
func ps3110IsParam(pass *analysis.Pass, e ast.Expr, param *types.Var) bool {
	id, ok := ps2110Unparen(e).(*ast.Ident)
	return ok && pass.TypesInfo.Uses[id] == param
}

// ps3110RefsParam reports whether e mentions the parameter anywhere.
func ps3110RefsParam(pass *analysis.Pass, e ast.Expr, param *types.Var) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && pass.TypesInfo.Uses[id] == param {
			found = true
		}
		return !found
	})
	return found
}

// ps3110CallFree reports whether e contains no function call, conversion or
// channel receive — the constructs whose once-per-element evaluation in the
// closure would diverge from slices.Index's single evaluation.
func ps3110CallFree(e ast.Expr) bool {
	free := true
	ast.Inspect(e, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			free = false
		case *ast.UnaryExpr:
			if x.Op == token.ARROW { // channel receive
				free = false
			}
		}
		return free
	})
	return free
}

// ps3110Classify decides the report: silent (elem == "") when the searched
// operand is not a resolvable slice, advisory with a reason suffix when an
// interface element is below go1.20, fixable otherwise. Matching the == closure
// already proved the element type comparable.
func ps3110Classify(pass *analysis.Pass, f *ast.File, call *ast.CallExpr) (elem string, fixable bool, why string) {
	t := pass.TypesInfo.TypeOf(call.Args[0])
	if t == nil {
		return "", false, ""
	}
	s, ok := t.Underlying().(*types.Slice)
	if !ok {
		return "", false, ""
	}
	e := s.Elem()
	elem = types.TypeString(e, types.RelativeTo(pass.Pkg))
	if tp, isParam := types.Unalias(e).(*types.TypeParam); isParam {
		if iface, isIface := tp.Underlying().(*types.Interface); isIface && iface.IsComparable() {
			return elem, true, ""
		}
		return elem, false, " (no auto-fix: the type parameter's constraint was not proven comparable)"
	}
	if !types.Comparable(e) {
		return "", false, ""
	}
	if ps1008StrictlyComparable(e) {
		return elem, true, ""
	}
	if ps1008ComparableIfaceAvailable(pass, f) {
		return elem, true, ""
	}
	return elem, false, " (no auto-fix: an interface element satisfies comparable only from go1.20 on)"
}

// ps3110Fix rewrites slices.IndexFunc(s, func(x){return x==target}) to
// slices.Index(s, target): the Func selector becomes the base name and the whole
// closure argument becomes the target expression (rendered). The searched slice
// stays verbatim in place and the package qualifier keeps its alias. A comment
// in the closure or the target would be lost — advisory then.
func ps3110Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr, base string, target ast.Expr) *analysis.SuggestedFix {
	if ps2111CommentIn(f, call.Args[1].Pos(), call.Args[1].End()) {
		return nil
	}
	targetText, ok := ps2107ExprText(target)
	if !ok {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + ps3107Qualifier(sel) + base,
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte(base)},
			{Pos: call.Args[1].Pos(), End: call.Args[1].End(), NewText: []byte(targetText)},
		},
	}
}

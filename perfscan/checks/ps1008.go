package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"go/version"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS1008 reports a slices.EqualFunc whose equality func is nothing but a
// bare x == y on its two parameters — slices.Equal spelled with a
// per-element indirect call — and rewrites it to slices.Equal. Sibling of
// PS3107 (slices.SortFunc with a bare cmp.Compare comparator): the same
// "generic Func variant fed the trivial callback" anti-pattern, on the
// equality scan instead of the sort.
var PS1008 = register(&lint.Check{
	ID:       "PS1008",
	Category: "indirect",
	Slug:     "equalfunc-eq-closure-to-slices-equal",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.EqualFunc with a bare x == y equality closure is slices.Equal spelled with a per-element indirect call",
		Text: `slices.EqualFunc(a, b, func(x, y T) bool { return x == y })
answers exactly what slices.Equal(a, b) answers, but routes every element
pair through the supplied func value — nominally an indirect call per
pair, where slices.Equal compares s1[i] != s2[i] directly.

On CURRENT gc the two compile to the same loop at a direct call site:
slices.EqualFunc (inline cost 50) is inlined and the literal closure is
then devirtualized and inlined too (measured parity — see MeasuredWin).
The win is therefore the same class as PS3104/PS2125/PS3102, honest
robustness rather than a gc speedup: the devirtualization holds only
while the closure stays a literal at an inlinable call site (hoist the
func value into a variable, or grow EqualFunc past the inline budget in
a future stdlib, and the per-pair indirect call comes back — slices.Equal
cannot regress that way), toolchains without closure devirtualization
(gccgo, tinygo, older gc) pay the per-pair dispatch today, and the
rewrite deletes a closure worth of code besides.

The result is BIT-IDENTICAL on every input. Both functions first compare
len(a) and len(b) and return false on a mismatch (nil and empty compare
equal to each other in both), then scan left-to-right and short-circuit
on the first differing pair; == and != are side-effect-free, symmetric
and non-overloadable, so a closure spelling return y == x is matched
too. Floats keep their exact semantics — NaN != NaN makes BOTH sides
answer false, and -0.0 == +0.0 makes both answer true — which is why
floats are FIXED here while the ordered-sort family (PS3002/PS3104/
PS3107) must keep them advisory. Interface elements also behave
identically: both sides compare dynamic types first and panic with the
same runtime error only when a pair's identical dynamic type is
uncomparable. The slice arguments are kept verbatim in place, evaluated
once on both sides.

The match is deliberately EXACT, so the rewrite always compiles and
never changes behavior. The equality func must be a fresh func literal
whose whole body is a single return x == y (or return y == x) with the
two parameters — named, non-blank, resolved by object identity — as the
bare operands: a field selector (x.ID == y.ID), a captured variable, an
extra statement, or any other operator fails the match silently. Both
slice arguments must have the SAME type (slices.EqualFunc accepts two
unrelated element types; slices.Equal requires one E comparable) — same
element type but different slice types stays advisory. An explicit
instantiation slices.EqualFunc[S1, S2, E1, E2](...) stays advisory too:
its four type arguments do not transfer to Equal's two. Elements whose
type is not strictly comparable (interfaces, or structs containing them)
satisfy the comparable constraint only from go1.20 on, so those sites
are auto-fixed only when the effective language version allows it;
strictly comparable elements (integers, strings, floats, pointers,
channels, comparable structs/arrays, and comparable-constrained type
parameters) are fixed unconditionally. slices is resolved with type
information — a shadowed local or a same-named method never matches —
and an aliased import keeps its alias.`,
		Before: `slices.EqualFunc(a, b, func(x, y int) bool { return x == y })`,
		After:  `slices.Equal(a, b)`,
		MeasuredWin: `BenchmarkPS1008 (two equal 4096-element []int scanned per op —
the worst case, no early exit — Apple M2 Pro, gc 1.26): parity — ~1.4
µs/op before and after, 0 allocs either way. Expected: gc inlines
slices.EqualFunc (cost 50) and devirtualizes the literal closure, so
both sides become the identical direct-comparison loop. The win on
current gc is source-level (the closure and its call-site scaffolding go
away, and slices.Equal cannot fall off the devirtualization path the way
a hoisted or grown callback can); on toolchains without closure
devirtualization the rewrite removes a real indirect call per element
pair.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS1008",
		Doc:  "slices.EqualFunc with a bare x == y equality closure instead of slices.Equal",
		Run:  runPS1008,
	},
})

func runPS1008(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 || call.Ellipsis.IsValid() {
				return true
			}
			// The callee must be the receiver-less package function
			// slices.EqualFunc, resolved through type info — never a
			// shadowed slices or a same-named method.
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "EqualFunc")
			if !ok {
				return true
			}
			// The equality func must BE x == y on the two parameters (in
			// either order — == is symmetric) — otherwise the call is not
			// a slices.Equal and is never reported.
			if !ps1008BareEq(pass, call.Args[2]) {
				return true
			}
			elem, fixable, why := ps1008Classify(pass, f, call)
			if elem == "" {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "slices.EqualFunc with a bare x == y equality closure pays an indirect call per element pair; slices.Equal compares the " + elem + " elements with the identical result and the comparison inlined" + why,
			}
			if fixable {
				if fix := ps1008Fix(f, call, sel); fix != nil {
					diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
				}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps1008BareEq reports whether the equality argument is a fresh func
// literal whose whole body is a single `return x == y` (or `return y == x`
// — == is symmetric) with x and y the literal's two parameters, resolved by
// object identity. Anything else — a named func value, a blank parameter, a
// selector operand, a captured variable, an extra statement, another
// operator — fails the match. Under this shape the closure references
// nothing but its two parameters, so deleting it can never orphan an
// import or drop a side effect.
func ps1008BareEq(pass *analysis.Pass, eq ast.Expr) bool {
	lit, ok := ps2110Unparen(eq).(*ast.FuncLit)
	if !ok {
		return false
	}
	// Exactly two named parameters (func(x, y T) and func(x T, y T) both).
	// A blank _ parameter cannot be an operand, so requiring resolvable
	// names loses nothing.
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
	// The whole body is a single `return <ident> == <ident>`.
	if len(lit.Body.List) != 1 {
		return false
	}
	ret, ok := lit.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	bin, ok := ps2110Unparen(ret.Results[0]).(*ast.BinaryExpr)
	if !ok || bin.Op != token.EQL {
		return false
	}
	x, ok := ps2110Unparen(bin.X).(*ast.Ident)
	if !ok {
		return false
	}
	y, ok := ps2110Unparen(bin.Y).(*ast.Ident)
	if !ok {
		return false
	}
	ox, oy := pass.TypesInfo.Uses[x], pass.TypesInfo.Uses[y]
	return (ox == params[0] && oy == params[1]) || (ox == params[1] && oy == params[0])
}

// ps1008Classify inspects the two slice arguments and decides the report:
// silent (elem == "") when slices.Equal is not even a candidate replacement
// (unresolved types, non-slice operands, different element types), advisory
// with a reason suffix when the advice holds but the mechanical rewrite is
// not guaranteed to compile (different slice types, explicit instantiation,
// a not-strictly-comparable element below go1.20), and fixable otherwise.
func ps1008Classify(pass *analysis.Pass, f *ast.File, call *ast.CallExpr) (elem string, fixable bool, why string) {
	t0 := pass.TypesInfo.TypeOf(call.Args[0])
	t1 := pass.TypesInfo.TypeOf(call.Args[1])
	if t0 == nil || t1 == nil {
		return "", false, ""
	}
	s0, ok0 := t0.Underlying().(*types.Slice)
	s1, ok1 := t1.Underlying().(*types.Slice)
	if !ok0 || !ok1 {
		// A type-parameter-typed operand (S ~[]E) or an unresolved type:
		// stay silent.
		return "", false, ""
	}
	e := s0.Elem()
	elem = types.TypeString(e, types.RelativeTo(pass.Pkg))
	if !types.Identical(t0, t1) {
		if types.Identical(e, s1.Elem()) {
			// Same element type, different slice types (named vs unnamed):
			// slices.Equal wants ONE S — advisory only.
			return elem, false, " (no auto-fix: the slice arguments have different types)"
		}
		// Two unrelated element types (EqualFunc allows E1 != E2):
		// slices.Equal cannot express the comparison at all.
		return "", false, ""
	}
	switch ps2110Unparen(call.Fun).(type) {
	case *ast.IndexExpr, *ast.IndexListExpr:
		return elem, false, " (no auto-fix: EqualFunc's explicit type arguments do not transfer to Equal's two type parameters)"
	}
	if tp, isParam := types.Unalias(e).(*types.TypeParam); isParam {
		// x == y on type-parameter operands type-checks only when the
		// constraint's type set is strictly comparable — exactly what
		// satisfying Equal's E comparable needs, on every language
		// version with generics. Verify it anyway.
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
		// Strictly comparable types satisfy comparable on every language
		// version with generics.
		return elem, true, ""
	}
	// Spec-comparable but not strictly (an interface, or a struct/array
	// containing one): such a type satisfies the comparable constraint
	// only from go1.20 on. Behavior is identical either way — both sides
	// panic with the same runtime error on an uncomparable identical
	// dynamic type pair — the gate is purely "does the rewrite compile".
	if ps1008ComparableIfaceAvailable(pass, f) {
		return elem, true, ""
	}
	return elem, false, " (no auto-fix: an interface element satisfies comparable only from go1.20 on)"
}

// ps1008StrictlyComparable reports whether t is strictly comparable — spec-
// comparable with no interface anywhere, so == can never panic and the type
// satisfies the comparable constraint on every language version with
// generics. Conservative: false only ever demotes a fix to the go1.20-gated
// path or to advisory.
func ps1008StrictlyComparable(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Info()&(types.IsBoolean|types.IsNumeric|types.IsString) != 0 ||
			u.Kind() == types.UnsafePointer
	case *types.Pointer, *types.Chan:
		return true
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if !ps1008StrictlyComparable(u.Field(i).Type()) {
				return false
			}
		}
		return true
	case *types.Array:
		return ps1008StrictlyComparable(u.Elem())
	default:
		// Interfaces (comparable but may panic), type parameters inside
		// composites, and everything uncomparable.
		return false
	}
}

// ps1008ComparableIfaceAvailable reports whether f's effective language
// version lets a spec-comparable-but-not-strictly-comparable type (an
// interface) satisfy the comparable constraint (go1.20). The per-file
// version wins over the package's; an unknown or unparseable version does
// not block the fix — perfscan itself requires a far newer toolchain, so an
// empty version means "module default", not "ancient" (same policy as
// ps3104SlicesSortAvailable).
func ps1008ComparableIfaceAvailable(pass *analysis.Pass, f *ast.File) bool {
	v := ""
	if pass.TypesInfo.FileVersions != nil {
		v = pass.TypesInfo.FileVersions[f]
	}
	if v == "" && pass.Pkg != nil {
		v = pass.Pkg.GoVersion()
	}
	if v == "" {
		return true
	}
	lang := version.Lang(v)
	if lang == "" {
		return true
	}
	return version.Compare(lang, "go1.20") >= 0
}

// ps1008Fix builds the slices.Equal(a, b) rewrite for one call, or nil when
// a guard fails and the report must stay advisory. Only the EqualFunc
// selector name and the text after the second slice argument are replaced:
// both slice expressions stay untouched in place (text and single
// evaluation preserved) and the package qualifier keeps the file's alias.
// The deleted span holds nothing but the comma, the closure and the closing
// parenthesis — the closure references only its own two parameters, so no
// import can be orphaned and no import edit is ever needed.
func ps1008Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	// A comment inside the deleted span would be silently destroyed —
	// advisory then. (The other replaced span is the EqualFunc identifier
	// itself, which cannot contain a comment.)
	if ps2111CommentIn(f, call.Args[1].End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + ps3107Qualifier(sel) + "Equal(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Equal")},
			{Pos: call.Args[1].End(), End: call.End(), NewText: []byte(")")},
		},
	}
}

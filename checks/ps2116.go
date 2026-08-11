package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"go/version"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2116 reports a loop that zeroes a slice element-by-element where
// clear(s) (Go 1.21) says the same thing in one call.
var PS2116 = register(&lint.Check{
	ID:       "PS2116",
	Category: "alloc",
	Slug:     "slice-zeroing-loop",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a slice zeroed element-by-element instead of with clear",
		Text: `for i := range s { s[i] = 0 } spells "set every element to its
zero value" as an index loop. Go 1.21's clear(s) states that intent
directly, works for every element type, and cannot drift out of the
compiler's memclr-recognized shape when the loop is later edited.

Both the range form and the canonical counted form
(for i := 0; i < len(s); i++) are matched, but ONLY when the body is
exactly the single statement s[i] = Z with Z provably the zero value of
the element type: the literals 0, "", false, nil, or an empty composite
literal T{} whose type is a struct or array. Guards keep the rewrite
bit-identical:

  - s must be a plain identifier of slice type. Arrays and
    pointers-to-array are not clearable, and a map value-zeroing loop
    (for k := range m { m[k] = 0 }) is NOT clear(m) — clear on a map
    deletes the keys instead of zeroing the values.
  - a constant 0/""/false only counts when the element type is a basic
    type: assigned to an interface element it boxes a non-nil value,
    which is not the interface's zero value. Interface, pointer, map,
    chan and func elements match only the literal nil.
  - T{} counts only for struct/array element types: []E{} or map[K]V{}
    would store a non-nil empty value, not the nil zero value.
  - the index must be declared BY the loop (for i := range s, or
    i := 0 in the for clause): with plain = the variable's post-loop
    value is observable and the loop cannot be deleted.
  - only the literal spellings above are matched; a named constant that
    happens to be zero keeps its own meaning and is left alone.
  - non-zero fills are out of scope entirely: a fill loop is the
    idiomatic remedy and there is nothing better to suggest.

The automatic fix replaces the whole loop with clear(s). It is
suppressed — leaving the plain report — when the identifier clear is
shadowed at the loop's position, or when a comment sits inside the
loop's interior lines (the rewrite would silently delete it).`,
		Before: `for i := range buf {
	buf[i] = 0
}`,
		After: `clear(buf)`,
		MeasuredWin: `BenchmarkPS2116 (zeroing a 4096-byte []byte, Apple
M2 Pro): ~131 ns/op -> ~132 ns/op — PARITY, as expected: gc already
lowers this exact loop to a runtime memclr call. The win is directness
and robustness, not cycles: any later edit to the loop body (a second
statement, a changed index) silently loses the memclr lowering and
falls back to an element-at-a-time loop, while clear(s) cannot regress.
Zero allocations either way.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2116",
		Doc:  "slice zeroed element-by-element instead of clear(s)",
		Run:  runPS2116,
	},
})

func runPS2116(pass *analysis.Pass) (any, error) {
	if !ps2116VersionOK(pass) {
		return nil, nil
	}
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			var loop ast.Stmt
			var s *ast.Ident
			switch l := n.(type) {
			case *ast.RangeStmt:
				loop, s = l, ps2116MatchRange(pass, l)
			case *ast.ForStmt:
				loop, s = l, ps2116MatchCounted(pass, l)
			default:
				return true
			}
			if s == nil {
				return true
			}
			repl := "clear(" + s.Name + ")"
			diag := analysis.Diagnostic{
				Pos:     loop.Pos(),
				End:     loop.End(),
				Message: "the loop writes the zero value to every element of " + s.Name + "; " + repl + " states that directly (Go 1.21)",
			}
			// The rewrite deletes the loop's whole source range, so it
			// must not swallow an interior comment, and the injected
			// identifier must still mean the builtin at this position.
			if ps2116ClearUsable(pass, loop.Pos()) && !ps2116InteriorComment(pass, f, loop) {
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace the loop with " + repl,
					TextEdits: []analysis.TextEdit{
						{Pos: loop.Pos(), End: loop.End(), NewText: []byte(repl)},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2116MatchRange matches `for i := range s { s[i] = Z }` where s is an
// identifier of slice type, i is declared by the loop, and Z is the zero
// value of the element type. It returns the slice identifier, or nil.
func ps2116MatchRange(pass *analysis.Pass, rng *ast.RangeStmt) *ast.Ident {
	// Tok must be := — with plain = the index variable's post-loop value
	// (last index reached) is observable and deleting the loop changes it.
	if rng.Tok != token.DEFINE || rng.Value != nil {
		return nil
	}
	key, ok := rng.Key.(*ast.Ident)
	if !ok || key.Name == "_" {
		return nil
	}
	s, ok := rng.X.(*ast.Ident)
	if !ok {
		return nil
	}
	sObj, elem := ps2116SliceVar(pass, s)
	if sObj == nil {
		return nil
	}
	iObj := pass.TypesInfo.Defs[key]
	if iObj == nil || !ps2116BodyZeroes(pass, rng.Body, sObj, iObj, elem) {
		return nil
	}
	return s
}

// ps2116MatchCounted matches the canonical counted form
// `for i := 0; i < len(s); i++ { s[i] = Z }` with the same guards as the
// range form. Only this exact shape is recognized: any other bound,
// start, step, or comparison direction is left alone.
func ps2116MatchCounted(pass *analysis.Pass, fs *ast.ForStmt) *ast.Ident {
	init, ok := fs.Init.(*ast.AssignStmt)
	if !ok || init.Tok != token.DEFINE || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return nil
	}
	iIdent, ok := init.Lhs[0].(*ast.Ident)
	if !ok || iIdent.Name == "_" || !ps2116IsZeroIntLit(pass, init.Rhs[0]) {
		return nil
	}
	iObj := pass.TypesInfo.Defs[iIdent]
	if iObj == nil {
		return nil
	}
	cond, ok := fs.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.LSS {
		return nil
	}
	condI, ok := cond.X.(*ast.Ident)
	if !ok || pass.TypesInfo.ObjectOf(condI) != iObj {
		return nil
	}
	lenCall, ok := cond.Y.(*ast.CallExpr)
	if !ok || len(lenCall.Args) != 1 || lenCall.Ellipsis.IsValid() {
		return nil
	}
	lenIdent, ok := lenCall.Fun.(*ast.Ident)
	if !ok {
		return nil
	}
	// Type info pins len to the builtin: a shadowing local resolves to a
	// different object and is rejected.
	if b, isBuiltin := pass.TypesInfo.ObjectOf(lenIdent).(*types.Builtin); !isBuiltin || b.Name() != "len" {
		return nil
	}
	s, ok := lenCall.Args[0].(*ast.Ident)
	if !ok {
		return nil
	}
	sObj, elem := ps2116SliceVar(pass, s)
	if sObj == nil {
		return nil
	}
	post, ok := fs.Post.(*ast.IncDecStmt)
	if !ok || post.Tok != token.INC {
		return nil
	}
	postI, ok := post.X.(*ast.Ident)
	if !ok || pass.TypesInfo.ObjectOf(postI) != iObj {
		return nil
	}
	if !ps2116BodyZeroes(pass, fs.Body, sObj, iObj, elem) {
		return nil
	}
	return s
}

// ps2116SliceVar returns s's variable object and element type when s
// names a variable whose underlying type is a slice. Arrays,
// pointers-to-array, maps, strings, and integer range bounds all fail
// here: clear either rejects them or (for maps) means something else.
func ps2116SliceVar(pass *analysis.Pass, s *ast.Ident) (types.Object, types.Type) {
	v, ok := pass.TypesInfo.ObjectOf(s).(*types.Var)
	if !ok {
		return nil, nil
	}
	sl, ok := v.Type().Underlying().(*types.Slice)
	if !ok {
		return nil, nil
	}
	return v, sl.Elem()
}

// ps2116BodyZeroes reports whether body is exactly the single statement
// s[i] = Z with s and i resolving to sObj and iObj and Z the zero value
// of elem. Anything else in the body — a second statement, a different
// target, a multi-assign — rejects the loop.
func ps2116BodyZeroes(pass *analysis.Pass, body *ast.BlockStmt, sObj, iObj types.Object, elem types.Type) bool {
	if body == nil || len(body.List) != 1 {
		return false
	}
	as, ok := body.List[0].(*ast.AssignStmt)
	if !ok || as.Tok != token.ASSIGN || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return false
	}
	ix, ok := as.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return false
	}
	base, ok := ix.X.(*ast.Ident)
	if !ok || pass.TypesInfo.ObjectOf(base) != sObj {
		return false
	}
	idx, ok := ix.Index.(*ast.Ident)
	if !ok || pass.TypesInfo.ObjectOf(idx) != iObj {
		return false
	}
	return ps2116IsZeroValue(pass, as.Rhs[0], elem)
}

// ps2116IsZeroValue reports whether e is one of the recognized zero-value
// spellings for element type elem: the literal constants 0/""/false (only
// for basic element types — assigned to an interface element a constant
// boxes a NON-nil value), the predeclared nil (the zero value wherever it
// is assignable), or an empty composite literal of a struct/array type
// identical to elem ([]E{} or map[K]V{} would be non-nil, so slice and
// map elements match only nil). Named constants are deliberately not
// matched even when zero: the value is checked semantically, but the
// spelling must be the anonymous literal.
func ps2116IsZeroValue(pass *analysis.Pass, e ast.Expr, elem types.Type) bool {
	tv, ok := pass.TypesInfo.Types[e]
	if !ok {
		return false
	}
	switch e := e.(type) {
	case *ast.BasicLit:
		if _, basic := elem.Underlying().(*types.Basic); !basic {
			return false
		}
		return tv.Value != nil && ps2116ConstZero(tv.Value)
	case *ast.Ident:
		if tv.IsNil() {
			return true
		}
		// `false`. The VALUE decides, so a shadowed false that is not
		// the constant false cannot slip through.
		if _, basic := elem.Underlying().(*types.Basic); !basic {
			return false
		}
		return tv.Value != nil && tv.Value.Kind() == constant.Bool && !constant.BoolVal(tv.Value)
	case *ast.CompositeLit:
		if len(e.Elts) != 0 || tv.Type == nil || !types.Identical(tv.Type, elem) {
			return false
		}
		switch elem.Underlying().(type) {
		case *types.Struct, *types.Array:
			return true
		}
	}
	return false
}

// ps2116ConstZero reports whether v is the zero of its constant kind.
func ps2116ConstZero(v constant.Value) bool {
	switch v.Kind() {
	case constant.Int, constant.Float:
		return constant.Sign(v) == 0
	case constant.Complex:
		return constant.Sign(constant.Real(v)) == 0 && constant.Sign(constant.Imag(v)) == 0
	case constant.String:
		return constant.StringVal(v) == ""
	case constant.Bool:
		return !constant.BoolVal(v)
	}
	return false
}

// ps2116IsZeroIntLit reports whether e is the integer literal 0 (the
// counted loop's required start).
func ps2116IsZeroIntLit(pass *analysis.Pass, e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return false
	}
	tv, ok := pass.TypesInfo.Types[lit]
	return ok && tv.Value != nil && tv.Value.Kind() == constant.Int && constant.Sign(tv.Value) == 0
}

// ps2116ClearUsable reports whether the identifier clear denotes the
// builtin at pos: a local or package-level object of that name would
// capture the injected call, so the fix is suppressed.
func ps2116ClearUsable(pass *analysis.Pass, pos token.Pos) bool {
	scope := pass.Pkg.Scope().Innermost(pos)
	if scope == nil {
		return false
	}
	_, obj := scope.LookupParent("clear", pos)
	_, isBuiltin := obj.(*types.Builtin)
	return isBuiltin
}

// ps2116InteriorComment reports whether a comment overlaps the loop's
// source range on any line after the for clause's line: the rewrite
// deletes the whole range, and an interior comment would vanish with it.
// A trailing comment on the for line itself is tolerated (this is also
// where analysistest expectations live).
func ps2116InteriorComment(pass *analysis.Pass, f *ast.File, loop ast.Stmt) bool {
	forLine := pass.Fset.Position(loop.Pos()).Line
	for _, cg := range f.Comments {
		if cg.End() <= loop.Pos() || cg.Pos() >= loop.End() {
			continue
		}
		if pass.Fset.Position(cg.Pos()).Line != forLine {
			return true
		}
	}
	return false
}

// ps2116VersionOK reports whether the package's language version has the
// clear builtin (Go 1.21). An unknown version is treated as current.
func ps2116VersionOK(pass *analysis.Pass) bool {
	v := pass.Pkg.GoVersion()
	return v == "" || version.Compare(version.Lang(v), "go1.21") >= 0
}

package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Pointer-to-pointer out-arguments generate a cgo base alias and a constant
// true pointer-check argument, unlike the direct-handle PS6133 form. Prove the
// complete compiler wrapper and aliases; never waive arbitrary IIFE effects.
func ps6137GeneratedArguments(pass *analysis.Pass, outer *ast.CallExpr) (*ast.CallExpr, bool) {
	literal, ok := ps2110Unparen(outer.Fun).(*ast.FuncLit)
	if !ok || len(outer.Args) != 0 || len(literal.Body.List) < 2 {
		return nil, false
	}
	native := ps6135Return(literal.Body.List[len(literal.Body.List)-1])
	if native == nil {
		return nil, false
	}
	id, ok := ps2110Unparen(native.Fun).(*ast.Ident)
	if !ok || !ps6110SyntheticCgoObject(pass, id) {
		return nil, false
	}
	if _, ok := ps2004GeneratedCgoName(id.Name); !ok {
		return nil, false
	}
	aliases := make(map[types.Object]ast.Expr, len(native.Args)+1)
	used := make(map[types.Object]bool, len(native.Args)+1)
	checked := make(map[types.Object]bool, len(native.Args))
	checking := false
	resolve := func(e ast.Expr) (ast.Expr, types.Object, bool) { return ps6137ResolveAlias(pass, e, aliases, used) }
	for _, stmt := range literal.Body.List[:len(literal.Body.List)-1] {
		var name *ast.Ident
		var value ast.Expr
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			if checking || s.Tok != token.DEFINE || len(s.Lhs) != 1 || len(s.Rhs) != 1 {
				return nil, false
			}
			name, ok = s.Lhs[0].(*ast.Ident)
			if !ok {
				return nil, false
			}
			value = s.Rhs[0]
		case *ast.DeclStmt:
			gen, ok := s.Decl.(*ast.GenDecl)
			if checking || !ok || gen.Tok != token.VAR || len(gen.Specs) != 1 {
				return nil, false
			}
			spec, ok := gen.Specs[0].(*ast.ValueSpec)
			if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
				return nil, false
			}
			name = spec.Names[0]
			value = spec.Values[0]
		case *ast.ExprStmt:
			checking = true
			call, ok := ps2110Unparen(s.X).(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return nil, false
			}
			id, ok := ps2110Unparen(call.Fun).(*ast.Ident)
			if !ok || !ps6133RuntimePointerCheck(pass, id) {
				return nil, false
			}
			arg, ok := ps2110Unparen(call.Args[0]).(*ast.Ident)
			if !ok || aliases[identObject(pass, arg)] == nil {
				return nil, false
			}
			root, object, ok := resolve(arg)
			if !ok || checked[object] {
				return nil, false
			}
			checked[object] = true
			if !ps6137Nil(pass, call.Args[1]) {
				v := pass.TypesInfo.Types[ps2110Unparen(call.Args[1])].Value
				u, ok := ps2110Unparen(root).(*ast.UnaryExpr)
				if v == nil || v.Kind() != constant.Bool || !constant.BoolVal(v) || !ok || u.Op != token.AND {
					return nil, false
				}
			}
			continue
		default:
			return nil, false
		}
		object := pass.TypesInfo.Defs[name]
		if object == nil || aliases[object] != nil {
			return nil, false
		}
		aliases[object] = value
	}
	args := make([]ast.Expr, len(native.Args))
	seen := make(map[types.Object]bool, len(native.Args))
	for i, arg := range native.Args {
		id, ok := ps2110Unparen(arg).(*ast.Ident)
		if !ok || aliases[identObject(pass, id)] == nil {
			return nil, false
		}
		value, root, ok := resolve(arg)
		if !ok || seen[root] {
			return nil, false
		}
		seen[root] = true
		args[i] = value
	}
	if len(used) != len(aliases) {
		return nil, false
	}
	copy := *native
	copy.Args = args
	return &copy, true
}

func ps6137ResolveAlias(pass *analysis.Pass, e ast.Expr, aliases map[types.Object]ast.Expr, used map[types.Object]bool) (ast.Expr, types.Object, bool) {
	var root types.Object
	seen := make(map[types.Object]bool, len(aliases))
	for {
		id, ok := ps2110Unparen(e).(*ast.Ident)
		if !ok {
			return e, root, root != nil
		}
		object := identObject(pass, id)
		value := aliases[object]
		if value == nil {
			return e, root, root != nil
		}
		if seen[object] {
			return nil, nil, false
		}
		seen[object] = true
		used[object] = true
		root = object
		e = value
	}
}

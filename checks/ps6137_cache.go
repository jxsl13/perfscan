package checks

import (
	"go/ast"
	goToken "go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

type ps6137CacheRoles struct{ count, inline, rest, text, token, owned string }

func ps6137CacheRolesFor(t types.Type) (ps6137CacheRoles, bool) {
	var roles ps6137CacheRoles
	p, ok := types.Unalias(t).(*types.Pointer)
	if !ok {
		return roles, false
	}
	record, ok := types.Unalias(p.Elem()).Underlying().(*types.Struct)
	if !ok || record.NumFields() != 4 {
		return roles, false
	}
	for i := 0; i < record.NumFields(); i++ {
		field := record.Field(i)
		switch t := types.Unalias(field.Type()).Underlying().(type) {
		case *types.Basic:
			if roles.count != "" || !types.Identical(field.Type(), types.Typ[types.Int]) {
				return roles, false
			}
			roles.count = field.Name()
		case *types.Array:
			if roles.inline != "" || t.Len() <= 0 {
				return roles, false
			}
			roles.inline = field.Name()
			entry, ok := types.Unalias(t.Elem()).Underlying().(*types.Struct)
			if !ok || entry.NumFields() != 2 {
				return roles, false
			}
			for j := 0; j < entry.NumFields(); j++ {
				f := entry.Field(j)
				if types.Identical(f.Type(), types.Typ[types.String]) {
					roles.owned = f.Name()
				} else if types.Identical(f.Type(), types.Typ[types.Uintptr]) {
					roles.token = f.Name()
				} else {
					return roles, false
				}
			}
		case *types.Map:
			if !types.Identical(t.Elem(), types.Typ[types.String]) {
				return roles, false
			}
			if types.Identical(t.Key(), types.Typ[types.Uintptr]) {
				if roles.rest != "" {
					return roles, false
				}
				roles.rest = field.Name()
			} else if types.Identical(t.Key(), types.Typ[types.String]) {
				if roles.text != "" {
					return roles, false
				}
				roles.text = field.Name()
			} else {
				return roles, false
			}
		default:
			return roles, false
		}
	}
	return roles, roles.count != "" && roles.inline != "" && roles.rest != "" && roles.text != "" && roles.token != "" && roles.owned != ""
}

func ps6137CacheField(pass *analysis.Pass, e ast.Expr, receiver types.Object, name string) bool {
	s, ok := ps2110Unparen(e).(*ast.SelectorExpr)
	return ok && s.Sel.Name == name && ps6135Object(pass, s.X, receiver)
}

func ps6137CacheSlot(pass *analysis.Pass, e ast.Expr, receiver, index types.Object, roles ps6137CacheRoles, name string) bool {
	s, ok := ps2110Unparen(e).(*ast.SelectorExpr)
	if !ok || s.Sel.Name != name {
		return false
	}
	slot, ok := ps2110Unparen(s.X).(*ast.IndexExpr)
	return ok && ps6137CacheField(pass, slot.X, receiver, roles.inline) && ps6135Object(pass, slot.Index, index)
}

func ps6137OneReturn(pass *analysis.Pass, stmt ast.Stmt, value func(ast.Expr) bool) bool {
	r, ok := stmt.(*ast.ReturnStmt)
	return ok && len(r.Results) == 1 && value(r.Results[0])
}

func ps6137CacheLookup(pass *analysis.Pass, stmt ast.Stmt, receiver, key types.Object, field string, rest func(ast.Stmt, types.Object) bool) (types.Object, bool) {
	branch, ok := stmt.(*ast.IfStmt)
	if !ok || branch.Else != nil {
		return nil, false
	}
	init, ok := branch.Init.(*ast.AssignStmt)
	if !ok || init.Tok != goToken.DEFINE || len(init.Lhs) != 2 || len(init.Rhs) != 1 {
		return nil, false
	}
	value := ps6137Local(pass, init.Lhs[0])
	present := ps6137Local(pass, init.Lhs[1])
	if value == nil || present == nil || !ps6135Object(pass, branch.Cond, present) {
		return nil, false
	}
	lookup, ok := ps2110Unparen(init.Rhs[0]).(*ast.IndexExpr)
	if !ok || !ps6137CacheField(pass, lookup.X, receiver, field) || !ps6135Object(pass, lookup.Index, key) {
		return nil, false
	}
	want := 1
	if rest != nil {
		want = 2
	}
	if len(branch.Body.List) != want {
		return nil, false
	}
	if rest != nil && !rest(branch.Body.List[0], value) {
		return nil, false
	}
	return value, ps6137OneReturn(pass, branch.Body.List[want-1], func(e ast.Expr) bool { return ps6135Object(pass, e, value) })
}

func ps6137CacheMapWrite(pass *analysis.Pass, stmt ast.Stmt, receiver, key, value types.Object, field string) bool {
	a, ok := stmt.(*ast.AssignStmt)
	if !ok || a.Tok != goToken.ASSIGN || len(a.Lhs) != 1 || len(a.Rhs) != 1 || !ps6135Object(pass, a.Rhs[0], value) {
		return false
	}
	slot, ok := ps2110Unparen(a.Lhs[0]).(*ast.IndexExpr)
	return ok && ps6137CacheField(pass, slot.X, receiver, field) && ps6135Object(pass, slot.Index, key)
}

func ps6137CacheCapacity(pass *analysis.Pass, stmt ast.Stmt, receiver, token types.Object, roles ps6137CacheRoles, value func(ast.Expr) bool) bool {
	branch, ok := stmt.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 2 {
		return false
	}
	less, ok := ps2110Unparen(branch.Cond).(*ast.BinaryExpr)
	if !ok || less.Op != goToken.LSS || !ps6137CacheField(pass, less.X, receiver, roles.count) {
		return false
	}
	length, ok := ps2110Unparen(less.Y).(*ast.CallExpr)
	if !ok || len(length.Args) != 1 || !ps6137CacheField(pass, length.Args[0], receiver, roles.inline) {
		return false
	}
	id, ok := ps2110Unparen(length.Fun).(*ast.Ident)
	if !ok || identObject(pass, id) != types.Universe.Lookup("len") {
		return false
	}
	write, ok := branch.Body.List[0].(*ast.AssignStmt)
	if !ok || write.Tok != goToken.ASSIGN || len(write.Lhs) != 1 || len(write.Rhs) != 1 {
		return false
	}
	slot, ok := ps2110Unparen(write.Lhs[0]).(*ast.IndexExpr)
	if !ok || !ps6137CacheField(pass, slot.X, receiver, roles.inline) || !ps6137CacheField(pass, slot.Index, receiver, roles.count) {
		return false
	}
	literal, ok := ps2110Unparen(write.Rhs[0]).(*ast.CompositeLit)
	if !ok {
		return false
	}
	fields, ok := ps6137Fields(literal)
	if !ok || len(fields) != 2 || !ps6135Object(pass, fields[roles.token], token) || !value(fields[roles.owned]) {
		return false
	}
	increment, ok := branch.Body.List[1].(*ast.IncDecStmt)
	return ok && increment.Tok == goToken.INC && ps6137CacheField(pass, increment.X, receiver, roles.count)
}

func ps6137CacheLoop(pass *analysis.Pass, stmt ast.Stmt, receiver, token, view types.Object, roles ps6137CacheRoles, content bool) bool {
	loop, ok := stmt.(*ast.RangeStmt)
	if !ok || loop.Tok != goToken.DEFINE || loop.Value != nil || len(loop.Body.List) != 1 || !ps6137CacheField(pass, loop.X, receiver, roles.count) {
		return false
	}
	index := ps6137Local(pass, loop.Key)
	if index == nil {
		return false
	}
	branch, ok := loop.Body.List[0].(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil {
		return false
	}
	eq, ok := ps2110Unparen(branch.Cond).(*ast.BinaryExpr)
	if !ok || eq.Op != goToken.EQL {
		return false
	}
	field := roles.token
	source := token
	want := 1
	if content {
		field = roles.owned
		source = view
		want = 2
	}
	if len(branch.Body.List) != want || !ps6135Object(pass, eq.X, source) || !ps6137CacheSlot(pass, eq.Y, receiver, index, roles, field) {
		return false
	}
	value := func(e ast.Expr) bool { return ps6137CacheSlot(pass, e, receiver, index, roles, roles.owned) }
	if content && !ps6137CacheCapacity(pass, branch.Body.List[0], receiver, token, roles, value) {
		return false
	}
	return ps6137OneReturn(pass, branch.Body.List[want-1], value)
}

func ps6137CacheSemantics(pass *analysis.Pass, f ps6137Function) bool {
	roles, ok := ps6137CacheRolesFor(f.sig.Recv().Type())
	if !ok {
		return false
	}
	list := f.decl.Body.List
	if len(list) != 14 {
		return false
	}
	receiver := f.sig.Recv()
	token := f.sig.Params().At(0)
	viewBind, ok := ps6137Assignment(list[5])
	if !ok {
		return false
	}
	view := ps6137Local(pass, viewBind.Lhs[0])
	if view == nil {
		return false
	}
	viewCall, ok := ps2110Unparen(viewBind.Rhs[0]).(*ast.CallExpr)
	if !ok || ps6137CallID(pass, viewCall) != "unsafe.String" {
		return false
	}
	if len(viewCall.Args) != 2 {
		return false
	}
	data, ok := ps2110Unparen(viewCall.Args[0]).(*ast.CallExpr)
	if !ok || len(data.Args) != 1 {
		return false
	}
	bytes := ps6137Local(pass, data.Args[0])
	length := ps6137Local(pass, viewCall.Args[1])
	bytesBind, ok := ps6137Assignment(list[2])
	if !ok || ps6137Local(pass, bytesBind.Lhs[0]) != bytes {
		return false
	}
	lengthBind, ok := ps6137Assignment(list[3])
	if !ok || ps6137Local(pass, lengthBind.Lhs[0]) != length {
		return false
	}
	if _, ok := list[4].(*ast.ForStmt); !ok {
		return false
	}
	if !ps6137CacheLoop(pass, list[0], receiver, token, view, roles, false) {
		return false
	}
	if _, ok := ps6137CacheLookup(pass, list[1], receiver, token, roles.rest, nil); !ok {
		return false
	}
	if !ps6137CacheLoop(pass, list[6], receiver, token, view, roles, true) {
		return false
	}
	if _, ok := ps6137CacheLookup(pass, list[7], receiver, view, roles.text, func(stmt ast.Stmt, value types.Object) bool {
		return ps6137CacheMapWrite(pass, stmt, receiver, token, value, roles.rest)
	}); !ok {
		return false
	}
	cloneBind, ok := ps6137Assignment(list[8])
	if !ok {
		return false
	}
	owned := ps6137Local(pass, cloneBind.Lhs[0])
	clone, ok := ps2110Unparen(cloneBind.Rhs[0]).(*ast.CallExpr)
	if !ok || owned == nil || ps6137CallID(pass, clone) != "strings.Clone" || len(clone.Args) != 1 || !ps6135Object(pass, clone.Args[0], view) {
		return false
	}
	capacity, ok := list[9].(*ast.IfStmt)
	if !ok || len(capacity.Body.List) != 3 {
		return false
	}
	copy := *capacity
	copy.Body = &ast.BlockStmt{List: capacity.Body.List[:2]}
	if !ps6137CacheCapacity(pass, &copy, receiver, token, roles, func(e ast.Expr) bool { return ps6135Object(pass, e, owned) }) || !ps6137OneReturn(pass, capacity.Body.List[2], func(e ast.Expr) bool { return ps6135Object(pass, e, owned) }) {
		return false
	}
	allocate, ok := list[10].(*ast.IfStmt)
	if !ok || allocate.Init != nil || allocate.Else != nil || len(allocate.Body.List) != 2 {
		return false
	}
	nilCheck, ok := ps2110Unparen(allocate.Cond).(*ast.BinaryExpr)
	if !ok || nilCheck.Op != goToken.EQL || !ps6137CacheField(pass, nilCheck.X, receiver, roles.rest) || !ps6137Nil(pass, nilCheck.Y) {
		return false
	}
	for i, field := range []string{roles.rest, roles.text} {
		a, ok := allocate.Body.List[i].(*ast.AssignStmt)
		if !ok || a.Tok != goToken.ASSIGN || len(a.Lhs) != 1 || len(a.Rhs) != 1 || !ps6137CacheField(pass, a.Lhs[0], receiver, field) {
			return false
		}
		makeCall, ok := ps2110Unparen(a.Rhs[0]).(*ast.CallExpr)
		if !ok || len(makeCall.Args) != 1 {
			return false
		}
		id, ok := ps2110Unparen(makeCall.Fun).(*ast.Ident)
		if !ok || identObject(pass, id) != types.Universe.Lookup("make") {
			return false
		}
	}
	return ps6137CacheMapWrite(pass, list[11], receiver, token, owned, roles.rest) && ps6137CacheMapWrite(pass, list[12], receiver, owned, owned, roles.text) && ps6137OneReturn(pass, list[13], func(e ast.Expr) bool { return ps6135Object(pass, e, owned) })
}

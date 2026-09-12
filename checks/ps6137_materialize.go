package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
)

func ps6137Count(pass *analysis.Pass, e ast.Expr, count types.Object) bool {
	if ps6135Object(pass, e, count) {
		return true
	}
	call, ok := ps2110Unparen(e).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || !ps6135Object(pass, call.Args[0], count) {
		return false
	}
	id, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	return ok && identObject(pass, id) == types.Universe.Lookup("int")
}

func ps6137Number(pass *analysis.Pass, e ast.Expr, n int64) bool {
	v := pass.TypesInfo.Types[ps2110Unparen(e)].Value
	if v == nil || v.Kind() != constant.Int {
		return false
	}
	i, ok := constant.Int64Val(v)
	return ok && i == n
}

func ps6137Events(pass *analysis.Pass, e ast.Expr, result types.Object, c *config.NativeSnapshotReuseContract) bool {
	if c.ResultSliceField == "" {
		return ps6135Object(pass, e, result)
	}
	selector, ok := ps2110Unparen(e).(*ast.SelectorExpr)
	return ok && selector.Sel.Name == c.ResultSliceField && ps6135Object(pass, selector.X, result)
}

func ps6137LenEvents(pass *analysis.Pass, e ast.Expr, result types.Object, c *config.NativeSnapshotReuseContract) bool {
	call, ok := ps2110Unparen(e).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || !ps6137Events(pass, call.Args[0], result, c) {
		return false
	}
	id, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	return ok && identObject(pass, id) == types.Universe.Lookup("len")
}

func ps6137NumericSource(pass *analysis.Pass, e ast.Expr, source func(ast.Expr) bool) bool {
	if source(e) {
		return true
	}
	call, ok := ps2110Unparen(e).(*ast.CallExpr)
	return ok && len(call.Args) == 1 && pass.TypesInfo.Types[call.Fun].IsType() && ps6137Integer(pass.TypesInfo.TypeOf(call)) && ps6137NumericSource(pass, call.Args[0], source)
}

func ps6137Fields(literal *ast.CompositeLit) (map[string]ast.Expr, bool) {
	fields := make(map[string]ast.Expr, len(literal.Elts))
	for _, element := range literal.Elts {
		kv, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return nil, false
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || fields[key.Name] != nil {
			return nil, false
		}
		fields[key.Name] = kv.Value
	}
	return fields, true
}

func ps6137EventLiteral(pass *analysis.Pass, literal *ast.CompositeLit, event types.Object, c *config.NativeSnapshotReuseContract, label func(ast.Expr) bool) bool {
	fields, ok := ps6137Fields(literal)
	if !ok || len(fields) != 1+len(c.EventNumericFields) || !label(fields[c.ResultStringField]) {
		return false
	}
	for _, role := range c.EventNumericFields {
		if fields[role.ResultField] == nil || !ps6137NumericSource(pass, fields[role.ResultField], func(e ast.Expr) bool {
			s, ok := ps2110Unparen(e).(*ast.SelectorExpr)
			return ok && s.Sel.Name == role.NativeField && ps6135Object(pass, s.X, event)
		}) {
			return false
		}
	}
	return true
}

func ps6137LabelPointer(pass *analysis.Pass, e ast.Expr, event types.Object, c *config.NativeSnapshotReuseContract) bool {
	u, ok := ps2110Unparen(e).(*ast.UnaryExpr)
	if !ok || u.Op != token.AND {
		return false
	}
	index, ok := ps2110Unparen(u.X).(*ast.IndexExpr)
	if !ok || !ps6137Zero(pass, index.Index) {
		return false
	}
	field, ok := ps2110Unparen(index.X).(*ast.SelectorExpr)
	return ok && field.Sel.Name == c.NativeStringField && ps6135Object(pass, field.X, event)
}

func ps6137ResultLiteral(pass *analysis.Pass, literal *ast.CompositeLit, a ps6137Acquisition, c *config.NativeSnapshotReuseContract, events func(ast.Expr) bool, span func(ast.Expr) bool) bool {
	fields, ok := ps6137Fields(literal)
	if !ok || !events(fields[c.ResultSliceField]) {
		return false
	}
	want := 1 + len(c.ScalarResultFields)
	if span != nil {
		want++
		if !span(fields[c.EventSpanField]) {
			return false
		}
	}
	if len(fields) != want {
		return false
	}
	for i, name := range c.ScalarResultFields {
		if fields[name] == nil || !ps6137NumericSource(pass, fields[name], func(e ast.Expr) bool { return ps6135Object(pass, e, a.scalars[i]) }) {
			return false
		}
	}
	return true
}

func ps6137Span(pass *analysis.Pass, e ast.Expr, event types.Object, c *config.NativeSnapshotReuseContract) bool {
	b, ok := ps2110Unparen(e).(*ast.BinaryExpr)
	if !ok || b.Op != token.ADD {
		return false
	}
	field := func(e ast.Expr, name string) bool {
		s, ok := ps2110Unparen(e).(*ast.SelectorExpr)
		return ok && s.Sel.Name == name && ps6135Object(pass, s.X, event)
	}
	return field(b.X, c.EventStartField) && field(b.Y, c.EventDurationField)
}

func ps6137Single(pass *analysis.Pass, f ps6137Function, stmt ast.Stmt, a ps6137Acquisition, c *config.NativeSnapshotReuseContract, resultType types.Type) bool {
	branch, ok := stmt.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 3 {
		return false
	}
	equal, ok := ps2110Unparen(branch.Cond).(*ast.BinaryExpr)
	if !ok || equal.Op != token.EQL || !ps6135Object(pass, equal.X, a.count) || !ps6137Number(pass, equal.Y, 1) {
		return false
	}
	bind, ok := branch.Body.List[0].(*ast.AssignStmt)
	if !ok || bind.Tok != token.DEFINE || len(bind.Lhs) != 1 || len(bind.Rhs) != 1 || !ps6135Object(pass, bind.Rhs[0], a.pointer) {
		return false
	}
	event := ps6137Local(pass, bind.Lhs[0])
	if event == nil {
		return false
	}
	owned, ok := branch.Body.List[1].(*ast.AssignStmt)
	if !ok || owned.Tok != token.DEFINE || len(owned.Lhs) != 1 || len(owned.Rhs) != 1 {
		return false
	}
	record := ps6137Local(pass, owned.Lhs[0])
	literal, ok := ps2110Unparen(owned.Rhs[0]).(*ast.CompositeLit)
	if !ok || record == nil {
		return false
	}
	if !ps6137EventLiteral(pass, literal, event, c, func(e ast.Expr) bool {
		call, ok := ps2110Unparen(e).(*ast.CallExpr)
		return ok && len(call.Args) == 1 && ps6137Call(pass, f.file, call, "C.GoString") && ps6137LabelPointer(pass, call.Args[0], event, c) && types.Identical(pass.TypesInfo.TypeOf(call), types.Typ[types.String])
	}) {
		return false
	}
	r, ok := branch.Body.List[2].(*ast.ReturnStmt)
	if !ok || len(r.Results) != 2 || !ps6137Nil(pass, r.Results[1]) {
		return false
	}
	out, ok := ps2110Unparen(r.Results[0]).(*ast.CompositeLit)
	if !ok || !types.Identical(pass.TypesInfo.TypeOf(out), resultType) {
		return false
	}
	events := func(e ast.Expr) bool {
		lit, ok := ps2110Unparen(e).(*ast.CompositeLit)
		return ok && len(lit.Elts) == 1 && ps6135Object(pass, lit.Elts[0], record)
	}
	if c.ResultSliceField == "" {
		return events(out)
	}
	var span func(ast.Expr) bool
	if c.EventSpanField != "" {
		span = func(e ast.Expr) bool { return ps6137Span(pass, e, record, c) }
	}
	return ps6137ResultLiteral(pass, out, a, c, events, span)
}

func ps6137Make(pass *analysis.Pass, e ast.Expr, count types.Object, sliceType types.Type) bool {
	call, ok := ps2110Unparen(e).(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || !ps6137Count(pass, call.Args[1], count) || !types.Identical(types.Unalias(pass.TypesInfo.TypeOf(call)).Underlying(), types.Unalias(sliceType).Underlying()) {
		return false
	}
	id, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	return ok && identObject(pass, id) == types.Universe.Lookup("make")
}

func ps6137Assignment(stmt ast.Stmt) (*ast.AssignStmt, bool) {
	a, ok := stmt.(*ast.AssignStmt)
	return a, ok && a.Tok == token.DEFINE && len(a.Lhs) == 1 && len(a.Rhs) == 1
}

func ps6137Materialize(pass *analysis.Pass, f ps6137Function, a ps6137Acquisition, c *config.NativeSnapshotReuseContract, resultType types.Type, slice *types.Slice, fill ps6137Function) bool {
	list := f.decl.Body.List
	next := a.index + 3
	if next >= len(list) {
		return false
	}
	if !ps6137Single(pass, f, list[next], a, c, resultType) {
		return false
	}
	next++
	if len(list) != next+4 {
		return false
	}
	out, ok := ps6137Assignment(list[next])
	if !ok {
		return false
	}
	result := ps6137Local(pass, out.Lhs[0])
	if result == nil || !types.Identical(result.Type(), resultType) {
		return false
	}
	makeEvents := func(e ast.Expr) bool { return ps6137Make(pass, e, a.count, slice) }
	if c.ResultSliceField == "" {
		if !makeEvents(out.Rhs[0]) {
			return false
		}
	} else {
		literal, ok := ps2110Unparen(out.Rhs[0]).(*ast.CompositeLit)
		if !ok || !ps6137ResultLiteral(pass, literal, a, c, makeEvents, nil) {
			return false
		}
	}
	native, ok := ps6137Assignment(list[next+1])
	if !ok {
		return false
	}
	view := ps6137Local(pass, native.Lhs[0])
	call, ok := ps2110Unparen(native.Rhs[0]).(*ast.CallExpr)
	if !ok || view == nil || ps6137CallID(pass, call) != "unsafe.Slice" || len(call.Args) != 2 || !ps6135Object(pass, call.Args[0], a.pointer) || !ps6137Count(pass, call.Args[1], a.count) {
		return false
	}
	if !ps6137TokenFill(pass, f, list[next+2], result, view, c, fill) {
		return false
	}
	r, ok := list[next+3].(*ast.ReturnStmt)
	return ok && len(r.Results) == 2 && ps6135Object(pass, r.Results[0], result) && ps6137Nil(pass, r.Results[1])
}

func ps6137TokenFill(pass *analysis.Pass, f ps6137Function, stmt ast.Stmt, result, view types.Object, c *config.NativeSnapshotReuseContract, fill ps6137Function) bool {
	branch, ok := stmt.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 3 {
		return false
	}
	greater, ok := ps2110Unparen(branch.Cond).(*ast.BinaryExpr)
	if !ok || greater.Op != token.GTR || !ps6137LenEvents(pass, greater.X, result, c) || !ps6137Number(pass, greater.Y, 1) {
		return false
	}
	decl, ok := branch.Body.List[0].(*ast.DeclStmt)
	if !ok {
		return false
	}
	gen, ok := decl.Decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.VAR || len(gen.Specs) != 1 {
		return false
	}
	spec, ok := gen.Specs[0].(*ast.ValueSpec)
	if !ok || len(spec.Names) != 1 || len(spec.Values) != 0 {
		return false
	}
	tokens := ps6137Local(pass, spec.Names[0])
	if tokens == nil {
		return false
	}
	p, ok := types.Unalias(tokens.Type()).(*types.Pointer)
	if !ok || !ps6137Integer(p.Elem()) {
		return false
	}
	guard, ok := branch.Body.List[1].(*ast.IfStmt)
	if !ok || guard.Init == nil || guard.Else != nil || len(guard.Body.List) != 1 {
		return false
	}
	init, ok := ps6137Assignment(guard.Init)
	if !ok {
		return false
	}
	status := ps6137Local(pass, init.Lhs[0])
	call, ok := ps2110Unparen(init.Rhs[0]).(*ast.CallExpr)
	if !ok || status == nil || !ps6137Integer(status.Type()) || !ps6137Call(pass, f.file, call, c.TokensCallable) {
		return false
	}
	if _, generated := ps2110Unparen(call.Fun).(*ast.FuncLit); generated {
		call, ok = ps6137GeneratedArguments(pass, call)
		if !ok {
			return false
		}
	}
	if len(call.Args) != 2 || ps6137AddressLocal(pass, call.Args[1]) != tokens {
		return false
	}
	_, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature.Variadic() || signature.Params().Len() != 2 || signature.Results().Len() != 1 || !ps6137Integer(signature.Results().At(0).Type()) {
		return false
	}
	for i, arg := range call.Args {
		if !types.Identical(pass.TypesInfo.TypeOf(arg), signature.Params().At(i).Type()) {
			return false
		}
	}
	handle, ok := ps2110Unparen(call.Args[0]).(*ast.SelectorExpr)
	if !ok || handle.Sel.Name != c.ReceiverHandleField || !ps6135Object(pass, handle.X, f.sig.Recv()) {
		return false
	}
	or, ok := ps2110Unparen(guard.Cond).(*ast.BinaryExpr)
	if !ok || or.Op != token.LOR || !ps6137Binary(pass, or.X, token.NEQ, status, true) {
		return false
	}
	nilCheck, ok := ps2110Unparen(or.Y).(*ast.BinaryExpr)
	if !ok || nilCheck.Op != token.EQL || !ps6135Object(pass, nilCheck.X, tokens) || !ps6137Nil(pass, nilCheck.Y) {
		return false
	}
	// This auxiliary error occurs after a fresh allocation in the authentic
	// allocating API. No caller-owned destination has yet been mutated.
	if !ps6137ErrorGuard(pass, &ast.IfStmt{Cond: guard.Cond, Body: guard.Body}, func(ast.Expr) bool { return true }) {
		return false
	}
	expr, ok := branch.Body.List[2].(*ast.ExprStmt)
	if !ok {
		return false
	}
	invoke, ok := ps2110Unparen(expr.X).(*ast.CallExpr)
	if !ok || ps6137CallID(pass, invoke) != c.FillCallable || len(invoke.Args) != 3 || !ps6135Object(pass, invoke.Args[1], view) {
		return false
	}
	if c.ResultSliceField == "" {
		if !ps6135Object(pass, invoke.Args[0], result) {
			return false
		}
	} else if ps6137AddressLocal(pass, invoke.Args[0]) != result {
		return false
	}
	nativeTokens, ok := ps2110Unparen(invoke.Args[2]).(*ast.CallExpr)
	if !ok || ps6137CallID(pass, nativeTokens) != "unsafe.Slice" || len(nativeTokens.Args) != 2 || !ps6135Object(pass, nativeTokens.Args[0], tokens) || !ps6137LenEvents(pass, nativeTokens.Args[1], result, c) {
		return false
	}
	return fill.sig.Params().Len() == 3 && types.Identical(pass.TypesInfo.TypeOf(invoke.Args[0]), fill.sig.Params().At(0).Type()) && types.Identical(pass.TypesInfo.TypeOf(invoke.Args[1]), fill.sig.Params().At(1).Type()) && types.Identical(pass.TypesInfo.TypeOf(invoke.Args[2]), fill.sig.Params().At(2).Type())
}

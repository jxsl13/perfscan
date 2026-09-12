package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
)

// ps6131Source is an observed typed routing boundary, never a config threshold.
type ps6131Source struct {
	position  token.Pos
	boundary  int64
	dtype     string
	functions map[string]*ast.FuncDecl
	owner     *ast.FuncDecl
	guard     *ast.IfStmt
}

func ps6131SourceBound(pass *analysis.Pass, c *config.DispatchCrossoverContract) (*ps6131Source, bool) {
	functions := ps6115Declarations(pass)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Body == nil {
				functions[ps6113DeclarationID(pass, fn)] = fn
			}
		}
	}
	owner := functions[c.DispatchFunction]
	if owner == nil || owner.Body == nil || owner.Recv != nil || owner.Type.TypeParams != nil {
		return nil, false
	}
	if source, ok := ps6131OwnerRoute(pass, owner, functions, c); ok {
		return source, true
	}
	fn, ok := pass.TypesInfo.Defs[owner.Name].(*types.Func)
	if !ok {
		return nil, false
	}
	sig := fn.Type().(*types.Signature)
	dtype, ok := ps6131Signature(sig, false)
	if !ok {
		return nil, false
	}
	for _, name := range []string{c.SerialPolicy, c.ParallelPolicy} {
		decl := functions[name]
		if decl == nil || decl.Body == nil {
			return nil, false
		}
		object, ok := pass.TypesInfo.Defs[decl.Name].(*types.Func)
		if !ok {
			return nil, false
		}
		other, ok := ps6131Signature(object.Type().(*types.Signature), true)
		if !ok || other != dtype {
			return nil, false
		}
	}
	for _, name := range []string{c.SerialOperation, c.ParallelOperation} {
		decl := functions[name]
		if decl == nil || decl.Body == nil {
			return nil, false
		}
		object, ok := pass.TypesInfo.Defs[decl.Name].(*types.Func)
		if !ok {
			return nil, false
		}
		other, ok := ps6131Signature(object.Type().(*types.Signature), false)
		if !ok || other != dtype {
			return nil, false
		}
	}
	// The supported operation allocates its result once, then selects between
	// exactly two direct policy calls. Additional guards/work are unsupported.
	if len(owner.Body.List) != 3 {
		return nil, false
	}
	allocate, ok := owner.Body.List[0].(*ast.AssignStmt)
	if !ok || allocate.Tok != token.DEFINE || len(allocate.Lhs) != 1 || len(allocate.Rhs) != 1 {
		return nil, false
	}
	result, ok := allocate.Lhs[0].(*ast.Ident)
	if !ok || result.Name == "_" {
		return nil, false
	}
	makeCall, ok := ps2110Unparen(allocate.Rhs[0]).(*ast.CallExpr)
	if !ok || len(makeCall.Args) != 2 {
		return nil, false
	}
	builtin, ok := pass.TypesInfo.ObjectOf(ps6131Ident(makeCall.Fun)).(*types.Builtin)
	if !ok || builtin.Name() != "make" || !types.Identical(pass.TypesInfo.TypeOf(makeCall), sig.Results().At(0).Type()) || !ps6131Length(pass, makeCall.Args[1], sig.Params().At(0)) {
		return nil, false
	}
	route, ok := owner.Body.List[1].(*ast.IfStmt)
	if !ok || route.Init != nil || route.Else == nil {
		return nil, false
	}
	condition, ok := ps2110Unparen(route.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op != token.LSS || !ps6131Length(pass, condition.X, sig.Params().At(0)) {
		return nil, false
	}
	threshold, ok := pass.TypesInfo.ObjectOf(ps6131Ident(condition.Y)).(*types.Const)
	if !ok || threshold.Pkg() != pass.Pkg || pass.Pkg.Path()+"."+threshold.Name() != c.ThresholdConstant {
		return nil, false
	}
	boundary, ok := constant.Int64Val(threshold.Val())
	if !ok || boundary < 2 {
		return nil, false
	}
	parallel, ok := route.Else.(*ast.BlockStmt)
	if !ok || !ps6131PolicyArm(pass, route.Body, c.SerialPolicy, pass.TypesInfo.ObjectOf(result), sig.Params().At(0)) || !ps6131PolicyArm(pass, parallel, c.ParallelPolicy, pass.TypesInfo.ObjectOf(result), sig.Params().At(0)) {
		return nil, false
	}
	returned, ok := owner.Body.List[2].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 || pass.TypesInfo.ObjectOf(ps6131Ident(returned.Results[0])) != pass.TypesInfo.ObjectOf(result) {
		return nil, false
	}
	// The policy-to-leaf/worker association must exist in actual typed source.
	// All direct local callees are walked, including synchronous worker callback
	// bodies. Uninvoked closures, defer/go and unknown function-value edges fail
	// closed rather than turning an unused configured name into a dependency.
	serial, ok := ps6131Closure(pass, functions, c.SerialPolicy)
	if !ok {
		return nil, false
	}
	par, ok := ps6131Closure(pass, functions, c.ParallelPolicy)
	if !ok || !par[c.WorkerRunner] {
		return nil, false
	}
	for _, name := range c.LeafKernels {
		if !serial[name] || !par[name] {
			return nil, false
		}
	}
	for _, pair := range [][2]string{{c.SerialOperation, c.SerialPolicy}, {c.ParallelOperation, c.ParallelPolicy}} {
		closure, ok := ps6131Closure(pass, functions, pair[0])
		if !ok || !closure[pair[1]] {
			return nil, false
		}
	}
	return &ps6131Source{position: threshold.Pos(), boundary: boundary, dtype: dtype, functions: functions, owner: owner, guard: route}, true
}

// OwnerRoute covers the actual #914 operation: a dtype-specific block takes
// a serial leaf below the threshold and exits its switch case, otherwise a
// worker callback slices the same output/input. The allocated operation may
// return tensors/errors; it is not incorrectly narrowed to []T APIs.
func ps6131OwnerRoute(pass *analysis.Pass, owner *ast.FuncDecl, functions map[string]*ast.FuncDecl, c *config.DispatchCrossoverContract) (*ps6131Source, bool) {
	var found *ps6131Source
	parents := ps6071Parents(owner.Body)
	flow := ps6122NewFlow(pass, owner.Body)
	ast.Inspect(owner.Body, func(n ast.Node) bool {
		if _, literal := n.(*ast.FuncLit); literal {
			return false
		}
		guard, ok := n.(*ast.IfStmt)
		if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 2 {
			return true
		}
		condition, ok := ps2110Unparen(guard.Cond).(*ast.BinaryExpr)
		if !ok || condition.Op != token.LSS {
			return true
		}
		if !ps6122CanEnter(pass, owner.Body, flow.parents, condition.Pos()) || ps6122BlockAt(pass, flow, condition.Pos()) == nil {
			return true
		}
		threshold, ok := pass.TypesInfo.ObjectOf(ps6131Ident(condition.Y)).(*types.Const)
		if !ok || threshold.Pkg() != pass.Pkg || pass.Pkg.Path()+"."+threshold.Name() != c.ThresholdConstant {
			return true
		}
		boundary, ok := constant.Int64Val(threshold.Val())
		if !ok || boundary < 2 {
			return true
		}
		length, ok := ps2110Unparen(condition.X).(*ast.CallExpr)
		if !ok || len(length.Args) != 1 {
			return true
		}
		dst := pass.TypesInfo.ObjectOf(ps6131Ident(length.Args[0]))
		if dst == nil || !ps6131Length(pass, condition.X, dst) {
			return true
		}
		call := ps6032StatementCall(guard.Body.List[0])
		if call == nil || len(call.Args) != 2 {
			return true
		}
		fn := ps6071CalledFunction(pass, call)
		if fn == nil || ps6090FunctionID(fn) != c.SerialPolicy {
			return true
		}
		src := pass.TypesInfo.ObjectOf(ps6131Ident(call.Args[1]))
		if src == nil || src == dst || pass.TypesInfo.ObjectOf(ps6131Ident(call.Args[0])) != dst {
			return true
		}
		dtype, ok := ps6131Signature(fn.Type().(*types.Signature), true)
		if !ok {
			return true
		}
		branch, ok := guard.Body.List[1].(*ast.BranchStmt)
		if !ok || branch.Tok != token.BREAK || branch.Label != nil {
			return true
		}
		// Find the exact successor in the containing switch case. A break
		// inside a loop or nested switch does not establish this route.
		clause, ok := parents[guard].(*ast.CaseClause)
		if !ok {
			return true
		}
		index := -1
		for i, statement := range clause.Body {
			if statement == guard {
				index = i
			}
		}
		if index < 0 || index+2 != len(clause.Body) {
			return true
		}
		parallel := ps6032StatementCall(clause.Body[index+1])
		if parallel == nil {
			return true
		}
		worker := ps6071CalledFunction(pass, parallel)
		if parallel == nil || worker == nil || ps6090FunctionID(worker) != c.WorkerRunner || len(parallel.Args) != 2 || !ps6131Length(pass, parallel.Args[0], dst) {
			return true
		}
		callback, ok := ps2110Unparen(parallel.Args[1]).(*ast.FuncLit)
		if !ok || len(callback.Body.List) != 1 {
			return true
		}
		invocation := ps6032StatementCall(callback.Body.List[0])
		if invocation == nil || ps6071CalledFunction(pass, invocation) != fn || len(invocation.Args) != 2 {
			return true
		}
		cbtype := pass.TypesInfo.TypeOf(callback).(*types.Signature)
		if cbtype.Params().Len() != 2 {
			return true
		}
		for i, object := range []types.Object{dst, src} {
			slice, ok := ps2110Unparen(invocation.Args[i]).(*ast.SliceExpr)
			if !ok || slice.Slice3 || pass.TypesInfo.ObjectOf(ps6131Ident(slice.X)) != object || pass.TypesInfo.ObjectOf(ps6131Ident(slice.Low)) != cbtype.Params().At(0) || pass.TypesInfo.ObjectOf(ps6131Ident(slice.High)) != cbtype.Params().At(1) {
				return true
			}
		}
		closure, ok := ps6131Closure(pass, functions, c.SerialPolicy)
		if !ok {
			return true
		}
		for _, leaf := range c.LeafKernels {
			if !closure[leaf] {
				return true
			}
		}
		if found != nil {
			found = &ps6131Source{}
			return false
		}
		found = &ps6131Source{position: threshold.Pos(), boundary: boundary, dtype: dtype, functions: functions, owner: owner, guard: guard}
		return true
	})
	return found, found != nil && found.position != token.NoPos
}

func ps6131Ident(expr ast.Expr) *ast.Ident {
	id, _ := ps2110Unparen(expr).(*ast.Ident)
	return id
}

func ps6131Signature(sig *types.Signature, policy bool) (string, bool) {
	if sig.Recv() != nil || sig.Variadic() || sig.TypeParams().Len() != 0 {
		return "", false
	}
	parameters, results := 1, 1
	if policy {
		parameters, results = 2, 0
	}
	if sig.Params().Len() != parameters || sig.Results().Len() != results {
		return "", false
	}
	slice, ok := types.Unalias(sig.Params().At(0).Type()).(*types.Slice)
	if !ok {
		return "", false
	}
	basic, ok := types.Unalias(slice.Elem()).(*types.Basic)
	if !ok || basic.Kind() != types.Float32 && basic.Kind() != types.Float64 {
		return "", false
	}
	for i := 1; i < parameters; i++ {
		if !types.Identical(sig.Params().At(i).Type(), sig.Params().At(0).Type()) {
			return "", false
		}
	}
	if !policy && !types.Identical(sig.Results().At(0).Type(), sig.Params().At(0).Type()) {
		return "", false
	}
	return basic.Name(), true
}

func ps6131Length(pass *analysis.Pass, expr ast.Expr, input types.Object) bool {
	call, ok := ps2110Unparen(expr).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	builtin, ok := pass.TypesInfo.ObjectOf(ps6131Ident(call.Fun)).(*types.Builtin)
	return ok && builtin.Name() == "len" && pass.TypesInfo.ObjectOf(ps6131Ident(call.Args[0])) == input
}

func ps6131PolicyArm(pass *analysis.Pass, body *ast.BlockStmt, name string, dst, src types.Object) bool {
	if len(body.List) != 1 {
		return false
	}
	statement, ok := body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := ps2110Unparen(statement.X).(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return false
	}
	fn := ps6071CalledFunction(pass, call)
	return fn != nil && ps6090FunctionID(fn) == name && pass.TypesInfo.ObjectOf(ps6131Ident(call.Args[0])) == dst && pass.TypesInfo.ObjectOf(ps6131Ident(call.Args[1])) == src
}

// Closure records potential implementation dependencies, not guaranteed
// execution counts. Recursion, unknown indirect edges and imported bodies
// cannot establish the configured leaf association in this bounded grammar.
func ps6131Closure(pass *analysis.Pass, functions map[string]*ast.FuncDecl, root string) (map[string]bool, bool) {
	seen := map[string]bool{}
	active := map[string]bool{}
	var visit func(string) bool
	var body func(*ast.BlockStmt, *types.Signature) bool
	body = func(block *ast.BlockStmt, signature *types.Signature) bool {
		flow := ps6122NewFlow(pass, block)
		parameters := make(map[types.Object]bool, signature.Params().Len())
		for i := range signature.Params().Len() {
			parameters[signature.Params().At(i)] = true
		}
		valid := true
		ast.Inspect(block, func(n ast.Node) bool {
			if !valid {
				return false
			}
			if _, literal := n.(*ast.FuncLit); literal {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			for ancestor := flow.parents[call]; ancestor != nil; ancestor = flow.parents[ancestor] {
				switch ancestor.(type) {
				case *ast.GoStmt, *ast.DeferStmt:
					valid = false
					return false
				}
			}
			if !ps6122CanEnter(pass, block, flow.parents, call.Pos()) || ps6122BlockAt(pass, flow, call.Pos()) == nil {
				return false
			}
			fn := ps6071CalledFunction(pass, call)
			if fn == nil {
				// Builtins and conversions cannot name implementation leaves.
				if id := ps6131Ident(call.Fun); id != nil {
					object := pass.TypesInfo.ObjectOf(id)
					switch object.(type) {
					case *types.Builtin, *types.TypeName:
						return true
					}
					// Callback parameter invocation is an implementation edge whose
					// actual literal is separately joined at the direct callsite.
					if variable, ok := object.(*types.Var); ok && parameters[variable] {
						if _, ok := variable.Type().Underlying().(*types.Signature); ok && ps6128ParameterUnmodified(pass, block, variable) {
							return true
						}
					}
				}
				valid = false
				return false
			}
			name := ps6090FunctionID(fn)
			if fn.Pkg() != pass.Pkg {
				// External standard-library scheduling is included in controlled
				// build materials, but supplies no configured local leaf facts.
				for _, arg := range call.Args {
					if typ := pass.TypesInfo.TypeOf(arg); typ != nil {
						if _, ok := typ.Underlying().(*types.Signature); ok {
							valid = false
							return false
						}
					}
				}
				return true
			}
			if !visit(name) {
				valid = false
				return false
			}
			decl := functions[name]
			sig := fn.Type().(*types.Signature)
			for i, arg := range call.Args {
				literal, ok := ps2110Unparen(arg).(*ast.FuncLit)
				if !ok {
					if typ := pass.TypesInfo.TypeOf(arg); typ != nil {
						if _, callable := typ.Underlying().(*types.Signature); callable {
							object := pass.TypesInfo.ObjectOf(ps6131Ident(arg))
							direct, ok := object.(*types.Func)
							if !ok || direct.Pkg() != pass.Pkg || !visit(ps6090FunctionID(direct)) || i >= sig.Params().Len() || decl == nil || decl.Body == nil || !ps6131InvokesParameter(pass, decl.Body, sig.Params().At(i)) {
								valid = false
								return false
							}
						}
					}
					continue
				}
				if i >= sig.Params().Len() || decl == nil || decl.Body == nil || !ps6131InvokesParameter(pass, decl.Body, sig.Params().At(i)) || !body(literal.Body, pass.TypesInfo.TypeOf(literal).(*types.Signature)) {
					valid = false
					return false
				}
			}
			return true
		})
		return valid
	}
	visit = func(name string) bool {
		if active[name] || len(seen) >= 64 {
			return false
		}
		if seen[name] {
			return true
		}
		decl := functions[name]
		if decl == nil {
			return false
		}
		seen[name] = true
		if decl.Body == nil {
			return true
		} // native leaf declaration; materials retain assembly.
		active[name] = true
		fn, ok := pass.TypesInfo.Defs[decl.Name].(*types.Func)
		if !ok {
			return false
		}
		ok = body(decl.Body, fn.Type().(*types.Signature))
		delete(active, name)
		return ok
	}
	return seen, visit(root)
}

func ps6131InvokesParameter(pass *analysis.Pass, block *ast.BlockStmt, parameter types.Object) bool {
	if !ps6128ParameterUnmodified(pass, block, parameter) {
		return false
	}
	flow := ps6122NewFlow(pass, block)
	found := false
	ast.Inspect(block, func(n ast.Node) bool {
		if _, literal := n.(*ast.FuncLit); literal {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if ok {
			for ancestor := flow.parents[call]; ancestor != nil; ancestor = flow.parents[ancestor] {
				switch ancestor.(type) {
				case *ast.GoStmt, *ast.DeferStmt:
					return false
				}
			}
		}
		if ok && pass.TypesInfo.ObjectOf(ps6131Ident(call.Fun)) == parameter && ps6122CanEnter(pass, block, flow.parents, call.Pos()) && ps6122BlockAt(pass, flow, call.Pos()) != nil {
			found = true
		}
		return !found
	})
	return found
}

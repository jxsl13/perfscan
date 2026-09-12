package checks

import (
	"bytes"
	_ "embed"
	"errors"
	"go/ast"
	"go/constant"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"strings"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

//go:embed testdata/ps6131_owner_benchmarks.go.txt
var ps6131OwnerBenchmarkSource string

// ps6131Original binds the bounded, committed benchmark shape matrix to the
// owner operation. A benchmark label alone is not evidence of this association.
// Registration is a source association, not a whole-program registry proof.
func ps6131Original(pkg *packages.Package, source *ps6131Source, c *config.DispatchCrossoverContract) ([]int, error) {
	if source.dtype != "float32" {
		return nil, errors.New("the supported original owner adapter qualifies only the observed F32 dispatch case")
	}
	name, suffix, ok := strings.Cut(c.ProductionBenchmark, "/")
	if !ok || suffix != "n{n}" {
		return nil, errors.New("original benchmark requires the observed n{n} subbenchmark grammar")
	}
	var benchmark *ast.FuncDecl
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name {
				benchmark = fn
			}
		}
	}
	if benchmark == nil || benchmark.Recv != nil || benchmark.Body == nil {
		return nil, errors.New("original benchmark declaration is unavailable")
	}
	if !ps6131CanonicalOriginal(pkg, source, benchmark) {
		return nil, errors.New("original benchmark differs from the supported typed owner-shaped timed route")
	}
	owner := pkg.TypesInfo.Defs[source.owner.Name]
	allocation, ok := source.owner.Body.List[1].(*ast.AssignStmt)
	if !ok || len(allocation.Rhs) != 1 {
		return nil, errors.New("owner allocation source is unsupported")
	}
	allocationCall, ok := allocation.Rhs[0].(*ast.CallExpr)
	if !ok {
		return nil, errors.New("owner allocation constructor is unsupported")
	}
	allocationFunction, ok := pkg.TypesInfo.ObjectOf(ps6131IdentSelector(allocationCall.Fun)).(*types.Func)
	if !ok {
		return nil, errors.New("owner allocation constructor identity is unavailable")
	}
	ops := make(map[types.Object]bool)
	pass := &analysis.Pass{Fset: pkg.Fset, Files: pkg.Syntax, Pkg: pkg.Types, TypesInfo: pkg.TypesInfo}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			init, ok := decl.(*ast.FuncDecl)
			if !ok || init.Name.Name != "init" || init.Body == nil {
				continue
			}
			flow := ps6122NewFlow(pass, init.Body)
			ast.Inspect(init.Body, func(node ast.Node) bool {
				if _, literal := node.(*ast.FuncLit); literal {
					return false
				}
				call, ok := node.(*ast.CallExpr)
				if !ok || !ps6122CanEnter(pass, init.Body, flow.parents, call.Pos()) || ps6122BlockAt(pass, flow, call.Pos()) == nil {
					return true
				}
				if !ps6131Registration(pkg, call, allocationFunction) {
					return true
				}
				for _, arg := range call.Args {
					if pkg.TypesInfo.ObjectOf(ps6131Ident(arg)) != owner {
						continue
					}
					// Both the owner's reg(OpAbs, kernel) closure and a direct
					// Register(OpAbs, kernel) declaration have an exact typed Op.
					if len(call.Args) >= 2 {
						if op, ok := pkg.TypesInfo.ObjectOf(ps6131IdentSelector(call.Args[0])).(*types.Const); ok {
							ops[op] = true
						}
					}
				}
				return true
			})
		}
	}
	var sizes []int
	qualified := 0
	contexts := ps6131OriginalContexts(pkg, benchmark)
	if len(ops) == 0 || len(contexts) == 0 {
		return nil, errors.New("original benchmark lacks observed registration or CPU context association")
	}
	ast.Inspect(benchmark.Body, func(node ast.Node) bool {
		loop, ok := node.(*ast.RangeStmt)
		if !ok || loop.Tok != token.DEFINE {
			return true
		}
		values, ok := ps2110Unparen(loop.X).(*ast.CompositeLit)
		if !ok {
			return true
		}
		n := pkg.TypesInfo.ObjectOf(ps6131Ident(loop.Value))
		if n == nil {
			return true
		}
		matrix := make([]int, 0, len(values.Elts))
		for _, value := range values.Elts {
			observed := pkg.TypesInfo.Types[value].Value
			if observed == nil {
				return true
			}
			v, ok := constant.Int64Val(observed)
			if !ok || v < 2 || int64(int(v)) != v {
				return true
			}
			matrix = append(matrix, int(v))
		}
		if !ps6131OriginalLoop(pkg, loop, n, owner, allocationFunction, ops, contexts) {
			return true
		}
		qualified++
		sizes = matrix
		return false
	})
	if qualified != 1 || len(sizes) < 2 {
		return nil, errors.New("original benchmark is not uniquely bound to owner operation and typed F32 shape matrix")
	}
	return sizes, nil
}

// The supported owner adapter is deliberately exact apart from the observed
// literal size matrix. It does not discover Execute calls in arbitrary closure,
// async, dead, reassigned or untimed code and then infer that those calls are the
// benchmark. Aliases are normalized only through observed imported objects.
func ps6131CanonicalOriginal(pkg *packages.Package, source *ps6131Source, benchmark *ast.FuncDecl) bool {
	owner, ok := pkg.TypesInfo.Defs[source.owner.Name].(*types.Func)
	if !ok {
		return false
	}
	sig := owner.Type().(*types.Signature)
	if sig.Params().Len() != 3 {
		return false
	}
	context, ok := types.Unalias(sig.Params().At(0).Type()).(*types.Pointer)
	if !ok {
		return false
	}
	contextName, ok := types.Unalias(context.Elem()).(*types.Named)
	if !ok || contextName.Obj().Pkg() == nil {
		return false
	}
	input, ok := types.Unalias(sig.Params().At(1).Type()).(*types.Slice)
	if !ok {
		return false
	}
	tensorPtr, ok := types.Unalias(input.Elem()).(*types.Pointer)
	if !ok {
		return false
	}
	tensorName, ok := types.Unalias(tensorPtr.Elem()).(*types.Named)
	if !ok || tensorName.Obj().Pkg() == nil {
		return false
	}
	var encoded bytes.Buffer
	if format.Node(&encoded, pkg.Fset, benchmark) != nil {
		return false
	}
	actual, err := parser.ParseFile(token.NewFileSet(), "actual.go", "package cpu\n"+encoded.String(), 0)
	if err != nil {
		return false
	}
	var originalIDs, copyIDs []*ast.Ident
	ast.Inspect(benchmark, func(node ast.Node) bool {
		if id, ok := node.(*ast.Ident); ok {
			originalIDs = append(originalIDs, id)
		}
		return true
	})
	actualFn := actual.Decls[0].(*ast.FuncDecl)
	ast.Inspect(actualFn, func(node ast.Node) bool {
		if id, ok := node.(*ast.Ident); ok {
			copyIDs = append(copyIDs, id)
		}
		return true
	})
	if len(originalIDs) != len(copyIDs) {
		return false
	}
	for i, id := range originalIDs {
		imported, ok := pkg.TypesInfo.ObjectOf(id).(*types.PkgName)
		if !ok {
			continue
		}
		switch imported.Imported() {
		case contextName.Obj().Pkg():
			copyIDs[i].Name = "backend"
		case tensorName.Obj().Pkg():
			copyIDs[i].Name = "tensor"
		default:
			path := imported.Imported().Path()
			if path == "testing" || path == "strconv" {
				copyIDs[i].Name = path
			} else if strings.HasSuffix(path, "/internal/bench") || path == strings.TrimSuffix(pkg.PkgPath, "/cpu")+"/bench" {
				copyIDs[i].Name = "bench"
			} else {
				return false
			}
		}
	}
	expectedFile, err := parser.ParseFile(token.NewFileSet(), "owner.go", ps6131OwnerBenchmarkSource, 0)
	if err != nil {
		return false
	}
	var expected *ast.FuncDecl
	for _, decl := range expectedFile.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "BenchmarkAbsF32CPU" {
			expected = fn
		}
	}
	if expected == nil || len(actualFn.Body.List) != 4 {
		return false
	}
	actualLoop, ok := actualFn.Body.List[3].(*ast.RangeStmt)
	if !ok {
		return false
	}
	// The actual matrix is separately resolved from typed integer constants.
	actualLoop.X = expected.Body.List[3].(*ast.RangeStmt).X
	actualFn.Name.Name = expected.Name.Name
	encoded.Reset()
	var wanted bytes.Buffer
	return format.Node(&encoded, token.NewFileSet(), actualFn) == nil && format.Node(&wanted, token.NewFileSet(), expected) == nil && bytes.Equal(encoded.Bytes(), wanted.Bytes())
}

func ps6131OriginalLoop(pkg *packages.Package, loop *ast.RangeStmt, n, owner types.Object, allocation *types.Func, ops, contexts map[types.Object]bool) bool {
	var callback *ast.FuncLit
	ast.Inspect(loop.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Run" {
			return true
		}
		fn, ok := pkg.TypesInfo.ObjectOf(selector.Sel).(*types.Func)
		if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "testing" {
			return true
		}
		label, ok := call.Args[0].(*ast.BinaryExpr)
		if !ok || label.Op != token.ADD {
			return true
		}
		literal, ok := label.X.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING || literal.Value != "\"n\"" {
			return true
		}
		itoa, ok := label.Y.(*ast.CallExpr)
		if !ok || len(itoa.Args) != 1 || pkg.TypesInfo.ObjectOf(ps6131Ident(itoa.Args[0])) != n {
			return true
		}
		id, ok := itoa.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		format, ok := pkg.TypesInfo.ObjectOf(id.Sel).(*types.Func)
		if !ok || format.Pkg() == nil || format.Pkg().Path() != "strconv" || format.Name() != "Itoa" {
			return true
		}
		candidate, ok := call.Args[1].(*ast.FuncLit)
		if !ok || callback != nil {
			return true
		}
		callback = candidate
		return false
	})
	if callback == nil {
		return false
	}
	inputs := make(map[types.Object]bool)
	for _, stmt := range callback.Body.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			continue
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok || len(call.Args) < 1 {
			continue
		}
		shape, ok := call.Args[0].(*ast.CompositeLit)
		if !ok || len(shape.Elts) != 1 {
			continue
		}
		if pkg.TypesInfo.ObjectOf(ps6131Ident(shape.Elts[0])) != n {
			continue
		}
		if ps6131F32Constructor(pkg, call.Fun, allocation, make(map[*types.Func]bool)) {
			inputs[pkg.TypesInfo.ObjectOf(ps6131Ident(assign.Lhs[0]))] = true
		}
	}
	bound := make(map[types.Object]bool)
	for _, stmt := range callback.Body.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			continue
		}
		list, ok := assign.Rhs[0].(*ast.CompositeLit)
		if !ok || len(list.Elts) != 1 {
			continue
		}
		if inputs[pkg.TypesInfo.ObjectOf(ps6131Ident(list.Elts[0]))] {
			bound[pkg.TypesInfo.ObjectOf(ps6131Ident(assign.Lhs[0]))] = true
		}
	}
	calls := 0
	valid := true
	ast.Inspect(callback.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		id := ps6131Ident(call.Fun)
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
			id = selector.Sel
		}
		fn := pkg.TypesInfo.ObjectOf(id)
		if fn == owner {
			valid = false // Tensor path requires the original backend dispatch.
		}
		f, ok := fn.(*types.Func)
		if !ok || f.Name() != "Execute" {
			return true
		}
		calls++
		if len(call.Args) != 4 || !contexts[pkg.TypesInfo.ObjectOf(ps6131Ident(call.Args[0]))] || !ops[pkg.TypesInfo.ObjectOf(ps6131IdentSelector(call.Args[1]))] || !bound[pkg.TypesInfo.ObjectOf(ps6131Ident(call.Args[2]))] {
			valid = false
		}
		return true
	})
	return valid && calls >= 2
}

func ps6131OriginalContexts(pkg *packages.Package, benchmark *ast.FuncDecl) map[types.Object]bool {
	backends := make(map[types.Object]bool)
	contexts := make(map[types.Object]bool)
	for _, stmt := range benchmark.Body.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE || len(assign.Rhs) != 1 {
			continue
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			continue
		}
		fn, ok := pkg.TypesInfo.ObjectOf(ps6131IdentSelector(call.Fun)).(*types.Func)
		if !ok || fn.Pkg() == nil {
			continue
		}
		if fn.Name() == "Get" && len(call.Args) == 1 && len(assign.Lhs) == 2 {
			cpu, ok := pkg.TypesInfo.ObjectOf(ps6131IdentSelector(call.Args[0])).(*types.Const)
			if ok && cpu.Pkg() == fn.Pkg() && cpu.Name() == "CPU" {
				backends[pkg.TypesInfo.ObjectOf(ps6131Ident(assign.Lhs[0]))] = true
			}
		}
		if fn.Name() == "WithBackend" && len(call.Args) == 1 && len(assign.Lhs) == 1 && backends[pkg.TypesInfo.ObjectOf(ps6131Ident(call.Args[0]))] {
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			create, ok := selector.X.(*ast.CallExpr)
			if !ok || len(create.Args) != 0 {
				continue
			}
			constructor, ok := pkg.TypesInfo.ObjectOf(ps6131IdentSelector(create.Fun)).(*types.Func)
			if ok && constructor.Pkg() == fn.Pkg() && constructor.Name() == "NewContext" {
				contexts[pkg.TypesInfo.ObjectOf(ps6131Ident(assign.Lhs[0]))] = true
			}
		}
	}
	return contexts
}

// Follow the selected, typed constructor bodies, rather than assuming a helper
// named RandF32 returns F32 or preserves its shape parameter.
func ps6131F32Constructor(root *packages.Package, expr ast.Expr, allocation *types.Func, seen map[*types.Func]bool) bool {
	id := ps6131Ident(expr)
	if selector, ok := expr.(*ast.SelectorExpr); ok {
		id = selector.Sel
	}
	fn, ok := root.TypesInfo.ObjectOf(id).(*types.Func)
	if !ok || fn.Pkg() == nil || seen[fn] || len(seen) >= 8 {
		return false
	}
	seen[fn] = true
	var pkg *packages.Package
	var find func(*packages.Package)
	visited := make(map[*packages.Package]bool)
	find = func(p *packages.Package) {
		if visited[p] {
			return
		}
		visited[p] = true
		if p.Types == fn.Pkg() {
			pkg = p
		}
		for _, dependency := range p.Imports {
			find(dependency)
		}
	}
	find(root)
	if pkg == nil {
		return false
	}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			body, ok := decl.(*ast.FuncDecl)
			if !ok || pkg.TypesInfo.Defs[body.Name] != fn || body.Body == nil {
				continue
			}
			sig := fn.Type().(*types.Signature)
			if sig.Params().Len() == 0 {
				return false
			}
			shape := sig.Params().At(0)
			returned := make(map[types.Object]bool)
			constructions := make(map[types.Object]*ast.CallExpr)
			for _, stmt := range body.Body.List {
				assign, ok := stmt.(*ast.AssignStmt)
				if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
					continue
				}
				call, ok := assign.Rhs[0].(*ast.CallExpr)
				if !ok {
					continue
				}
				callee, ok := pkg.TypesInfo.ObjectOf(ps6131IdentSelector(call.Fun)).(*types.Func)
				if !ok || sig.Results().Len() != 1 {
					continue
				}
				dtypeIndex, shapeIndex := -1, -1
				if callee == allocation && len(call.Args) == 3 {
					dtypeIndex, shapeIndex = 1, 2
				} else if callee.Pkg() == allocation.Pkg() && callee.Name() == "New" && len(call.Args) == 2 && ps6131NewForwards(pkg, callee, allocation) {
					dtypeIndex, shapeIndex = 0, 1
				}
				if shapeIndex >= 0 && pkg.TypesInfo.ObjectOf(ps6131Ident(call.Args[shapeIndex])) == shape {
					dtype, ok := pkg.TypesInfo.ObjectOf(ps6131IdentSelector(call.Args[dtypeIndex])).(*types.Const)
					if ok && dtype.Pkg() == allocation.Pkg() && dtype.Name() == "F32" && types.Identical(pkg.TypesInfo.TypeOf(call), sig.Results().At(0).Type()) {
						object := pkg.TypesInfo.ObjectOf(ps6131Ident(assign.Lhs[0]))
						returned[object] = true
						constructions[object] = call
					}
				}
			}
			last, ok := body.Body.List[len(body.Body.List)-1].(*ast.ReturnStmt)
			if !ok || len(last.Results) != 1 {
				return false
			}
			object := pkg.TypesInfo.ObjectOf(ps6131Ident(last.Results[0]))
			if returned[object] {
				return ps6131ConstructorStable(pkg, body, shape, returned, allocation, map[*ast.CallExpr]bool{constructions[object]: true})
			}
			call, ok := last.Results[0].(*ast.CallExpr)
			return ok && len(call.Args) > 0 && pkg.TypesInfo.ObjectOf(ps6131Ident(call.Args[0])) == shape && ps6131F32Constructor(pkg, call.Fun, allocation, seen) && ps6131ConstructorStable(pkg, body, shape, returned, allocation, map[*ast.CallExpr]bool{call: true})
		}
	}
	return false
}

func ps6131ConstructorStable(pkg *packages.Package, body *ast.FuncDecl, shape types.Object, returned map[types.Object]bool, allocation *types.Func, inspectedShapes map[*ast.CallExpr]bool) bool {
	parents := ps6071Parents(body.Body)
	valid := true
	ast.Inspect(body.Body, func(node ast.Node) bool {
		if _, literal := node.(*ast.FuncLit); literal {
			valid = false
			return false
		}
		switch node.(type) {
		case *ast.GoStmt, *ast.DeferStmt:
			valid = false
		}
		if assign, ok := node.(*ast.AssignStmt); ok {
			for _, left := range assign.Lhs {
				if ps6131ConstructorDataWrite(pkg, left, returned, allocation) {
					continue
				}
				ast.Inspect(left, func(n ast.Node) bool {
					id, ok := n.(*ast.Ident)
					if !ok {
						return true
					}
					object := pkg.TypesInfo.ObjectOf(id)
					if object == shape || returned[object] && pkg.TypesInfo.Defs[id] != object {
						valid = false
					}
					return true
				})
			}
		}
		if loop, ok := node.(*ast.RangeStmt); ok && loop.Tok != token.DEFINE {
			for _, target := range []ast.Expr{loop.Key, loop.Value} {
				ast.Inspect(target, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok {
						object := pkg.TypesInfo.ObjectOf(id)
						if object == shape || returned[object] {
							valid = false
						}
					}
					return true
				})
			}
		}
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := pkg.TypesInfo.ObjectOf(id)
		if object != shape && !returned[object] || pkg.TypesInfo.Defs[id] == object {
			return true
		}
		if selector, ok := parents[id].(*ast.SelectorExpr); ok && selector.X == id {
			call, called := parents[selector].(*ast.CallExpr)
			if object == shape && called && call.Fun == selector && len(call.Args) == 0 && ps6131ReadonlyNumel(pkg, selector, allocation) || returned[object] && ps6131StorageUse(pkg, selector, parents, allocation) {
				return true
			}
		}
		if ret, ok := parents[id].(*ast.ReturnStmt); ok && returned[object] && len(ret.Results) == 1 && ret.Results[0] == id {
			return true
		}
		if call, ok := parents[id].(*ast.CallExpr); ok && object == shape {
			if inspectedShapes[call] {
				return true
			}
		}
		valid = false
		return true
	})
	return valid
}

func ps6131ReadonlyNumel(pkg *packages.Package, selector *ast.SelectorExpr, allocation *types.Func) bool {
	fn, ok := pkg.TypesInfo.ObjectOf(selector.Sel).(*types.Func)
	if !ok || fn.Pkg() != allocation.Pkg() || fn.Name() != "Numel" {
		return false
	}
	selected, body := ps6131FunctionSource(pkg, fn)
	sig := fn.Type().(*types.Signature)
	if body == nil || body.Body == nil || len(body.Body.List) != 3 || sig.Recv() == nil || sig.Params().Len() != 0 || sig.Results().Len() != 1 {
		return false
	}
	initial, ok := body.Body.List[0].(*ast.AssignStmt)
	if !ok || initial.Tok != token.DEFINE || len(initial.Lhs) != 1 || len(initial.Rhs) != 1 {
		return false
	}
	value := selected.TypesInfo.Types[initial.Rhs[0]].Value
	if value == nil || value.Kind() != constant.Int || constant.Compare(value, token.NEQ, constant.MakeInt64(1)) {
		return false
	}
	product := selected.TypesInfo.ObjectOf(ps6131Ident(initial.Lhs[0]))
	loop, ok := body.Body.List[1].(*ast.RangeStmt)
	if !ok || loop.Tok != token.DEFINE || ps6131Ident(loop.Key) == nil || ps6131Ident(loop.Key).Name != "_" || selected.TypesInfo.ObjectOf(ps6131Ident(loop.X)) != sig.Recv() || len(loop.Body.List) != 1 {
		return false
	}
	dimension := selected.TypesInfo.ObjectOf(ps6131Ident(loop.Value))
	multiply, ok := loop.Body.List[0].(*ast.AssignStmt)
	if !ok || multiply.Tok != token.MUL_ASSIGN || len(multiply.Lhs) != 1 || len(multiply.Rhs) != 1 || selected.TypesInfo.ObjectOf(ps6131Ident(multiply.Lhs[0])) != product || selected.TypesInfo.ObjectOf(ps6131Ident(multiply.Rhs[0])) != dimension {
		return false
	}
	ret, ok := body.Body.List[2].(*ast.ReturnStmt)
	return ok && len(ret.Results) == 1 && selected.TypesInfo.ObjectOf(ps6131Ident(ret.Results[0])) == product
}

// A storage pointer is not itself a readonly use. Permit only the selected
// Tensor accessor → F32 data view, or the observed storage length field read.
// An alias, address, range target or unknown call taking storage rejects.
func ps6131StorageUse(pkg *packages.Package, selector *ast.SelectorExpr, parents map[ast.Node]ast.Node, allocation *types.Func) bool {
	var storage ast.Expr = selector
	if fn, ok := pkg.TypesInfo.ObjectOf(selector.Sel).(*types.Func); ok {
		if fn.Pkg() != allocation.Pkg() || fn.Name() != "Storage" || !ps6131ReadonlyAccessor(pkg, fn) {
			return false
		}
		call, ok := parents[selector].(*ast.CallExpr)
		if !ok || call.Fun != selector || len(call.Args) != 0 {
			return false
		}
		storage = call
	} else if field, ok := pkg.TypesInfo.ObjectOf(selector.Sel).(*types.Var); !ok || !field.IsField() || field.Pkg() != allocation.Pkg() || field.Name() != "storage" {
		return false
	}
	member, ok := parents[storage].(*ast.SelectorExpr)
	if !ok || member.X != storage {
		return false
	}
	if field, ok := pkg.TypesInfo.ObjectOf(member.Sel).(*types.Var); ok {
		if !field.IsField() || field.Pkg() != allocation.Pkg() || field.Name() != "n" {
			return false
		}
		switch parent := parents[member].(type) {
		case *ast.BinaryExpr:
			return parent.Op == token.NEQ || parent.Op == token.EQL
		case *ast.CallExpr:
			fn, ok := pkg.TypesInfo.ObjectOf(ps6131IdentSelector(parent.Fun)).(*types.Func)
			return ok && fn.Pkg() != nil && fn.Pkg().Path() == "fmt" && fn.Name() == "Sprintf"
		}
		return false
	}
	fn, ok := pkg.TypesInfo.ObjectOf(member.Sel).(*types.Func)
	if !ok || fn.Pkg() != allocation.Pkg() || fn.Name() != "F32" || !ps6131ReadonlyAccessor(pkg, fn) {
		return false
	}
	view, ok := parents[member].(*ast.CallExpr)
	if !ok || view.Fun != member || len(view.Args) != 0 {
		return false
	}
	switch parent := parents[view].(type) {
	case *ast.IndexExpr:
		return parent.X == view
	case *ast.RangeStmt:
		return parent.X == view
	case *ast.CallExpr:
		builtin, ok := pkg.TypesInfo.ObjectOf(ps6131Ident(parent.Fun)).(*types.Builtin)
		return ok && (builtin.Name() == "copy" || builtin.Name() == "len")
	}
	return false
}

func ps6131ReadonlyAccessor(pkg *packages.Package, fn *types.Func) bool {
	selected, body := ps6131FunctionSource(pkg, fn)
	sig := fn.Type().(*types.Signature)
	if body == nil || body.Body == nil || sig.Recv() == nil || sig.Params().Len() != 0 || sig.Results().Len() != 1 || len(body.Body.List) < 1 || len(body.Body.List) > 2 {
		return false
	}
	ret, ok := body.Body.List[len(body.Body.List)-1].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	result := ret.Results[0]
	if address, ok := result.(*ast.UnaryExpr); ok && address.Op == token.AND && fn.Name() == "Storage" {
		result = address.X
	}
	field, ok := result.(*ast.SelectorExpr)
	if !ok || selected.TypesInfo.ObjectOf(ps6131Ident(field.X)) != sig.Recv() {
		return false
	}
	object, ok := selected.TypesInfo.ObjectOf(field.Sel).(*types.Var)
	if !ok || !object.IsField() || object.Pkg() != fn.Pkg() {
		return false
	}
	if len(body.Body.List) == 1 {
		return fn.Name() == "Storage" && object.Name() == "storage" || fn.Name() == "F32" && object.Name() == "f32"
	}
	guard, ok := body.Body.List[0].(*ast.IfStmt)
	if !ok || fn.Name() != "F32" || object.Name() != "f32" || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 {
		return false
	}
	condition, ok := guard.Cond.(*ast.BinaryExpr)
	if !ok || condition.Op != token.EQL || selected.TypesInfo.ObjectOf(ps6131Ident(condition.Y)) != types.Universe.Lookup("nil") {
		return false
	}
	checked, ok := condition.X.(*ast.SelectorExpr)
	if !ok || selected.TypesInfo.ObjectOf(ps6131Ident(checked.X)) != sig.Recv() || selected.TypesInfo.ObjectOf(checked.Sel) != object {
		return false
	}
	statement, ok := guard.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	panicCall, ok := statement.X.(*ast.CallExpr)
	if !ok || len(panicCall.Args) != 1 {
		return false
	}
	panicBuiltin, ok := selected.TypesInfo.ObjectOf(ps6131Ident(panicCall.Fun)).(*types.Builtin)
	return ok && panicBuiltin.Name() == "panic"
}

func ps6131ConstructorDataWrite(pkg *packages.Package, expr ast.Expr, returned map[types.Object]bool, allocation *types.Func) bool {
	index, ok := expr.(*ast.IndexExpr)
	if !ok {
		return false
	}
	view, ok := index.X.(*ast.CallExpr)
	if !ok || len(view.Args) != 0 {
		return false
	}
	selector, ok := view.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "F32" {
		return false
	}
	storage, ok := selector.X.(*ast.CallExpr)
	if !ok || len(storage.Args) != 0 {
		return false
	}
	receiver, ok := storage.Fun.(*ast.SelectorExpr)
	if !ok || receiver.Sel.Name != "Storage" || !returned[pkg.TypesInfo.ObjectOf(ps6131Ident(receiver.X))] {
		return false
	}
	viewFn, ok := pkg.TypesInfo.ObjectOf(selector.Sel).(*types.Func)
	storageFn, storageOK := pkg.TypesInfo.ObjectOf(receiver.Sel).(*types.Func)
	return ok && storageOK && viewFn.Pkg() == allocation.Pkg() && storageFn.Pkg() == allocation.Pkg()
}

func ps6131NewForwards(pkg *packages.Package, fn, allocation *types.Func) bool {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			body, ok := decl.(*ast.FuncDecl)
			if !ok || pkg.TypesInfo.Defs[body.Name] != fn || body.Body == nil || len(body.Body.List) != 1 {
				continue
			}
			sig := fn.Type().(*types.Signature)
			ret, ok := body.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 || sig.Params().Len() != 2 {
				return false
			}
			call, ok := ret.Results[0].(*ast.CallExpr)
			if !ok || len(call.Args) != 3 || pkg.TypesInfo.ObjectOf(ps6131IdentSelector(call.Fun)) != allocation {
				return false
			}
			cpu, ok := call.Args[0].(*ast.CallExpr)
			if !ok || len(cpu.Args) != 0 {
				return false
			}
			cpuFn, ok := pkg.TypesInfo.ObjectOf(ps6131IdentSelector(cpu.Fun)).(*types.Func)
			return ok && cpuFn.Pkg() == allocation.Pkg() && cpuFn.Name() == "CPU" && pkg.TypesInfo.ObjectOf(ps6131Ident(call.Args[1])) == sig.Params().At(0) && pkg.TypesInfo.ObjectOf(ps6131Ident(call.Args[2])) == sig.Params().At(1)
		}
	}
	return false
}

func ps6131IdentSelector(expr ast.Expr) *ast.Ident {
	if selector, ok := expr.(*ast.SelectorExpr); ok {
		return selector.Sel
	}
	return ps6131Ident(expr)
}

func ps6131Registration(pkg *packages.Package, call *ast.CallExpr, allocation *types.Func) bool {
	if len(call.Args) != 2 {
		return false
	}
	object := pkg.TypesInfo.ObjectOf(ps6131IdentSelector(call.Fun))
	if fn, ok := object.(*types.Func); ok {
		// The portable integration fixture's direct registry is itself inspected;
		// an arbitrary sink with the same arguments is not a registration edge.
		if fn.Name() != "Register" || fn.Pkg() == nil || fn.Pkg().Path() != strings.TrimSuffix(allocation.Pkg().Path(), "/tensor")+"/backend" {
			return false
		}
		dependency, body := ps6131FunctionSource(pkg, fn)
		if body == nil || body.Body == nil || len(body.Body.List) != 1 {
			return false
		}
		sig := fn.Type().(*types.Signature)
		assign, ok := body.Body.List[0].(*ast.AssignStmt)
		if !ok || sig.Params().Len() != 2 || sig.Results().Len() != 0 || assign.Tok != token.ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return false
		}
		global, ok := dependency.TypesInfo.ObjectOf(ps6131Ident(assign.Lhs[0])).(*types.Var)
		return ok && global.Parent() == dependency.Types.Scope() && types.Identical(global.Type(), sig.Params().At(1).Type()) && dependency.TypesInfo.ObjectOf(ps6131Ident(assign.Rhs[0])) == sig.Params().At(1)
	}
	variable, ok := object.(*types.Var)
	if !ok || variable.Pkg() != pkg.Types {
		return false
	}
	var literal *ast.FuncLit
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			assign, ok := node.(*ast.AssignStmt)
			if ok && assign.Tok == token.DEFINE && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 && pkg.TypesInfo.ObjectOf(ps6131Ident(assign.Lhs[0])) == variable {
				literal, _ = assign.Rhs[0].(*ast.FuncLit)
			}
			return true
		})
	}
	if literal == nil || len(literal.Body.List) != 2 {
		return false
	}
	if !ps6131RegistrationBinding(pkg, variable, literal, call) {
		return false
	}
	sig, ok := pkg.TypesInfo.TypeOf(literal).(*types.Signature)
	if !ok || sig.Params().Len() != 2 || sig.Results().Len() != 0 {
		return false
	}
	var addFunction *types.Func
	var receiver types.Object
	for i, stmt := range literal.Body.List {
		expression, ok := stmt.(*ast.ExprStmt)
		if !ok {
			return false
		}
		add, ok := expression.X.(*ast.CallExpr)
		if !ok || len(add.Args) != 3 || pkg.TypesInfo.ObjectOf(ps6131Ident(add.Args[0])) != sig.Params().At(0) || pkg.TypesInfo.ObjectOf(ps6131Ident(add.Args[2])) != sig.Params().At(1) {
			return false
		}
		selector, ok := add.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		fn, ok := pkg.TypesInfo.ObjectOf(selector.Sel).(*types.Func)
		if !ok || fn.Pkg() != pkg.Types || !ps6131RegistryAdd(pkg, fn) {
			return false
		}
		if i == 0 {
			addFunction, receiver = fn, pkg.TypesInfo.ObjectOf(ps6131Ident(selector.X))
		} else if fn != addFunction || pkg.TypesInfo.ObjectOf(ps6131Ident(selector.X)) != receiver {
			return false
		}
		dtype, ok := pkg.TypesInfo.ObjectOf(ps6131IdentSelector(add.Args[1])).(*types.Const)
		want := "F32"
		if i == 1 {
			want = "F64"
		}
		if !ok || dtype.Pkg() != allocation.Pkg() || dtype.Name() != want {
			return false
		}
	}
	return receiver != nil
}

func ps6131RegistrationBinding(pkg *packages.Package, variable *types.Var, literal *ast.FuncLit, call *ast.CallExpr) bool {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			init, ok := decl.(*ast.FuncDecl)
			if !ok || init.Name.Name != "init" || init.Body == nil {
				continue
			}
			definition, invocation := -1, -1
			var defined *ast.Ident
			for i, stmt := range init.Body.List {
				if assign, ok := stmt.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 && assign.Rhs[0] == literal {
					defined = ps6131Ident(assign.Lhs[0])
					if pkg.TypesInfo.Defs[defined] == variable {
						definition = i
					}
				}
				if expression, ok := stmt.(*ast.ExprStmt); ok && expression.X == call {
					invocation = i
				}
			}
			if definition < 0 || invocation <= definition {
				continue
			}
			parents := ps6071Parents(init.Body)
			valid := true
			ast.Inspect(init.Body, func(node ast.Node) bool {
				id, ok := node.(*ast.Ident)
				if !ok || pkg.TypesInfo.ObjectOf(id) != variable || id == defined {
					return true
				}
				use, ok := parents[id].(*ast.CallExpr)
				if !ok || use.Fun != id {
					valid = false
					return true
				}
				for ancestor := parents[use]; ancestor != nil; ancestor = parents[ancestor] {
					switch ancestor.(type) {
					case *ast.GoStmt, *ast.DeferStmt, *ast.FuncLit:
						valid = false
					}
				}
				return true
			})
			return valid
		}
	}
	return false
}

func ps6131RegistryAdd(pkg *packages.Package, fn *types.Func) bool {
	_, body := ps6131FunctionSource(pkg, fn)
	sig := fn.Type().(*types.Signature)
	if body == nil || body.Body == nil || len(body.Body.List) != 1 || sig.Recv() == nil || sig.Params().Len() != 3 || sig.Results().Len() != 0 {
		return false
	}
	assign, ok := body.Body.List[0].(*ast.AssignStmt)
	if !ok || assign.Tok != token.ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 || pkg.TypesInfo.ObjectOf(ps6131Ident(assign.Rhs[0])) != sig.Params().At(2) {
		return false
	}
	index, ok := assign.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return false
	}
	field, ok := index.X.(*ast.SelectorExpr)
	if !ok || pkg.TypesInfo.ObjectOf(ps6131Ident(field.X)) != sig.Recv() {
		return false
	}
	key, ok := index.Index.(*ast.CompositeLit)
	if !ok || len(key.Elts) != 2 {
		return false
	}
	if pkg.TypesInfo.ObjectOf(ps6131Ident(key.Elts[0])) != sig.Params().At(0) {
		return false
	}
	if pkg.TypesInfo.ObjectOf(ps6131Ident(key.Elts[1])) != sig.Params().At(1) {
		return false
	}
	mapping, ok := types.Unalias(pkg.TypesInfo.TypeOf(index.X)).(*types.Map)
	return ok && types.Identical(mapping.Elem(), sig.Params().At(2).Type()) && types.Identical(mapping.Key(), pkg.TypesInfo.TypeOf(key))
}

func ps6131FunctionSource(root *packages.Package, fn *types.Func) (*packages.Package, *ast.FuncDecl) {
	visited := make(map[*packages.Package]bool)
	var search func(*packages.Package) (*packages.Package, *ast.FuncDecl)
	search = func(pkg *packages.Package) (*packages.Package, *ast.FuncDecl) {
		if visited[pkg] {
			return nil, nil
		}
		visited[pkg] = true
		if pkg.Types == fn.Pkg() {
			for _, file := range pkg.Syntax {
				for _, decl := range file.Decls {
					if body, ok := decl.(*ast.FuncDecl); ok && pkg.TypesInfo.Defs[body.Name] == fn {
						return pkg, body
					}
				}
			}
		}
		for _, dependency := range pkg.Imports {
			if selected, body := search(dependency); body != nil {
				return selected, body
			}
		}
		return nil, nil
	}
	return search(root)
}

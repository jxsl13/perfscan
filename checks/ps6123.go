package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math/big"
	"strconv"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6123 implements owner issue #966.
var PS6123 = register(&lint.Check{
	ID: "PS6123", Category: "verify", Slug: "eager-expensive-simd-select", Level: lint.LevelAggressive,
	AutoFix: false, NeedsConfig: true, Vocab: []string{"pureComputeFuncs"},
	Doc: lint.Documentation{
		Title: "a typed SIMD select eagerly computes substantially more expensive work for one arm",
		Text: `Typed SIMD IfElse/select operations evaluate both arms before selecting lanes. PS6123 follows straight-line local value versions (including ordinary rebinding and simultaneous-assignment RHS snapshots), counts configured pure compute calls, vector division, and long MulAdd chains, and reports an arm-exclusive expensive DAG only when the containing helper has a reachable direct call in a potentially repeated loop. Multiple and nested selects are assessed independently. The typed scope is Float64x2 with its two-lane Mask64x2 guard; a dynamic stride-two caller proves potential, not guaranteed, repetition.

The finding is an L3 measurement candidate, not proof that machine code retains the work or that a branch wins. A dominating all-lanes mask return can avoid the eager region, but Any/one-lane/unrelated-mask guards are insufficient. There is NO automatic fix. Preserve distinct rounded floating-point inputs, signed zero, finite bits, NaN/Inf classes, mixed-lane fallback, scalar tails, input immutability, and operation results. Benchmark the leaf and complete operation on homogeneous-small, homogeneous-large, and mixed inputs in repeated alternating campaigns; inspect branches, spills, code size, and allocations.

Both owner variants remain rejected. V1 improved large original-control cells 1.257939–2.795224x but regressed every small n2048 active-forward Execute cell by 68.95–76.65%. V2's original-control results (large 2.058–3.924x, small 1.497–1.672x) are not a V1-to-V2 marginal gain and failed the predeclared repeated-allocation byte veto. Do not waive controls, discard samples, or infer profitability from this advisory.`,
		Before: `large := expensivePure(x)
middle := small.IfElse(mask, large)
return saturated.IfElse(outerMask, middle)`,
		After: `lanes := mask.ToInt64x2()
if lanes.GetElem(0) != 0 && lanes.GetElem(1) != 0 { return small }
large := expensivePure(x)
middle := small.IfElse(mask, large)
return saturated.IfElse(outerMask, middle)`,
		MeasuredWin: `No measured win is claimed. Owner V1 and V2 were both REJECTED; V2 numbers compare original control to V2, not V1 to V2.`,
	},
	Analyzer: &analysis.Analyzer{Name: "PS6123", Doc: "eager expensive typed SIMD select arms", Run: runPS6123},
})

type ps6123Node struct {
	deps []*ps6123Node
	own  int
	pos  token.Pos
	kind string
}

type ps6123Guard struct {
	mask, selected *ps6123Node
	pos            token.Pos
}

type ps6123Candidate struct {
	call              *ast.CallExpr
	left, right, mask *ps6123Node
}

type ps6123State struct {
	pass       *analysis.Pass
	pure       map[string]bool
	values     map[types.Object]*ps6123Node
	guards     []ps6123Guard
	candidates []ps6123Candidate
	valid      bool
	opaque     bool
}

func runPS6123(pass *analysis.Pass) (any, error) {
	return runPS6123WithPure(pass, config.Current().PureComputeFuncs)
}

func runPS6123WithPure(pass *analysis.Pass, pure map[string]bool) (any, error) {
	if len(pure) == 0 {
		return nil, nil
	}
	parents := make(map[*ast.File]map[ast.Node]ast.Node, len(pass.Files))
	for _, file := range pass.Files {
		parents[file] = ps6071Parents(file)
	}
	hot := ps6123HotHelpers(pass, parents)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !hot[pass.TypesInfo.ObjectOf(fn.Name)] {
				continue
			}
			ps6123Inspect(pass, fn, pure)
		}
	}
	return nil, nil
}

func ps6123Inspect(pass *analysis.Pass, fn *ast.FuncDecl, pure map[string]bool) {
	state := &ps6123State{pass: pass, pure: pure, values: map[types.Object]*ps6123Node{}, valid: true}
	for _, statement := range fn.Body.List {
		switch value := statement.(type) {
		case *ast.AssignStmt:
			state.assign(value)
		case *ast.DeclStmt:
			general, ok := value.Decl.(*ast.GenDecl)
			if !ok {
				state.valid = false
				continue
			}
			for _, spec := range general.Specs {
				item, ok := spec.(*ast.ValueSpec)
				if !ok || len(item.Names) != len(item.Values) {
					state.valid = false
					continue
				}
				snapshot := make([]*ps6123Node, len(item.Values))
				for i, rhs := range item.Values {
					snapshot[i] = state.eval(rhs)
				}
				for i, name := range item.Names {
					state.values[pass.TypesInfo.ObjectOf(name)] = snapshot[i]
				}
			}
		case *ast.IfStmt:
			if value.Init != nil {
				assignment, ok := value.Init.(*ast.AssignStmt)
				if !ok {
					state.valid = false
					continue
				}
				state.assign(assignment)
			}
			guard, ok := state.guard(value)
			if ok {
				state.guards = append(state.guards, guard)
			} else if value.Else != nil || len(value.Body.List) != 1 {
				state.valid = false
			} else if _, returns := value.Body.List[0].(*ast.ReturnStmt); !returns {
				state.valid = false
			} else {
				state.eval(value.Cond)
			}
		case *ast.ReturnStmt:
			for _, expression := range value.Results {
				state.eval(expression)
			}
		case *ast.EmptyStmt:
		default:
			state.valid = false
		}
	}
	if !state.valid {
		return
	}
	flow := ps6122NewFlow(pass, fn.Body)
	for _, candidate := range state.candidates {
		if !ps6122CanEnter(pass, fn.Body, flow.parents, candidate.call.Pos()) || ps6122BlockAt(pass, flow, candidate.call.Pos()) == nil || !ps6122PositionCanReturn(pass, flow, candidate.call.Pos()) {
			continue
		}
		if !ps6122ExpressionExecuted(pass, flow.parents, candidate.call) || !ps6123PureTree(candidate.left) || !ps6123PureTree(candidate.right) || !ps6123PureTree(candidate.mask) || state.opaque && (ps6123HasKind(candidate.left, "global") || ps6123HasKind(candidate.right, "global") || ps6123HasKind(candidate.mask, "global")) {
			continue
		}
		left, right := ps6123Nodes(candidate.left), ps6123Nodes(candidate.right)
		leftCost, rightCost := 0, 0
		leftEarliest, rightEarliest := token.NoPos, token.NoPos
		for node := range left {
			if !right[node] && node.own > 0 {
				leftCost += node.own
				if leftEarliest == token.NoPos || node.pos < leftEarliest {
					leftEarliest = node.pos
				}
			}
		}
		for node := range right {
			if !left[node] && node.own > 0 {
				rightCost += node.own
				if rightEarliest == token.NoPos || node.pos < rightEarliest {
					rightEarliest = node.pos
				}
			}
		}
		cost, earliest := leftCost-rightCost, leftEarliest
		if cost < 0 {
			cost, earliest = -cost, rightEarliest
		}
		if cost < 6 || state.suppressed(candidate, earliest) {
			continue
		}
		pass.Report(analysis.Diagnostic{Pos: candidate.call.Pos(), End: candidate.call.End(), Message: "typed SIMD select eagerly evaluates an arm-exclusive expensive pure DAG that a uniform region may avoid; measure an all-lanes fast return and retain the mixed-lane fallback (PS6123 advisory, no automatic fix)"})
	}
}

func (state *ps6123State) assign(statement *ast.AssignStmt) {
	if len(statement.Lhs) != len(statement.Rhs) || statement.Tok != token.DEFINE && statement.Tok != token.ASSIGN {
		state.valid = false
		return
	}
	snapshot := make([]*ps6123Node, len(statement.Rhs))
	for i, rhs := range statement.Rhs {
		snapshot[i] = state.eval(rhs)
	}
	for i, lhs := range statement.Lhs {
		id, ok := ps2110Unparen(lhs).(*ast.Ident)
		if !ok {
			state.valid = false
			continue
		}
		if id.Name == "_" {
			continue
		}
		object := state.pass.TypesInfo.ObjectOf(id)
		if variable, ok := object.(*types.Var); !ok || variable.Parent() == state.pass.Pkg.Scope() {
			state.valid = false
			continue
		}
		state.values[object] = snapshot[i]
	}
}

func (state *ps6123State) eval(expression ast.Expr) *ps6123Node {
	expression = ps2110Unparen(expression)
	if id, ok := expression.(*ast.Ident); ok {
		object := state.pass.TypesInfo.ObjectOf(id)
		if object == nil {
			state.valid = false
			return nil
		}
		return state.valueOf(object, id.Pos())
	}
	node := &ps6123Node{pos: expression.Pos()}
	switch value := expression.(type) {
	case *ast.CallExpr:
		if selector, ok := value.Fun.(*ast.SelectorExpr); ok {
			node.deps = append(node.deps, state.eval(selector.X))
		}
		for _, arg := range value.Args {
			node.deps = append(node.deps, state.eval(arg))
		}
		if ps6123ConfiguredPure(state.pass, value, state.pure) {
			node.own, node.kind = 12, "pure"
		}
		if selector, ok := value.Fun.(*ast.SelectorExpr); ok && ps6123Vector(state.pass.TypesInfo.TypeOf(selector.X)) {
			if selector.Sel.Name == "Div" && ps6123VectorMethod(state.pass, value, 1) {
				node.own = 4
			}
			if selector.Sel.Name == "MulAdd" && ps6123VectorMethod(state.pass, value, 2) {
				node.own = 1
			}
		}
		if ps6123CanonicalMethod(state.pass, value, "ToInt64x2", "Mask64x2", "Int64x2", 0) {
			node.kind = "maskToLanes"
		}
		if _, method := value.Fun.(*ast.SelectorExpr); !method && node.own == 0 && ps6123Vector(state.pass.TypesInfo.TypeOf(value)) {
			state.valid = false
		}
		if ps6123Select(state.pass, value) && len(node.deps) == 3 {
			state.candidates = append(state.candidates, ps6123Candidate{call: value, left: node.deps[0], mask: node.deps[1], right: node.deps[2]})
		}
		if !ps6123ApprovedCall(state.pass, value, state.pure) {
			node.kind = "impure"
			state.opaque = true
		}
	case *ast.BinaryExpr:
		node.deps = []*ps6123Node{state.eval(value.X), state.eval(value.Y)}
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			state.valid = false
		}
		node.deps = []*ps6123Node{state.eval(value.X)}
	case *ast.FuncLit:
		state.valid = false
	case *ast.SelectorExpr:
		node.deps = []*ps6123Node{state.eval(value.X)}
	case *ast.IndexExpr:
		node.deps = []*ps6123Node{state.eval(value.X), state.eval(value.Index)}
	}
	return node
}

func (state *ps6123State) valueOf(object types.Object, pos token.Pos) *ps6123Node {
	if object == nil {
		state.valid = false
		return nil
	}
	if state.values[object] == nil {
		kind := "input"
		if variable, ok := object.(*types.Var); ok && variable.Parent() == state.pass.Pkg.Scope() {
			kind = "global"
		}
		state.values[object] = &ps6123Node{pos: pos, kind: kind}
	}
	return state.values[object]
}

func ps6123Nodes(root *ps6123Node) map[*ps6123Node]bool {
	result := map[*ps6123Node]bool{}
	var visit func(*ps6123Node)
	visit = func(node *ps6123Node) {
		if node == nil || result[node] {
			return
		}
		result[node] = true
		for _, dep := range node.deps {
			visit(dep)
		}
	}
	visit(root)
	return result
}

func ps6123PureTree(root *ps6123Node) bool {
	for node := range ps6123Nodes(root) {
		if node.kind == "impure" {
			return false
		}
	}
	return true
}

func ps6123HasKind(root *ps6123Node, kind string) bool {
	for node := range ps6123Nodes(root) {
		if node.kind == kind {
			return true
		}
	}
	return false
}

func ps6123Named(value types.Type, name string) bool {
	named, ok := types.Unalias(value).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "simd/archsimd" && named.Obj().Name() == name
}

func ps6123Vector(value types.Type) bool {
	return ps6123Named(value, "Float64x2") || ps6123Named(value, "Float32x4")
}

func ps6123ConfiguredPure(pass *analysis.Pass, call *ast.CallExpr, pure map[string]bool) bool {
	id, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	function, ok := pass.TypesInfo.ObjectOf(id).(*types.Func)
	if !ok || function.Pkg() == nil || !pure[function.Pkg().Path()+"."+function.Name()] {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	return signature != nil && !signature.Variadic() && signature.TypeParams().Len() == 0 && signature.Params().Len() == 1 && signature.Results().Len() == 1 && ps6123Vector(signature.Params().At(0).Type()) && types.Identical(signature.Params().At(0).Type(), signature.Results().At(0).Type())
}

func ps6123ApprovedCall(pass *analysis.Pass, call *ast.CallExpr, pure map[string]bool) bool {
	if ps6123ConfiguredPure(pass, call, pure) || ps6123Select(pass, call) {
		return true
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	receiverName := "Float64x2"
	switch selector.Sel.Name {
	case "And", "Or", "BitsToFloat64":
		receiverName = "Uint64x2"
	case "ToInt64x2":
		receiverName = "Mask64x2"
	case "GetElem":
		receiverName = "Int64x2"
	}
	signature, ok := ps6123MethodRole(pass, call, selector.Sel.Name, receiverName, len(call.Args))
	if !ok {
		return false
	}
	receiver := signature.Recv().Type()
	result := signature.Results().At(0).Type()
	allParams := func(value types.Type) bool {
		for i := 0; i < signature.Params().Len(); i++ {
			if !types.Identical(signature.Params().At(i).Type(), value) {
				return false
			}
		}
		return true
	}
	switch selector.Sel.Name {
	case "Abs", "Round":
		return len(call.Args) == 0 && types.Identical(receiver, result)
	case "Add", "Div", "Max", "Mul", "Sub":
		return len(call.Args) == 1 && allParams(receiver) && types.Identical(receiver, result)
	case "MulAdd":
		return len(call.Args) == 2 && allParams(receiver) && types.Identical(receiver, result)
	case "Less", "GreaterEqual":
		return len(call.Args) == 1 && allParams(receiver) && ps6123Named(result, "Mask64x2")
	case "ToBits":
		return len(call.Args) == 0 && ps6123Named(result, "Uint64x2")
	case "And", "Or":
		return len(call.Args) == 1 && allParams(receiver) && types.Identical(receiver, result)
	case "BitsToFloat64":
		return len(call.Args) == 0 && ps6123Named(result, "Float64x2")
	case "ToInt64x2":
		return ps6123CanonicalMethod(pass, call, "ToInt64x2", "Mask64x2", "Int64x2", 0)
	case "GetElem":
		return ps6123CanonicalMethod(pass, call, "GetElem", "Int64x2", "", 1)
	}
	return false
}

func ps6123MethodRole(pass *analysis.Pass, call *ast.CallExpr, method, receiver string, parameters int) (*types.Signature, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != method || len(call.Args) != parameters {
		return nil, false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.MethodVal {
		return nil, false
	}
	function, ok := selection.Obj().(*types.Func)
	if !ok || function.Pkg() == nil || function.Pkg().Path() != "simd/archsimd" {
		return nil, false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil || signature.Variadic() || signature.Params().Len() != parameters || signature.Results().Len() != 1 || !ps6123Named(signature.Recv().Type(), receiver) {
		return nil, false
	}
	return signature, true
}

func ps6123VectorMethod(pass *analysis.Pass, call *ast.CallExpr, parameters int) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	signature, ok := ps6123MethodRole(pass, call, selector.Sel.Name, "Float64x2", parameters)
	if !ok {
		return false
	}
	receiver := signature.Recv().Type()
	if !types.Identical(receiver, signature.Results().At(0).Type()) {
		return false
	}
	for i := 0; i < parameters; i++ {
		if !types.Identical(receiver, signature.Params().At(i).Type()) {
			return false
		}
	}
	return true
}

func ps6123CanonicalMethod(pass *analysis.Pass, call *ast.CallExpr, method, receiver, result string, parameters int) bool {
	signature, ok := ps6123MethodRole(pass, call, method, receiver, parameters)
	if !ok {
		return false
	}
	if method == "GetElem" {
		parameter, resultType := signature.Params().At(0).Type().Underlying(), signature.Results().At(0).Type().Underlying()
		p, pok := parameter.(*types.Basic)
		r, rok := resultType.(*types.Basic)
		return pok && rok && p.Kind() == types.Uint8 && r.Kind() == types.Int64
	}
	return result != "" && ps6123Named(signature.Results().At(0).Type(), result)
}

func (state *ps6123State) guard(statement *ast.IfStmt) (ps6123Guard, bool) {
	if statement.Init != nil || statement.Else != nil || len(statement.Body.List) != 1 {
		return ps6123Guard{}, false
	}
	returned, ok := statement.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return ps6123Guard{}, false
	}
	selectedID, ok := ps2110Unparen(returned.Results[0]).(*ast.Ident)
	if !ok {
		return ps6123Guard{}, false
	}
	and, ok := ps2110Unparen(statement.Cond).(*ast.BinaryExpr)
	if !ok || and.Op != token.LAND {
		return ps6123Guard{}, false
	}
	left, leftLane, ok := state.lane(and.X)
	if !ok {
		return ps6123Guard{}, false
	}
	right, rightLane, ok := state.lane(and.Y)
	if !ok || left != right || leftLane == rightLane || leftLane > 1 || rightLane > 1 {
		return ps6123Guard{}, false
	}
	if left == nil || left.kind != "maskToLanes" || len(left.deps) != 1 {
		return ps6123Guard{}, false
	}
	return ps6123Guard{mask: left.deps[0], selected: state.valueOf(state.pass.TypesInfo.ObjectOf(selectedID), selectedID.Pos()), pos: statement.Pos()}, true
}

func (state *ps6123State) lane(expression ast.Expr) (*ps6123Node, int, bool) {
	comparison, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || comparison.Op != token.NEQ || state.pass.TypesInfo.Types[comparison.Y].Value == nil || state.pass.TypesInfo.Types[comparison.Y].Value.String() != "0" {
		return nil, 0, false
	}
	call, ok := ps2110Unparen(comparison.X).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil, 0, false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !ps6123CanonicalMethod(state.pass, call, "GetElem", "Int64x2", "", 1) {
		return nil, 0, false
	}
	id, ok := ps2110Unparen(selector.X).(*ast.Ident)
	if !ok {
		return nil, 0, false
	}
	index := state.pass.TypesInfo.Types[call.Args[0]].Value
	if index == nil {
		return nil, 0, false
	}
	lane, err := strconv.Atoi(index.String())
	return state.values[state.pass.TypesInfo.ObjectOf(id)], lane, err == nil
}

func (state *ps6123State) suppressed(candidate ps6123Candidate, earliest token.Pos) bool {
	for _, guard := range state.guards {
		if guard.mask == candidate.mask && guard.selected == candidate.left && guard.pos < earliest {
			return true
		}
	}
	return false
}

func ps6123Select(pass *analysis.Pass, call *ast.CallExpr) bool {
	signature, ok := ps6123MethodRole(pass, call, "IfElse", "Float64x2", 2)
	return ok && ps6123Named(signature.Params().At(0).Type(), "Mask64x2") && types.Identical(signature.Recv().Type(), signature.Results().At(0).Type()) && types.Identical(signature.Recv().Type(), signature.Params().At(1).Type())
}

func ps6123HotHelpers(pass *analysis.Pass, parents map[*ast.File]map[ast.Node]ast.Node) map[types.Object]bool {
	hot := map[types.Object]bool{}
	flows := map[*ast.BlockStmt]*ps6122Flow{}
	executions := map[*ast.BlockStmt]*ps6121Execution{}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			caller, ok := declaration.(*ast.FuncDecl)
			if !ok || caller.Body == nil {
				continue
			}
			flow := flows[caller.Body]
			if flow == nil {
				flow = ps6122NewFlow(pass, caller.Body)
				flows[caller.Body] = flow
			}
			reachable := ps6099ReachableCalls(pass, caller, parents[file])
			ps6122WalkExecuted(caller.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				id, ok := ps2110Unparen(call.Fun).(*ast.Ident)
				if !ok {
					return true
				}
				object, ok := pass.TypesInfo.ObjectOf(id).(*types.Func)
				if !ok || object.Pkg() != pass.Pkg || hot[object] {
					return true
				}
				if ps6123RepeatedCall(pass, caller.Body, flow, parents[file], reachable, call, flows, executions) {
					hot[object] = true
				}
				return true
			})
		}
	}
	return hot
}

func ps6123RepeatedCall(pass *analysis.Pass, outer *ast.BlockStmt, outerFlow *ps6122Flow, parents map[ast.Node]ast.Node, reachable map[*ast.CallExpr]bool, call *ast.CallExpr, flows map[*ast.BlockStmt]*ps6122Flow, executions map[*ast.BlockStmt]*ps6121Execution) bool {
	position := ast.Node(call)
	body, flow := outer, outerFlow
	if literal := ps6123ImmediateLiteral(position, parents); literal != nil {
		body = literal.Body
		flow = flows[body]
		if flow == nil {
			flow = ps6122NewFlow(pass, body)
			flows[body] = flow
		}
	}
	repeated := false
	for {
		projected, ok := position.(*ast.CallExpr)
		if !ok {
			return false
		}
		localRepeat, viable := ps6123RepeatedSite(pass, body, flow, parents, projected, executions)
		if !viable {
			return false
		}
		repeated = repeated || localRepeat
		literal := ps6123ImmediateLiteral(position, parents)
		if literal == nil {
			return reachable[projected] && repeated
		}
		if !ps6122CanEnter(pass, literal.Body, flow.parents, position.Pos()) || (!repeated && !ps6122PositionCanReturn(pass, flow, position.Pos())) {
			return false
		}
		invocation, ok := parents[literal].(*ast.CallExpr)
		if !ok || ps2110Unparen(invocation.Fun) != literal {
			return false
		}
		position = invocation
		body, flow = outer, outerFlow
		if enclosing := ps6123ImmediateLiteral(position, parents); enclosing != nil {
			body = enclosing.Body
			flow = flows[body]
			if flow == nil {
				flow = ps6122NewFlow(pass, body)
				flows[body] = flow
			}
		}
	}
}

func ps6123ImmediateLiteral(node ast.Node, parents map[ast.Node]ast.Node) *ast.FuncLit {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		if literal, ok := parent.(*ast.FuncLit); ok {
			return literal
		}
	}
	return nil
}

func ps6123RepeatedSite(pass *analysis.Pass, body *ast.BlockStmt, flow *ps6122Flow, parents map[ast.Node]ast.Node, call *ast.CallExpr, executions map[*ast.BlockStmt]*ps6121Execution) (bool, bool) {
	if ps6122BlockAt(pass, flow, call.Pos()) == nil {
		return false, false
	}
	repeated := false
	execution := ps6121ExecutionFor(pass, body, executions)
	for parent := parents[call]; parent != nil && parent != body; parent = parents[parent] {
		switch loop := parent.(type) {
		case *ast.GoStmt, *ast.DeferStmt:
			return false, false
		case *ast.ForStmt:
			if !ps6123StableFor(pass, loop) {
				return false, false
			}
			class := ps6123ForTripClass(pass, loop)
			if class == ps6123TripUnsupported || class == ps6123TripZero {
				return false, false
			}
			if class >= ps6123TripMany && loop.Post != nil && execution.CycleThrough(call, loop.Post) {
				repeated = true
			}
		case *ast.RangeStmt:
			class := ps6122RangeTrips(pass, loop)
			if class <= 0 {
				return false, false
			}
			if (class < 0 || class >= 2) && ps6123RangeRepeats(pass, flow, call, loop) {
				repeated = true
			}
		}
	}
	return repeated, true
}

func ps6123RangeRepeats(pass *analysis.Pass, flow *ps6122Flow, call *ast.CallExpr, loop *ast.RangeStmt) bool {
	start := ps6122BlockAt(pass, flow, call.Pos())
	if start == nil || ps6122BlockHasBlockingTransfer(pass, flow, start, call.Pos(), token.NoPos) {
		return false
	}
	var backedge *cfg.Block
	for _, block := range flow.graph.Blocks {
		if block.Kind == cfg.KindRangeLoop && block.Stmt == loop {
			backedge = block
			break
		}
	}
	return backedge != nil && ps6123FlowPath(pass, flow, start, backedge) && ps6123FlowPath(pass, flow, backedge, start)
}

func ps6123FlowPath(pass *analysis.Pass, flow *ps6122Flow, start, target *cfg.Block) bool {
	if start == nil || target == nil || !flow.reachable[start] || !flow.reachable[target] {
		return false
	}
	seen := map[*cfg.Block]bool{}
	pending := []*cfg.Block{start}
	for len(pending) != 0 {
		block := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if block == target {
			return true
		}
		if block == nil || seen[block] || !flow.reachable[block] {
			continue
		}
		seen[block] = true
		pending = append(pending, ps6122Successors(pass, flow, block, token.NoPos)...)
	}
	return false
}

func ps6123StableFor(pass *analysis.Pass, loop *ast.ForStmt) bool {
	init, ok := loop.Init.(*ast.AssignStmt)
	if !ok || len(init.Lhs) != 1 {
		return false
	}
	indexObject := ps6114ExprObject(pass, init.Lhs[0])
	objects := map[types.Object]ast.Expr{}
	objects[indexObject] = init.Lhs[0]
	ast.Inspect(loop.Cond, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if ok {
			if object := pass.TypesInfo.ObjectOf(id); object != nil {
				objects[object] = id
			}
		}
		return true
	})
	for object, expression := range objects {
		if object == nil || !ps6122StableLoop(pass, loop.Body, expression) {
			return false
		}
		if object != indexObject && loop.Post != nil && !ps6122StableLoop(pass, &ast.BlockStmt{List: []ast.Stmt{loop.Post}}, expression) {
			return false
		}
	}
	return true
}

const (
	ps6123TripUnsupported = iota
	ps6123TripZero
	ps6123TripOne
	ps6123TripMany
	ps6123TripPotentialMany
)

func ps6123ForTripClass(pass *analysis.Pass, loop *ast.ForStmt) int {
	init, ok := loop.Init.(*ast.AssignStmt)
	condition, conditionOK := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
	if !ok || !conditionOK || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return ps6123TripUnsupported
	}
	index, ok := ps2110Unparen(init.Lhs[0]).(*ast.Ident)
	left, leftOK := ps2110Unparen(condition.X).(*ast.Ident)
	if !ok || !leftOK || pass.TypesInfo.ObjectOf(index) != pass.TypesInfo.ObjectOf(left) {
		return ps6123TripUnsupported
	}
	startValue := pass.TypesInfo.Types[init.Rhs[0]].Value
	if startValue == nil || startValue.Kind() != constant.Int {
		return ps6123TripUnsupported
	}
	start, ok := ps6123TypedInteger(pass, pass.TypesInfo.TypeOf(index), startValue)
	if !ok {
		return ps6123TripUnsupported
	}
	next := func(value *big.Int) (*big.Int, bool) { return new(big.Int).Set(value), true }
	switch post := loop.Post.(type) {
	case *ast.IncDecStmt:
		if ps6114ExprObject(pass, post.X) != pass.TypesInfo.ObjectOf(index) {
			break
		}
		step := big.NewInt(1)
		if post.Tok == token.INC {
		} else if post.Tok == token.DEC {
			step.Neg(step)
		} else {
			return ps6123TripUnsupported
		}
		next = func(value *big.Int) (*big.Int, bool) {
			return ps6123TypedBig(pass, pass.TypesInfo.TypeOf(index), new(big.Int).Add(value, step))
		}
	case *ast.AssignStmt:
		if len(post.Lhs) != 1 || len(post.Rhs) != 1 {
			return ps6123TripUnsupported
		}
		if ps6114ExprObject(pass, post.Lhs[0]) != pass.TypesInfo.ObjectOf(index) {
			break
		}
		amountValue := pass.TypesInfo.Types[post.Rhs[0]].Value
		amount, exact := ps6123ConstantBig(amountValue)
		if !exact {
			return ps6123TripUnsupported
		}
		switch post.Tok {
		case token.ASSIGN:
			next = func(*big.Int) (*big.Int, bool) { return ps6123TypedBig(pass, pass.TypesInfo.TypeOf(index), amount) }
		case token.ADD_ASSIGN, token.SUB_ASSIGN:
			if post.Tok == token.SUB_ASSIGN {
				amount = new(big.Int).Neg(amount)
			}
			next = func(value *big.Int) (*big.Int, bool) {
				return ps6123TypedBig(pass, pass.TypesInfo.TypeOf(index), new(big.Int).Add(value, amount))
			}
		default:
			return ps6123TripUnsupported
		}
	case nil:
	default:
		return ps6123TripUnsupported
	}
	endValue := pass.TypesInfo.Types[condition.Y].Value
	if endValue == nil {
		return ps6123TripPotentialMany
	}
	end, ok := ps6123TypedInteger(pass, pass.TypesInfo.TypeOf(index), endValue)
	if !ok {
		return ps6123TripUnsupported
	}
	count := 0
	value := start
	for count < 2 && ps6123CompareBig(value, condition.Op, end) {
		count++
		value, ok = next(value)
		if !ok {
			return ps6123TripUnsupported
		}
	}
	if count == 0 {
		return ps6123TripZero
	}
	if count == 1 {
		return ps6123TripOne
	}
	return ps6123TripMany
}

func ps6123ConstantBig(value constant.Value) (*big.Int, bool) {
	if value == nil || value.Kind() != constant.Int {
		return nil, false
	}
	result, ok := new(big.Int).SetString(value.ExactString(), 10)
	return result, ok
}

func ps6123TypedInteger(pass *analysis.Pass, valueType types.Type, value constant.Value) (*big.Int, bool) {
	integer, ok := ps6123ConstantBig(value)
	if !ok {
		return nil, false
	}
	return ps6123TypedBig(pass, valueType, integer)
}

func ps6123TypedBig(pass *analysis.Pass, valueType types.Type, value *big.Int) (*big.Int, bool) {
	basic, _ := types.Unalias(valueType).Underlying().(*types.Basic)
	if basic == nil || pass.TypesSizes == nil {
		return nil, false
	}
	bits := int(pass.TypesSizes.Sizeof(basic) * 8)
	signed := false
	switch basic.Kind() {
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
		signed = true
	case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
	default:
		return nil, false
	}
	modulus := new(big.Int).Lsh(big.NewInt(1), uint(bits))
	result := new(big.Int).Mod(new(big.Int).Set(value), modulus)
	if signed {
		half := new(big.Int).Rsh(new(big.Int).Set(modulus), 1)
		if result.Cmp(half) >= 0 {
			result.Sub(result, modulus)
		}
	}
	return result, true
}

func ps6123CompareBig(left *big.Int, op token.Token, right *big.Int) bool {
	comparison := left.Cmp(right)
	switch op {
	case token.LSS:
		return comparison < 0
	case token.LEQ:
		return comparison <= 0
	case token.GTR:
		return comparison > 0
	case token.GEQ:
		return comparison >= 0
	case token.EQL:
		return comparison == 0
	case token.NEQ:
		return comparison != 0
	}
	return false
}

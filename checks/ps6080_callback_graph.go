package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"
)

// The graph records source nodes, not invocation paths. A callback parameter is
// one node, a forwarding call is one edge, and a source callback invocation is
// one terminal. Recursion and diamond-shaped forwarding share those objects.
// Reachability is MAY information; it deliberately does not assert invocation
// count, order, or that a callback executes on every path.
type ps6080CallbackGraph struct {
	nodes map[ps6080CallbackNodeKey]*ps6080CallbackNode
}

type ps6080CallbackNodeKey struct {
	function  *types.Func
	parameter int
}

type ps6080CallbackNode struct {
	key       ps6080CallbackNodeKey
	direct    []*ps6080MayCallbackSite
	edges     []ps6080CallbackEdge
	reachable map[*ps6080MayCallbackSite]bool
	unknown   bool
	returned  bool
	opaque    bool
}

type ps6080CallbackEdge struct {
	target         *ps6080CallbackNode
	forward        ps6080MayCallbackForward
	argumentOffset int
}

// This finite source graph is the replacement representation for the legacy
// path enumerators. Consumers must separately handle MAY effects and optional
// definite-order provenance before switching to it.
func ps6080BuildCallbackGraph(pass *analysis.Pass) *ps6080CallbackGraph {
	result := &ps6080CallbackGraph{nodes: make(map[ps6080CallbackNodeKey]*ps6080CallbackNode)}
	type wrapperDependency struct {
		target   *ps6080CallbackNode
		literal  *ast.FuncLit
		call     *ast.CallExpr
		function *ps6080Function
		origins  *ps6080CallbackOrigins
		parents  map[ast.Node]ast.Node
	}
	var wrappers []wrapperDependency
	sites := make(map[*ast.CallExpr]*ps6080MayCallbackSite)
	directSites := make(map[*ps6080CallbackNode]map[*ps6080MayCallbackSite]bool)
	type callbackEdgeKey struct {
		current *ps6080CallbackNode
		target  *ps6080CallbackNode
		call    *ast.CallExpr
		offset  int
	}
	edgeKeys := make(map[callbackEdgeKey]bool)
	addEdge := func(current, target *ps6080CallbackNode, call *ast.CallExpr, function *ps6080Function, parents map[ast.Node]ast.Node, signature *types.Signature, offset int) {
		key := callbackEdgeKey{current: current, target: target, call: call, offset: offset}
		if edgeKeys[key] {
			return
		}
		edgeKeys[key] = true
		current.edges = append(current.edges, ps6080CallbackEdge{
			target: target,
			forward: ps6080MayCallbackForward{
				call: call, function: function, parents: parents, dispatcher: signature,
			},
			argumentOffset: offset,
		})
	}
	addSite := func(current *ps6080CallbackNode, call *ast.CallExpr, function *ps6080Function, parents map[ast.Node]ast.Node) {
		site := sites[call]
		if site == nil {
			site = &ps6080MayCallbackSite{call: call, function: function, parents: parents}
			sites[call] = site
		}
		direct := directSites[current]
		if direct == nil {
			direct = make(map[*ps6080MayCallbackSite]bool)
			directSites[current] = direct
		}
		if !direct[site] {
			direct[site] = true
			current.direct = append(current.direct, site)
			current.reachable[site] = true
		}
	}
	type wrapperDependencyKey struct {
		target  *ps6080CallbackNode
		literal *ast.FuncLit
		call    *ast.CallExpr
	}
	wrapperKeys := make(map[wrapperDependencyKey]bool)
	addWrapper := func(wrapper wrapperDependency) {
		key := wrapperDependencyKey{target: wrapper.target, literal: wrapper.literal, call: wrapper.call}
		if !wrapperKeys[key] {
			wrapperKeys[key] = true
			wrappers = append(wrappers, wrapper)
		}
	}
	functions := make(map[*types.Func]*ps6080Function)
	var ordered []*ps6080Function
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if !ok {
				continue
			}
			signature, _ := object.Type().(*types.Signature)
			if signature == nil {
				continue
			}
			info := &ps6080Function{declaration: function, object: object, signature: signature, body: function.Body}
			functions[object] = info
			ordered = append(ordered, info)
			for parameter := range signature.Params().Len() {
				if !ps6080CallableType(signature.Params().At(parameter).Type()) {
					continue
				}
				key := ps6080CallbackNodeKey{function: object, parameter: parameter}
				result.nodes[key] = &ps6080CallbackNode{key: key, reachable: make(map[*ps6080MayCallbackSite]bool)}
			}
		}
	}
	transfers := make(map[*types.Func]func(ast.Node) bool, len(ordered))
	for _, function := range ordered {
		parameters := make(map[types.Object]int)
		for index := range function.signature.Params().Len() {
			parameter := function.signature.Params().At(index)
			if ps6080CallableType(parameter.Type()) {
				parameters[parameter] = index
			}
		}
		if len(parameters) == 0 {
			continue
		}
		parents := ps6071Parents(function.body)
		graph := cfg.New(function.body, ps6080CallMayReturn(pass))
		literalGraphs := make(map[*ast.FuncLit]*cfg.CFG)
		literalInvocations := ps6080CallbackLiteralInvocations(pass, function, parents)
		origins := &ps6080CallbackOrigins{
			pass: pass, function: function, parameters: parameters, parents: parents,
			invocations: literalInvocations, scopes: make(map[*ast.BlockStmt]*ps6080CallbackOriginScope),
		}
		markUnknown := func(expression ast.Expr, query ast.Node, returned bool) {
			for parameter, present := range origins.at(expression, query) {
				if present {
					node := result.nodes[ps6080CallbackNodeKey{function: function.object, parameter: parameter}]
					node.unknown = true
					node.returned = node.returned || returned
					node.opaque = node.opaque || !returned
				}
			}
		}
		transfer := func(node ast.Node) bool {
			if literal, nested := node.(*ast.FuncLit); nested {
				return len(literalInvocations[literal]) > 0
			}
			if node == nil {
				return true
			}
			callGraph := graph
			if literal := ps6080ContainingLiteral(node, parents); literal != nil {
				callGraph = literalGraphs[literal]
				if callGraph == nil {
					callGraph = cfg.New(literal.Body, ps6080CallMayReturn(pass))
					literalGraphs[literal] = callGraph
				}
				if !ps6080CallbackLiteralReachable(pass, graph, literalGraphs, parents, literalInvocations, literal, false, make(map[*ast.FuncLit]bool)) {
					return true
				}
			}
			if !ps6080NodeReachable(pass, callGraph, parents, node) {
				return true
			}
			switch value := node.(type) {
			case *ast.ReturnStmt:
				for _, expression := range value.Results {
					markUnknown(expression, value, true)
				}
			case *ast.UnaryExpr:
				if value.Op == token.AND {
					if _, fresh := ps2110Unparen(value.X).(*ast.CompositeLit); fresh {
						break
					}
					markUnknown(value.X, value, false)
				}
			case *ast.SendStmt:
				markUnknown(value.Value, value, false)
			case *ast.AssignStmt:
				for index, lhs := range value.Lhs {
					local := false
					if identifier, ok := lhs.(*ast.Ident); ok {
						object := pass.TypesInfo.ObjectOf(identifier)
						local = identifier.Name == "_" || object != nil && object.Parent() != pass.Pkg.Scope()
					}
					if selector, ok := lhs.(*ast.SelectorExpr); ok {
						if identifier, direct := selector.X.(*ast.Ident); direct {
							object := pass.TypesInfo.ObjectOf(identifier)
							_, structure := object.Type().Underlying().(*types.Struct)
							local = structure && object.Parent() != pass.Pkg.Scope()
						}
					}
					if !local && len(value.Lhs) == len(value.Rhs) {
						markUnknown(value.Rhs[index], value, false)
					}
				}
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if pass.TypesInfo.Types[call.Fun].IsType() {
				return true
			}
			literalCallee := ps6080DirectLiteralCallee(pass, function.body, call.Fun, call.Pos())
			if identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok && literalCallee {
				body, _ := origins.bodyAt(call)
				for definition := range origins.definitionsAt(body, pass.TypesInfo.ObjectOf(identifier), nil, call.Pos()) {
					if definition == nil {
						literalCallee = false
					}
				}
			}
			if !literalCallee {
				for parameter, callback := range origins.at(call.Fun, call) {
					if !callback {
						continue
					}
					current := result.nodes[ps6080CallbackNodeKey{function: function.object, parameter: parameter}]
					addSite(current, call, function, parents)
				}
			}
			callee, signature, direct := typedCallee(pass, call.Fun)
			methodExpression := ps6091MethodExpression(pass, call.Fun)
			if !direct {
				if target, resolved := ps6080StaticNamedCallee(pass, function, call, parents); resolved && target.function != nil {
					callee = target.function
					signature, _ = callee.Type().(*types.Signature)
					direct = signature != nil
					methodExpression = target.methodExpression
				}
			}
			offset := 0
			if methodExpression {
				offset = 1
			}
			for argument, expression := range call.Args {
				if literalCallee {
					continue
				}
				if literal, resolved := ps6080StaticFunctionLiteral(pass, function, expression, call, parents); resolved && direct && functions[callee] != nil && signature != nil && !signature.Variadic() {
					calleeParameter := argument - offset
					if calleeParameter >= 0 {
						if target := result.nodes[ps6080CallbackNodeKey{function: callee, parameter: calleeParameter}]; target != nil {
							addWrapper(wrapperDependency{target: target, literal: literal, call: call, function: function, origins: origins, parents: parents})
							continue
						}
					}
				}
				for parameter, forwarded := range origins.at(expression, call) {
					if !forwarded {
						continue
					}
					current := result.nodes[ps6080CallbackNodeKey{function: function.object, parameter: parameter}]
					calleeParameter := argument - offset
					if !direct || functions[callee] == nil || signature == nil || signature.Variadic() || calleeParameter < 0 {
						current.unknown = true
						current.opaque = true
						continue
					}
					target := result.nodes[ps6080CallbackNodeKey{function: callee, parameter: calleeParameter}]
					if target == nil {
						current.unknown = true
						current.opaque = true
						continue
					}
					addEdge(current, target, call, function, parents, signature, offset)
				}
			}
			return true
		}
		transfers[function.object] = transfer
		ast.Inspect(function.body, transfer)
	}
	ps6080PropagateCallbackReachability(result)
	processed := make(map[wrapperDependencyKey]bool, len(wrappers))
	for {
		progress := false
		for _, wrapper := range wrappers {
			key := wrapperDependencyKey{target: wrapper.target, literal: wrapper.literal, call: wrapper.call}
			if processed[key] || len(wrapper.target.reachable) == 0 && !wrapper.target.opaque {
				continue
			}
			processed[key] = true
			progress = true
			wrapper.origins.invocations[wrapper.literal] = append(wrapper.origins.invocations[wrapper.literal], wrapper.call)
			ast.Inspect(wrapper.literal.Body, transfers[wrapper.function.object])
			ps6080PropagateCallbackReachability(result)
		}
		if !progress {
			break
		}
	}
	return result
}

func ps6080DirectLiteralCallee(pass *analysis.Pass, body *ast.BlockStmt, expression ast.Expr, before token.Pos) bool {
	if _, direct := ps2110Unparen(expression).(*ast.FuncLit); direct {
		return true
	}
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return false
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	found := false
	valid := true
	ps6071InspectOwnBody(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Pos() >= before || len(assignment.Lhs) != len(assignment.Rhs) {
			return true
		}
		for index, lhs := range assignment.Lhs {
			left, named := ps2110Unparen(lhs).(*ast.Ident)
			if !named || pass.TypesInfo.ObjectOf(left) != object {
				continue
			}
			if _, literal := ps2110Unparen(assignment.Rhs[index]).(*ast.FuncLit); !literal || found {
				valid = false
			} else {
				found = true
			}
		}
		return valid
	})
	return valid && found
}

func ps6080PropagateCallbackReachability(graph *ps6080CallbackGraph) {
	reverse := make(map[*ps6080CallbackNode][]*ps6080CallbackNode, len(graph.nodes))
	queue := make([]*ps6080CallbackNode, 0, len(graph.nodes))
	queued := make(map[*ps6080CallbackNode]bool, len(graph.nodes))
	for _, node := range graph.nodes {
		for _, edge := range node.edges {
			reverse[edge.target] = append(reverse[edge.target], node)
		}
		if len(node.reachable) > 0 || node.unknown {
			queue = append(queue, node)
			queued[node] = true
		}
	}
	for index := 0; index < len(queue); index++ {
		node := queue[index]
		queued[node] = false
		for _, caller := range reverse[node] {
			changed := false
			if node.unknown && !caller.unknown {
				caller.unknown = true
				changed = true
			}
			if node.returned && !caller.returned {
				caller.returned = true
				changed = true
			}
			if node.opaque && !caller.opaque {
				caller.opaque = true
				changed = true
			}
			for site := range node.reachable {
				if !caller.reachable[site] {
					caller.reachable[site] = true
					changed = true
				}
			}
			if changed && !queued[caller] {
				queue = append(queue, caller)
				queued[caller] = true
			}
		}
	}
}

package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"
)

// Provenance has finite height: absent, one exact source path, or ambiguous.
// A second path does not create another invocation. It invalidates ordering
// evidence while retaining the reachable callback effect. Linked cells share
// the single path and make recursive/diamond propagation independent of the
// number of invocation paths and unrelated declarations.
type ps6080CallbackProvenance struct {
	edge      *ps6080CallbackEdge
	inner     *ps6080CallbackProvenance
	ambiguous bool
}

func ps6080MayNamedCallbackSites(pass *analysis.Pass) map[*types.Func][][]*ps6080MayCallbackSite {
	if cached, ok := ps6080MayCallbackCaches.Load(pass); ok {
		return cached.(map[*types.Func][][]*ps6080MayCallbackSite)
	}
	graph := ps6080BuildCallbackGraph(pass)
	type incoming struct {
		node *ps6080CallbackNode
		edge *ps6080CallbackEdge
	}
	reverse := make(map[*ps6080CallbackNode][]incoming)
	states := make(map[*ps6080CallbackNode]map[*ps6080MayCallbackSite]*ps6080CallbackProvenance, len(graph.nodes))
	var queue []*ps6080CallbackNode
	queued := make(map[*ps6080CallbackNode]bool)
	for _, node := range graph.nodes {
		states[node] = make(map[*ps6080MayCallbackSite]*ps6080CallbackProvenance)
		for index := range node.edges {
			edge := &node.edges[index]
			reverse[edge.target] = append(reverse[edge.target], incoming{node: node, edge: edge})
		}
		for _, site := range node.direct {
			states[node][site] = &ps6080CallbackProvenance{}
		}
		if len(node.direct) > 0 {
			queue = append(queue, node)
			queued[node] = true
		}
	}
	for index := 0; index < len(queue); index++ {
		node := queue[index]
		queued[node] = false
		for _, caller := range reverse[node] {
			changed := false
			for site, inner := range states[node] {
				current := states[caller.node][site]
				if current == nil {
					states[caller.node][site] = &ps6080CallbackProvenance{
						edge: caller.edge, inner: inner, ambiguous: inner.ambiguous,
					}
					changed = true
				} else if !current.ambiguous &&
					(inner.ambiguous || current.edge != caller.edge || current.inner != inner) {
					current.ambiguous = true
					changed = true
				}
			}
			if changed && !queued[caller.node] {
				queue = append(queue, caller.node)
				queued[caller.node] = true
			}
		}
	}
	result := make(map[*types.Func][][]*ps6080MayCallbackSite, len(graph.nodes))
	for _, node := range graph.nodes {
		parameters := ps6080GrowIndexSlice(result[node.key.function], node.key.parameter)
		var sites []*ps6080MayCallbackSite
		if capacity := len(states[node]) + 1; len(states[node]) > 0 || node.unknown {
			sites = make([]*ps6080MayCallbackSite, 0, capacity)
		}
		for terminal, provenance := range states[node] {
			site := *terminal
			site.unknown = provenance.ambiguous
			if !site.unknown {
				site.order = []token.Pos{site.call.Pos()}
				var edges []*ps6080CallbackEdge
				for current := provenance; current.edge != nil; current = current.inner {
					edges = append(edges, current.edge)
				}
				for _, edge := range slices.Backward(edges) {
					forward := edge.forward
					forward.argumentOffset = edge.argumentOffset
					site.forwarding = append(site.forwarding, forward)
					site.order = append(site.order, edge.forward.call.Pos())
				}
			}
			sites = append(sites, &site)
		}
		if node.unknown {
			// An opaque escape is a possible invocation, not a proven no-op.
			sites = append(sites, &ps6080MayCallbackSite{unknown: true, returnOnly: node.returned && !node.opaque})
		}
		slices.SortFunc(sites, func(left, right *ps6080MayCallbackSite) int {
			if left.call == nil && right.call == nil {
				return 0
			}
			if left.call == nil {
				return -1
			}
			if right.call == nil {
				return 1
			}
			return int(left.call.Pos() - right.call.Pos())
		})
		parameters[node.key.parameter] = sites
		result[node.key.function] = parameters
	}
	value, _ := ps6080MayCallbackCaches.LoadOrStore(pass, result)
	return value.(map[*types.Func][][]*ps6080MayCallbackSite)
}

func ps6080UnknownCallbackSites(sites []*ps6080MayCallbackSite) bool {
	for _, site := range sites {
		if site.unknown {
			return true
		}
	}
	return false
}

func ps6080CallbackMayInvoke(sites []*ps6080MayCallbackSite) bool {
	for _, site := range sites {
		if !site.returnOnly {
			return true
		}
	}
	return false
}

// A nested literal inherits uncertain invocation provenance from its caller.
// Retain calls for MAY analysis, but never manufacture a definite order when
// crossing such a context. The fixed point ranges over source literals only.
func ps6080PropagateUnknownInvocations(result *ps6080InvokedLiteralResult, parents map[ast.Node]ast.Node) {
	changed := true
	for changed {
		changed = false
		for literal, calls := range result.calls {
			for call := range calls {
				outer := ps6080ContainingLiteral(call, parents)
				if len(result.unknown[outer]) == 0 || result.unknown[literal][call] {
					continue
				}
				if result.unknown[literal] == nil {
					result.unknown[literal] = make(map[*ast.CallExpr]bool)
				}
				result.unknown[literal][call] = true
				changed = true
			}
		}
	}
}

func ps6080NamedCallbackInvocations(pass *analysis.Pass) map[*types.Func][][]ps6080NamedCallbackInvocation {
	if cached, ok := ps6080NamedOrderCaches.Load(pass); ok {
		return cached.(map[*types.Func][][]ps6080NamedCallbackInvocation)
	}
	graphs := make(map[*ast.BlockStmt]*cfg.CFG)
	originScopes := make(map[*ast.BlockStmt]*ps6080CallbackOriginScope)
	stableParameter := func(function *ps6080Function, expression ast.Expr, query ast.Node) (int, bool) {
		origins := &ps6080CallbackOrigins{pass: pass, function: function, scopes: originScopes}
		seen := make(map[types.Object]bool)
		for {
			identifier, ok := ps2110Unparen(expression).(*ast.Ident)
			if !ok {
				return 0, false
			}
			object := pass.TypesInfo.ObjectOf(identifier)
			if object == nil || seen[object] || ps6080ObjectAddressTaken(pass, function.body, object) {
				return 0, false
			}
			parents := ps6071Parents(function.body)
			literalCalls := ps6080CallbackLiteralInvocations(pass, function, parents)
			capturedWrite := false
			for literal, calls := range literalCalls {
				if len(calls) > 0 && len(ps6080NonIdentityFunctionAssignments(pass, object, ps6080FunctionAssignments(pass, literal.Body, object))) > 0 {
					capturedWrite = true
					break
				}
			}
			if capturedWrite {
				// The intrabody definition lattice does not summarize writes
				// performed by invoked closures. Such a write prevents a MUST
				// identity proof; the MAY callback effect remains available.
				return 0, false
			}
			seen[object] = true
			definitions := origins.definitionsAt(function.body, object, nil, query.Pos())
			if len(definitions) != 1 {
				return 0, false
			}
			var definition *ps6080CallbackDefinition
			for candidate := range definitions {
				definition = candidate
			}
			if definition != nil {
				expression, query = definition.expression, definition.node
				continue
			}
			for index := range function.signature.Params().Len() {
				if function.signature.Params().At(index) == object {
					return index, true
				}
			}
			return 0, false
		}
	}
	guaranteed := func(function *ps6080Function, call *ast.CallExpr, parents map[ast.Node]ast.Node) bool {
		if ps6080ContainingLiteral(call, parents) != nil {
			return false
		}
		switch parents[call].(type) {
		case *ast.GoStmt, *ast.DeferStmt:
			return false
		}
		graph := graphs[function.body]
		if graph == nil {
			graph = cfg.New(function.body, ps6080CallMayReturn(pass))
			graphs[function.body] = graph
		}
		return ps6080CFGNodeGuaranteed(graph, call)
	}
	parameterIndex := func(signature *types.Signature, expression ast.Expr) (int, bool) {
		identifier, ok := ps2110Unparen(expression).(*ast.Ident)
		if !ok {
			return 0, false
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		for index := range signature.Params().Len() {
			if signature.Params().At(index) == object {
				return index, true
			}
		}
		return 0, false
	}
	result := make(map[*types.Func][][]ps6080NamedCallbackInvocation)
	for function, parameters := range ps6080MayNamedCallbackSites(pass) {
		for parameter, sites := range parameters {
			for _, site := range sites {
				if site.unknown || !guaranteed(site.function, site.call, site.parents) {
					continue
				}
				callback, stable := stableParameter(site.function, site.call.Fun, site.call)
				if !stable {
					continue
				}
				valid := true
				var arguments ps6080InvocationArguments
				for index, expression := range site.call.Args {
					if source, ok := parameterIndex(site.function.signature, expression); ok {
						ps6080AddInvocationArgument(&arguments, index, source)
					}
				}
				for _, forward := range site.forwarding {
					if !guaranteed(forward.function, forward.call, forward.parents) {
						valid = false
						break
					}
					offset := forward.argumentOffset
					if callback+offset >= len(forward.call.Args) {
						valid = false
						break
					}
					callback, stable = stableParameter(forward.function, forward.call.Args[callback+offset], forward.call)
					if !stable {
						valid = false
						break
					}
					var mapped ps6080InvocationArguments
					for argument, sources := range arguments {
						for source, present := range sources {
							if !present || source+offset >= len(forward.call.Args) {
								continue
							}
							if index, ok := parameterIndex(forward.function.signature, forward.call.Args[source+offset]); ok {
								ps6080AddInvocationArgument(&mapped, argument, index)
							}
						}
					}
					arguments = mapped
				}
				if valid && callback == parameter {
					values := ps6080GrowIndexSlice(result[function], parameter)
					order := slices.Clone(site.order)
					slices.Reverse(order)
					values[parameter] = append(values[parameter], ps6080NamedCallbackInvocation{order: order, arguments: arguments})
					result[function] = values
				}
			}
		}
	}
	value, _ := ps6080NamedOrderCaches.LoadOrStore(pass, result)
	return value.(map[*types.Func][][]ps6080NamedCallbackInvocation)
}

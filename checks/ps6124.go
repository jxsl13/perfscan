package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6124 implements owner issue #834. It proves a may-reach global-router
// route from typed source and facts; configuration names APIs and lifecycle
// boundaries, but never asserts that the route exists.
var PS6124 = register(&lint.Check{
	ID: "PS6124", Category: "verify", Slug: "scoped-backend-context-needs-global-route-pin",
	Level: lint.LevelAggressive, AutoFix: false, NeedsConfig: true,
	Vocab: []string{"scopedBackendRoutingContracts"},
	Doc: lint.Documentation{
		Title: "a backend-specific measurement context does not pin a downstream global route",
		Text: `A backend-specific test or benchmark can construct an explicit Context yet may execute a downstream operation through an independent process-global backend preference. PS6124 reports only a configured site with a typed selected-backend Get/WithBackend flow, a configured construction boundary, and a source-derived concrete may-reach path from the constructed value to a configured global router. Package imports, API spelling, and the contract alone do not establish runtime execution.

The route proof follows direct typed calls through package-local bodies and imported object facts. Calls through interfaces, function values, reflection, and unresolved callbacks are conservative unknowns and do not create a finding. A project-owned contract must separately attest that the measurement owns or serializes process-global preference writers.

A safe scoped pin snapshots the exact preference, sets the exact selected backend, immediately defers variadic restoration of that snapshot, precedes construction, and has no other visible preference mutation in the configured site or its invoked local closure. Conditional, late, wrong, shadowed, overwritten, or unrestored pins do not suppress the advisory.

There is NO automatic fix: global-state rewrites can affect concurrent tests and unrelated routing. Preserve context and global-route identity, serialize the mutation region, restore on every return and panic, and validate output plus backend attribution. Benchmark attribution is advisory, not a fabricated speedup claim. A benchmark with separately verified routing attribution may use //perfscan:ignore PS6124 with that evidence instead of mutating global state.`,
		Before: `model := loadModel()
run := func() {
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	model.DecodeStep(ctx) // downstream uses backend.Default()
}
run()`,
		After: `previous := backend.Preference()
backend.SetPreference(backend.CPU)
defer backend.SetPreference(previous...)
model := loadModel()
run := func() {
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	model.DecodeStep(ctx)
}
run()`,
	},
	Analyzer: &analysis.Analyzer{Name: "PS6124", Doc: "scoped context selection with an unpinned downstream global backend route", Run: runPS6124, FactTypes: []analysis.Fact{new(ps6124RouteFact)}},
})

type ps6124RouteFact struct {
	Routers []string
	Writers []string
	// InvokedParams contains zero-based ordinary parameter indexes which the
	// function may invoke, directly or through another source-proven callee.
	InvokedParams []int
}

func (*ps6124RouteFact) AFact() {}
func (f *ps6124RouteFact) String() string {
	return "global backend effects: routers=" + strings.Join(f.Routers, ",") + ";writers=" + strings.Join(f.Writers, ",")
}

type ps6124Package struct {
	pass    *analysis.Pass
	locals  map[*types.Func]*ast.FuncDecl
	routes  map[*types.Func]map[string]bool
	writers map[*types.Func]map[string]bool
	invoked map[*types.Func][]int
	flows   map[*ast.BlockStmt]*ps6124BodyFlow
}

type ps6124BodyFlow struct {
	flow    *ps6122Flow
	parents map[ast.Node]ast.Node
	live    map[*ast.CallExpr]bool
}

func runPS6124(pass *analysis.Pass) (any, error) {
	return runPS6124WithContracts(pass, config.Current().ScopedBackendRoutingContracts)
}

func runPS6124WithContracts(pass *analysis.Pass, configured []config.ScopedBackendRoutingContract) (any, error) {
	if config.UsableScopedBackendRoutingContractCount(configured) == 0 {
		return nil, nil
	}
	pkg := &ps6124Package{pass: pass, locals: make(map[*types.Func]*ast.FuncDecl), routes: make(map[*types.Func]map[string]bool), writers: make(map[*types.Func]map[string]bool), invoked: make(map[*types.Func][]int), flows: make(map[*ast.BlockStmt]*ps6124BodyFlow)}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			if object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func); ok {
				pkg.locals[object.Origin()] = function
			}
		}
	}
	pkg.deriveRoutes(configured)
	for index := range configured {
		contract := &configured[index]
		if !contract.Valid() || ps6124Ambiguous(configured, index) {
			continue
		}
		for object, declaration := range pkg.locals {
			if ps6090FunctionID(object) == contract.ConfiguredSite {
				pkg.scanConfiguredSite(declaration, contract)
			}
		}
	}
	return nil, nil
}

func (pkg *ps6124Package) bodyFlow(body *ast.BlockStmt) *ps6124BodyFlow {
	if summary := pkg.flows[body]; summary != nil {
		return summary
	}
	flow := ps6122NewFlow(pkg.pass, body)
	summary := &ps6124BodyFlow{flow: flow, parents: ps6087Parents(body), live: make(map[*ast.CallExpr]bool)}
	ps6071InspectOwnBody(body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok && ps6122CanEnter(pkg.pass, body, flow.parents, call.Pos()) && ps6122BlockAt(pkg.pass, flow, call.Pos()) != nil {
			summary.live[call] = true
		}
		return true
	})
	pkg.flows[body] = summary
	return summary
}

func ps6124Ambiguous(contracts []config.ScopedBackendRoutingContract, index int) bool {
	for other := range contracts {
		if other != index && contracts[other].Valid() && contracts[other].ConfiguredSite == contracts[index].ConfiguredSite {
			return true
		}
	}
	return false
}

func (pkg *ps6124Package) deriveRoutes(contracts []config.ScopedBackendRoutingContract) {
	knownRouters := make(map[string]bool)
	knownWriters := make(map[string]bool)
	for index := range contracts {
		if contracts[index].Valid() {
			for _, router := range contracts[index].GlobalRouters {
				knownRouters[router] = true
			}
			knownWriters[contracts[index].SetPreference] = true
		}
	}
	edges := make(map[*types.Func]map[*types.Func]bool, len(pkg.locals))
	pkg.deriveInvokedParameters()
	for object, declaration := range pkg.locals {
		pkg.routes[object] = make(map[string]bool)
		pkg.writers[object] = make(map[string]bool)
		edges[object] = make(map[*types.Func]bool)
		for _, body := range pkg.invokedBodies(declaration.Body, token.NoPos) {
			pkg.collectDirectEffects(body, knownRouters, knownWriters, pkg.routes[object], pkg.writers[object], edges[object])
		}
	}
	callers := make(map[*types.Func]map[*types.Func]bool)
	for caller, callees := range edges {
		for callee := range callees {
			if callers[callee] == nil {
				callers[callee] = make(map[*types.Func]bool)
			}
			callers[callee][caller] = true
		}
	}
	queue := make([]*types.Func, 0, len(pkg.locals))
	queued := make(map[*types.Func]bool)
	for object := range pkg.locals {
		if len(pkg.routes[object]) != 0 || len(pkg.writers[object]) != 0 {
			queue = append(queue, object)
			queued[object] = true
		}
	}
	for len(queue) != 0 {
		callee := queue[0]
		queue = queue[1:]
		queued[callee] = false
		for caller := range callers[callee] {
			changed := false
			routes, writers := pkg.routes[caller], pkg.writers[caller]
			for router := range pkg.routes[callee] {
				if !routes[router] {
					routes[router] = true
					changed = true
				}
			}
			for writer := range pkg.writers[callee] {
				if !writers[writer] {
					writers[writer] = true
					changed = true
				}
			}
			if changed && !queued[caller] {
				queue = append(queue, caller)
				queued[caller] = true
			}
		}
	}
	for object, routers := range pkg.routes {
		if len(routers) == 0 && len(pkg.writers[object]) == 0 && len(pkg.invoked[object]) == 0 {
			continue
		}
		fact := &ps6124RouteFact{Routers: make([]string, 0, len(routers)), Writers: make([]string, 0, len(pkg.writers[object])), InvokedParams: slices.Clone(pkg.invoked[object])}
		for router := range routers {
			fact.Routers = append(fact.Routers, router)
		}
		slices.Sort(fact.Routers)
		for writer := range pkg.writers[object] {
			fact.Writers = append(fact.Writers, writer)
		}
		slices.Sort(fact.Writers)
		pkg.pass.ExportObjectFact(object, fact)
	}
}

func (pkg *ps6124Package) deriveInvokedParameters() {
	type invocation struct {
		function  *types.Func
		parameter int
	}
	reverse := make(map[invocation]map[invocation]bool)
	known := make(map[invocation]bool)
	var queue []invocation
	add := func(value invocation) {
		if !known[value] {
			known[value] = true
			queue = append(queue, value)
		}
	}
	// Build the finite forwarding graph once. A node means “this ordinary
	// parameter may be invoked”; edges map a callee parameter back to the
	// caller parameter supplied to it.
	for caller, declaration := range pkg.locals {
		parameters := ps6124ParameterIndices(pkg.pass, declaration)
		for _, invokedBody := range pkg.invokedBodies(declaration.Body, token.NoPos) {
			summary := pkg.bodyFlow(invokedBody)
			flow := summary.flow
			live := summary.live
			ps6071InspectOwnBody(invokedBody, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || !live[call] {
					return true
				}
				if id, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok {
					for _, index := range ps6124ParameterOrigins(pkg.pass, flow, parameters, id, call.Pos(), nil) {
						if invokedBody == declaration.Body || pkg.parameterReachesBodyInvocation(declaration.Body, invokedBody, parameters, index) {
							add(invocation{caller, index})
						}
					}
				}
				callee := ps6071CalledFunction(pkg.pass, call)
				if callee == nil {
					return true
				}
				callee = callee.Origin()
				offset := ps6124MethodExpressionOffset(pkg.pass, call)
				for argument, expression := range call.Args {
					if argument < offset {
						continue
					}
					id, ok := ps2110Unparen(expression).(*ast.Ident)
					if !ok {
						continue
					}
					for _, callerIndex := range ps6124ParameterOrigins(pkg.pass, flow, parameters, id, call.Pos(), nil) {
						if invokedBody != declaration.Body && !pkg.parameterReachesBodyInvocation(declaration.Body, invokedBody, parameters, callerIndex) {
							continue
						}
						from := invocation{callee, argument - offset}
						to := invocation{caller, callerIndex}
						if reverse[from] == nil {
							reverse[from] = make(map[invocation]bool)
						}
						reverse[from][to] = true
					}
				}
				if callee.Pkg() != pkg.pass.Pkg {
					var fact ps6124RouteFact
					if pkg.pass.ImportObjectFact(callee, &fact) {
						for _, index := range fact.InvokedParams {
							add(invocation{callee, index})
						}
					}
				}
				return true
			})
		}
	}
	for len(queue) != 0 {
		value := queue[0]
		queue = queue[1:]
		for caller := range reverse[value] {
			add(caller)
		}
	}
	for value := range known {
		if value.function.Pkg() == pkg.pass.Pkg {
			pkg.invoked[value.function] = append(pkg.invoked[value.function], value.parameter)
		}
	}
	for function := range pkg.invoked {
		slices.Sort(pkg.invoked[function])
	}
}

func (pkg *ps6124Package) parameterReachesBodyInvocation(owner, invoked *ast.BlockStmt, parameters map[types.Object]int, index int) bool {
	var parameter types.Object
	for object, candidate := range parameters {
		if candidate == index {
			parameter = object
			break
		}
	}
	if parameter == nil {
		return false
	}
	summary := pkg.bodyFlow(owner)
	reaches := false
	ps6071InspectOwnBody(owner, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !summary.live[call] {
			return true
		}
		matched := false
		if literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit); ok {
			matched = literal.Body == invoked
		}
		if !matched {
			if id, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok {
				for _, literal := range ps6124ResolvedLiteralsAt(pkg.pass, summary.flow, owner, pkg.pass.TypesInfo.ObjectOf(id), call.Pos()) {
					if literal.Body == invoked {
						matched = true
					}
				}
			}
		}
		if matched && ps6124ParameterReaches(pkg.pass, summary.flow, parameter, call.Pos()) {
			reaches = true
		}
		return !reaches
	})
	return reaches
}

func (pkg *ps6124Package) invokedFor(callee *types.Func) []int {
	callee = callee.Origin()
	if callee.Pkg() == pkg.pass.Pkg {
		return pkg.invoked[callee]
	}
	var fact ps6124RouteFact
	if pkg.pass.ImportObjectFact(callee, &fact) {
		return fact.InvokedParams
	}
	return nil
}

func ps6124ParameterIndices(pass *analysis.Pass, declaration *ast.FuncDecl) map[types.Object]int {
	result := make(map[types.Object]int)
	position := 0
	if declaration.Type.Params == nil {
		return result
	}
	for _, field := range declaration.Type.Params.List {
		if len(field.Names) == 0 {
			position++
			continue
		}
		for _, name := range field.Names {
			result[pass.TypesInfo.Defs[name]] = position
			position++
		}
	}
	return result
}

func ps6124ParameterReaches(pass *analysis.Pass, flow *ps6122Flow, object types.Object, call token.Pos) bool {
	target := ps6122BlockAt(pass, flow, call)
	if target == nil || len(flow.graph.Blocks) == 0 {
		return false
	}
	seen := make(map[*cfg.Block]bool)
	queue := []*cfg.Block{flow.graph.Blocks[0]}
	for len(queue) != 0 {
		block := queue[0]
		queue = queue[1:]
		if seen[block] {
			continue
		}
		seen[block] = true
		killed := false
		for _, node := range block.Nodes {
			if node.Pos() >= call && block == target {
				break
			}
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				continue
			}
			for _, lhs := range assignment.Lhs {
				if ps6124ExprObject(pass, lhs) == object {
					killed = true
				}
			}
		}
		if killed {
			continue
		}
		if block == target {
			return true
		}
		queue = append(queue, ps6122Successors(pass, flow, block, token.NoPos)...)
	}
	return false
}

func ps6124ParameterOrigins(pass *analysis.Pass, flow *ps6122Flow, parameters map[types.Object]int, expression ast.Expr, at token.Pos, visiting map[types.Object]bool) []int {
	object := ps6124ExprObject(pass, expression)
	if object == nil {
		return nil
	}
	if index, ok := parameters[object]; ok && ps6124ParameterReaches(pass, flow, object, at) {
		return []int{index}
	}
	if visiting == nil {
		visiting = make(map[types.Object]bool)
	}
	if visiting[object] {
		return nil
	}
	visiting[object] = true
	seen := make(map[int]bool)
	ast.Inspect(flow.body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Pos() >= at || len(assignment.Lhs) != len(assignment.Rhs) {
			return true
		}
		for index, lhs := range assignment.Lhs {
			if ps6124ExprObject(pass, lhs) == object && ps6124DefinitionReaches(pass, flow, object, assignment.Pos(), at) {
				for _, origin := range ps6124ParameterOrigins(pass, flow, parameters, assignment.Rhs[index], assignment.Pos(), visiting) {
					seen[origin] = true
				}
			}
		}
		return true
	})
	delete(visiting, object)
	result := make([]int, 0, len(seen))
	for index := range seen {
		result = append(result, index)
	}
	slices.Sort(result)
	return result
}

func ps6124MethodExpressionOffset(pass *analysis.Pass, call *ast.CallExpr) int {
	selector, _ := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if selector != nil {
		if selection := pass.TypesInfo.Selections[selector]; selection != nil && selection.Kind() == types.MethodExpr {
			return 1
		}
	}
	return 0
}

func (pkg *ps6124Package) collectDirectEffects(body *ast.BlockStmt, configured, writers, result, writes map[string]bool, edges map[*types.Func]bool) {
	summary := pkg.bodyFlow(body)
	parents := summary.parents
	live := summary.live
	ps6071InspectOwnBody(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ps6124ConstantDead(pkg.pass, call, parents) {
			return true
		}
		if !live[call] {
			return true
		}
		callee := ps6071CalledFunction(pkg.pass, call)
		if callee == nil {
			return true
		}
		callee = callee.Origin()
		edges[callee] = true
		for _, index := range pkg.invokedFor(callee) {
			index += ps6124MethodExpressionOffset(pkg.pass, call)
			if index >= len(call.Args) {
				continue
			}
			literal, ok := ps2110Unparen(call.Args[index]).(*ast.FuncLit)
			if !ok {
				if id, named := ps2110Unparen(call.Args[index]).(*ast.Ident); named {
					if function, known := pkg.pass.TypesInfo.ObjectOf(id).(*types.Func); known {
						edges[function.Origin()] = true
					}
				}
				continue
			}
			for _, invoked := range pkg.invokedBodies(literal.Body, token.NoPos) {
				pkg.collectDirectEffects(invoked, configured, writers, result, writes, edges)
			}
		}
		id := ps6090FunctionID(callee)
		if configured[id] {
			result[id] = true
		}
		if writers[id] {
			writes[id] = true
		}
		if callee.Pkg() != pkg.pass.Pkg {
			var fact ps6124RouteFact
			if pkg.pass.ImportObjectFact(callee, &fact) {
				for _, router := range fact.Routers {
					if configured[router] {
						result[router] = true
					}
				}
				for _, writer := range fact.Writers {
					if writers[writer] {
						writes[writer] = true
					}
				}
			}
		}
		return true
	})
}

type ps6124Site struct {
	construct  *ast.CallExpr
	model      types.Object
	route      *ast.CallExpr
	context    types.Object
	getBackend types.Object
}

func (pkg *ps6124Package) scanConfiguredSite(function *ast.FuncDecl, contract *config.ScopedBackendRoutingContract) {
	site := pkg.construction(function.Body, contract)
	if site == nil {
		return
	}
	if ps6124ObjectAssignedAfter(pkg.pass, function.Body, site.model, site.construct.End()) {
		return
	}
	bodies := pkg.invokedBodies(function.Body, site.construct.Pos())
	for _, body := range bodies {
		if pkg.unknownBody(body, function.Body) {
			return
		}
	}
	for _, body := range bodies {
		candidate := pkg.routeInBody(body, contract, site)
		if candidate != nil {
			site = candidate
			break
		}
	}
	if site.construct == nil || site.model == nil || site.route == nil || site.context == nil || site.getBackend == nil {
		return
	}
	if ps6124ScopedPin(pkg.pass, function.Body, bodies, pkg, contract, site.construct.Pos()) {
		return
	}
	pkg.pass.Reportf(site.route.Pos(), "%s: the selected Context reaches a source-proven downstream global backend router on a source-feasible may-reach path without an exact scoped preference pin before construction; snapshot Preference, SetPreference to the same backend, immediately defer variadic restoration, keep the reviewed writer-isolation region serial, and validate backend attribution (PS6124 advisory, no automatic fix)", contract.Name)
}

func (pkg *ps6124Package) construction(body *ast.BlockStmt, contract *config.ScopedBackendRoutingContract) *ps6124Site {
	pass := pkg.pass
	var result *ps6124Site
	matches := 0
	summary := pkg.bodyFlow(body)
	parents := summary.parents
	live := summary.live
	ps6071InspectOwnBody(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ps6124ConstantDead(pass, call, parents) {
			return true
		}
		if !live[call] {
			return true
		}
		callee := ps6071CalledFunction(pass, call)
		if callee == nil {
			return true
		}
		if slices.Contains(contract.ConstructionCallables, ps6090FunctionID(callee.Origin())) {
			matches++
			object := ps6124AssignedObject(pass, body, call)
			if matches == 1 && object != nil {
				result = &ps6124Site{construct: call, model: object}
			}
		}
		return true
	})
	if matches != 1 {
		return nil
	}
	return result
}

func (pkg *ps6124Package) routeInBody(body *ast.BlockStmt, contract *config.ScopedBackendRoutingContract, base *ps6124Site) *ps6124Site {
	site := *base
	summary := pkg.bodyFlow(body)
	parents := summary.parents
	live := summary.live
	flow := summary.flow
	ast.Inspect(body, func(node ast.Node) bool {
		if literal, ok := node.(*ast.FuncLit); ok && literal.Body != body {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ps6124ConstantDead(pkg.pass, call, parents) {
			return true
		}
		if !live[call] {
			return true
		}
		callee := ps6071CalledFunction(pkg.pass, call)
		if callee == nil {
			return true
		}
		id := ps6090FunctionID(callee.Origin())
		if id == contract.BackendGet && len(call.Args) == 1 && ps6124ObjectID(pkg.pass, call.Args[0]) == contract.SelectedBackend && site.getBackend == nil {
			site.getBackend = ps6124AssignedObject(pkg.pass, body, call)
		}
		if id == contract.ContextWithBackend && site.getBackend != nil && len(call.Args) == 1 && ps6124ExprObject(pkg.pass, call.Args[0]) == site.getBackend && site.context == nil {
			site.context = ps6124AssignedObject(pkg.pass, body, call)
		}
		if site.context != nil && ps6124ReceiverObject(pkg.pass, call) == site.model && ps6124ArgsContain(pkg.pass, call.Args, site.context) && pkg.callRoutesTo(call, contract.GlobalRouters) {
			site.route = call
		}
		return true
	})
	if site.route == nil || ps6124Reassigned(pkg.pass, body, site.model, site.getBackend, site.context, site.route.Pos()) ||
		!ps6124OrderedFlow(pkg.pass, flow, site.getBackend, site.context, site.route) {
		return nil
	}
	if ps6122BlockAt(pkg.pass, flow, site.construct.Pos()) != nil && !ps6124CanReach(pkg.pass, flow, site.construct.Pos(), site.route.Pos()) {
		return nil
	}
	return &site
}

func ps6124ConstantDead(pass *analysis.Pass, node ast.Node, parents map[ast.Node]ast.Node) bool {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		conditional, ok := parent.(*ast.IfStmt)
		if !ok {
			continue
		}
		value := pass.TypesInfo.Types[conditional.Cond].Value
		if value == nil || value.Kind() != constant.Bool {
			continue
		}
		truth := constant.BoolVal(value)
		inBody := conditional.Body.Pos() <= node.Pos() && node.End() <= conditional.Body.End()
		if !truth && inBody {
			return true
		}
		if truth && conditional.Else != nil && conditional.Else.Pos() <= node.Pos() && node.End() <= conditional.Else.End() {
			return true
		}
	}
	return false
}

func ps6124OrderedFlow(pass *analysis.Pass, flow *ps6122Flow, backend, context types.Object, route *ast.CallExpr) bool {
	backendPos := ps6124DefinitionPosition(pass, flow.body, backend, route.Pos())
	contextPos := ps6124DefinitionPosition(pass, flow.body, context, route.Pos())
	return backendPos.IsValid() && contextPos.IsValid() &&
		ps6124CanReach(pass, flow, backendPos, contextPos) && ps6124CanReach(pass, flow, contextPos, route.Pos())
}

func ps6124DefinitionPosition(pass *analysis.Pass, body *ast.BlockStmt, object types.Object, before token.Pos) token.Pos {
	position := token.NoPos
	ast.Inspect(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Pos() >= before {
			return true
		}
		for _, lhs := range assignment.Lhs {
			if ps6124ExprObject(pass, lhs) == object {
				position = assignment.Pos()
			}
		}
		return true
	})
	return position
}

func ps6124CanReach(pass *analysis.Pass, flow *ps6122Flow, from, to token.Pos) bool {
	start := ps6122BlockAt(pass, flow, from)
	target := ps6122BlockAt(pass, flow, to)
	if start == nil || target == nil {
		return false
	}
	if start == target {
		return from < to
	}
	seen := map[*cfg.Block]bool{start: true}
	queue := ps6122Successors(pass, flow, start, from)
	for len(queue) != 0 {
		block := queue[0]
		queue = queue[1:]
		if block == target {
			return true
		}
		if seen[block] {
			continue
		}
		seen[block] = true
		queue = append(queue, ps6122Successors(pass, flow, block, token.NoPos)...)
	}
	return false
}

func (pkg *ps6124Package) callRoutesTo(call *ast.CallExpr, routers []string) bool {
	callee := ps6071CalledFunction(pkg.pass, call)
	if callee == nil {
		return false
	}
	callee = callee.Origin()
	for _, router := range routers {
		if pkg.routes[callee][router] {
			return true
		}
	}
	var fact ps6124RouteFact
	if callee.Pkg() != pkg.pass.Pkg && pkg.pass.ImportObjectFact(callee, &fact) {
		for _, router := range routers {
			if slices.Contains(fact.Routers, router) {
				return true
			}
		}
	}
	return false
}

func (pkg *ps6124Package) invokedBodies(body *ast.BlockStmt, after token.Pos) []*ast.BlockStmt {
	pass := pkg.pass
	bodies := []*ast.BlockStmt{body}
	summary := pkg.bodyFlow(body)
	parents := summary.parents
	live := summary.live
	flow := summary.flow
	seen := make(map[*ast.BlockStmt]bool)
	ps6071InspectOwnBody(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		afterReachable := !after.IsValid() || ps6124CanReach(pass, flow, after, call.Pos())
		if literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit); ok && call.Pos() > after && afterReachable && live[call] && !seen[literal.Body] {
			bodies = append(bodies, literal.Body)
			seen[literal.Body] = true
		}
		if id, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok {
			object := pass.TypesInfo.ObjectOf(id)
			for _, literal := range ps6124ResolvedLiteralsAt(pass, flow, body, object, call.Pos()) {
				if call.Pos() > after && afterReachable && live[call] && !ps6124ConstantDead(pass, call, parents) && !seen[literal.Body] {
					bodies = append(bodies, literal.Body)
					seen[literal.Body] = true
				}
			}
		}
		return true
	})
	for _, candidate := range slices.Clone(bodies[1:]) {
		nested := pkg.invokedBodies(candidate, token.NoPos)
		for _, nestedBody := range nested[1:] {
			if !seen[nestedBody] {
				bodies = append(bodies, nestedBody)
				seen[nestedBody] = true
			}
		}
	}
	return bodies
}

func ps6124ResolvedLiteralsAt(pass *analysis.Pass, flow *ps6122Flow, body *ast.BlockStmt, object types.Object, before token.Pos) []*ast.FuncLit {
	var result []*ast.FuncLit
	ps6071InspectOwnBody(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Pos() >= before || len(assignment.Lhs) != len(assignment.Rhs) {
			return true
		}
		for index, lhs := range assignment.Lhs {
			if ps6124ExprObject(pass, lhs) == object && ps6124DefinitionReaches(pass, flow, object, assignment.Pos(), before) {
				if literal, ok := ps2110Unparen(assignment.Rhs[index]).(*ast.FuncLit); ok {
					result = append(result, literal)
				}
			}
		}
		return true
	})
	return result
}

func ps6124DefinitionReaches(pass *analysis.Pass, flow *ps6122Flow, object types.Object, definition, use token.Pos) bool {
	start := ps6122BlockAt(pass, flow, definition)
	target := ps6122BlockAt(pass, flow, use)
	if start == nil || target == nil {
		return false
	}
	seen := make(map[*cfg.Block]bool)
	queue := []*cfg.Block{start}
	for len(queue) != 0 {
		block := queue[0]
		queue = queue[1:]
		if seen[block] {
			continue
		}
		seen[block] = true
		killed := false
		for _, node := range block.Nodes {
			if node.Pos() <= definition && block == start {
				continue
			}
			if node.Pos() >= use && block == target {
				break
			}
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				continue
			}
			for _, lhs := range assignment.Lhs {
				if ps6124ExprObject(pass, lhs) == object {
					killed = true
				}
			}
		}
		if killed {
			continue
		}
		if block == target {
			return true
		}
		queue = append(queue, ps6122Successors(pass, flow, block, token.NoPos)...)
	}
	return false
}

func ps6124ObjectAssignedAfter(pass *analysis.Pass, body *ast.BlockStmt, object types.Object, after token.Pos) bool {
	assigned := false
	ps6071InspectOwnBody(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Pos() <= after {
			return true
		}
		for _, lhs := range assignment.Lhs {
			if ps6124ExprObject(pass, lhs) == object {
				assigned = true
			}
		}
		return !assigned
	})
	return assigned
}

func ps6124Reassigned(pass *analysis.Pass, body *ast.BlockStmt, model, backend, context types.Object, before token.Pos) bool {
	counts := map[types.Object]int{model: 0, backend: 0, context: 0}
	addressed := false
	ast.Inspect(body, func(node ast.Node) bool {
		if unary, ok := node.(*ast.UnaryExpr); ok && unary.Pos() < before && unary.Op == token.AND {
			object := ps6124ExprObject(pass, unary.X)
			if object == model || object == backend || object == context {
				addressed = true
			}
		}
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Pos() >= before {
			return true
		}
		for _, lhs := range assignment.Lhs {
			if object := ps6124ExprObject(pass, lhs); object == model || object == backend || object == context {
				counts[object]++
			}
		}
		return true
	})
	return addressed || model == nil || backend == nil || context == nil || counts[model] > 1 || counts[backend] != 1 || counts[context] != 1
}

func (pkg *ps6124Package) unknownBody(body, owner *ast.BlockStmt) bool {
	pass := pkg.pass
	unsafe := false
	ps6071InspectOwnBody(body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.GoStmt:
			unsafe = true
		case *ast.CallExpr:
			callee := ps6071CalledFunction(pass, value)
			if callee == nil {
				if ident, ok := ps2110Unparen(value.Fun).(*ast.Ident); ok {
					if _, builtin := pass.TypesInfo.ObjectOf(ident).(*types.Builtin); builtin {
						return true
					}
				}
				if typed := pass.TypesInfo.Types[value.Fun]; typed.IsType() {
					return true
				}
				id, ok := ps2110Unparen(value.Fun).(*ast.Ident)
				if !ok || !pkg.uniqueLocalLiteral(owner, pass.TypesInfo.ObjectOf(id)) {
					unsafe = true
				}
				return true
			}
		}
		return !unsafe
	})
	return unsafe
}

func (pkg *ps6124Package) uniqueLocalLiteral(body *ast.BlockStmt, object types.Object) bool {
	pass := pkg.pass
	resolved := false
	live := pkg.bodyFlow(body).live
	ps6071InspectOwnBody(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok && live[call] {
			if id, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == object && len(ps6124ResolvedLiteralsAt(pass, pkg.bodyFlow(body).flow, body, object, call.Pos())) == 1 {
				resolved = true
			}
		}
		return true
	})
	return object != nil && resolved
}

func (pkg *ps6124Package) functionWrites(callee *types.Func, writer string) bool {
	callee = callee.Origin()
	if pkg.writers[callee][writer] {
		return true
	}
	if callee.Pkg() != pkg.pass.Pkg {
		var fact ps6124RouteFact
		if pkg.pass.ImportObjectFact(callee, &fact) {
			return slices.Contains(fact.Writers, writer)
		}
	}
	return false
}

func ps6124AssignedObject(pass *analysis.Pass, body *ast.BlockStmt, call *ast.CallExpr) types.Object {
	var result types.Object
	ast.Inspect(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for index, rhs := range assignment.Rhs {
			if rhs == call && index < len(assignment.Lhs) {
				if id, ok := assignment.Lhs[index].(*ast.Ident); ok {
					result = pass.TypesInfo.ObjectOf(id)
				}
			}
		}
		return result == nil
	})
	return result
}

func ps6124ExprObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	id, _ := ps2110Unparen(expression).(*ast.Ident)
	if id == nil {
		return nil
	}
	return pass.TypesInfo.ObjectOf(id)
}

func ps6124ReceiverObject(pass *analysis.Pass, call *ast.CallExpr) types.Object {
	selector, _ := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return nil
	}
	return ps6124ExprObject(pass, selector.X)
}

func ps6124ArgsContain(pass *analysis.Pass, arguments []ast.Expr, object types.Object) bool {
	if object == nil {
		return false
	}
	for _, argument := range arguments {
		if ps6124ExprObject(pass, argument) == object {
			return true
		}
	}
	return false
}

func ps6124ObjectID(pass *analysis.Pass, expression ast.Expr) string {
	var object types.Object
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		object = pass.TypesInfo.ObjectOf(value)
	case *ast.SelectorExpr:
		object = pass.TypesInfo.ObjectOf(value.Sel)
	}
	if object == nil || object.Pkg() == nil {
		return ""
	}
	return object.Pkg().Path() + "." + object.Name()
}

func ps6124ScopedPin(pass *analysis.Pass, body *ast.BlockStmt, reachable []*ast.BlockStmt, pkg *ps6124Package, contract *config.ScopedBackendRoutingContract, construction token.Pos) bool {
	var snapshot types.Object
	snapshotIndex, setIndex, restoreIndex := -1, -1, -1
	snapshotPos, setPos, restorePos := token.NoPos, token.NoPos, token.NoPos
	writes := 0
	for index, statement := range body.List {
		switch value := statement.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) == 1 && len(value.Rhs) == 1 {
				if call, ok := ps2110Unparen(value.Rhs[0]).(*ast.CallExpr); ok {
					if callee := ps6071CalledFunction(pass, call); callee != nil && ps6090FunctionID(callee.Origin()) == contract.Preference {
						if id, ok := value.Lhs[0].(*ast.Ident); ok {
							snapshot = pass.TypesInfo.ObjectOf(id)
							snapshotIndex = index
							snapshotPos = value.Pos()
						}
					}
				}
			}
		case *ast.ExprStmt:
			if call, ok := value.X.(*ast.CallExpr); ok && ps6124SetCall(pass, call, contract.SetPreference) {
				writes++
				if len(call.Args) == 1 && ps6124ObjectID(pass, call.Args[0]) == contract.SelectedBackend {
					setIndex = index
					setPos = call.Pos()
				}
			}
		case *ast.DeferStmt:
			call := value.Call
			if ps6124SetCall(pass, call, contract.SetPreference) {
				writes++
				if len(call.Args) == 1 && call.Ellipsis != token.NoPos && ps6124ExprObject(pass, call.Args[0]) == snapshot {
					restoreIndex = index
					restorePos = call.Pos()
				}
			}
		}
	}
	extraWrites := 0
	unstable := false
	for _, reachableBody := range reachable {
		live := pkg.bodyFlow(reachableBody).live
		ps6071InspectOwnBody(reachableBody, func(node ast.Node) bool {
			value, ok := node.(*ast.CallExpr)
			if !ok || !live[value] {
				return true
			}
			callee := ps6071CalledFunction(pass, value)
			if callee == nil {
				return true
			}
			id := ps6090FunctionID(callee.Origin())
			if id == contract.SetPreference {
				if value.Pos() != setPos && value.Pos() != restorePos {
					extraWrites++
				}
			} else if pkg.functionWrites(callee, contract.SetPreference) || pkg.callArgumentsWrite(value, callee, contract.SetPreference) {
				unstable = true
			}
			if callee.Pkg() != nil && callee.Pkg().Path() == "testing" && callee.Name() == "Parallel" {
				unstable = true
			}
			return true
		})
	}
	flow := pkg.bodyFlow(body).flow
	ordered := ps6124CanReach(pass, flow, snapshotPos, setPos) && ps6124CanReach(pass, flow, setPos, restorePos)
	dominates := ps6124Dominates(pass, flow, snapshotPos, construction) && ps6124Dominates(pass, flow, setPos, construction) && ps6124Dominates(pass, flow, restorePos, construction)
	return snapshot != nil && ps6124SnapshotStable(pass, body, pkg.bodyFlow(body).parents, snapshot, restoreIndex) && writes == 2 && extraWrites == 0 && !unstable && snapshotIndex+1 == setIndex && restoreIndex == setIndex+1 && body.List[restoreIndex].End() < construction && ordered && dominates
}

func (pkg *ps6124Package) callArgumentsWrite(call *ast.CallExpr, callee *types.Func, writer string) bool {
	for _, index := range pkg.invokedFor(callee) {
		index += ps6124MethodExpressionOffset(pkg.pass, call)
		if index >= len(call.Args) {
			return true
		}
		literal, ok := ps2110Unparen(call.Args[index]).(*ast.FuncLit)
		if !ok {
			if id, named := ps2110Unparen(call.Args[index]).(*ast.Ident); named {
				if function, known := pkg.pass.TypesInfo.ObjectOf(id).(*types.Func); known {
					if pkg.functionWrites(function, writer) {
						return true
					}
					continue
				}
			}
			return true
		}
		routes := make(map[string]bool)
		writes := make(map[string]bool)
		edges := make(map[*types.Func]bool)
		for _, body := range pkg.invokedBodies(literal.Body, token.NoPos) {
			pkg.collectDirectEffects(body, nil, map[string]bool{writer: true}, routes, writes, edges)
		}
		if writes[writer] {
			return true
		}
		for edge := range edges {
			if pkg.functionWrites(edge, writer) {
				return true
			}
		}
	}
	return false
}

func ps6124Dominates(pass *analysis.Pass, flow *ps6122Flow, dominator, target token.Pos) bool {
	domBlock := ps6122BlockAt(pass, flow, dominator)
	targetBlock := ps6122BlockAt(pass, flow, target)
	if domBlock == nil || targetBlock == nil {
		return false
	}
	if domBlock == targetBlock {
		return dominator < target
	}
	if len(flow.graph.Blocks) == 0 {
		return false
	}
	entry := flow.graph.Blocks[0]
	if entry == domBlock {
		return true
	}
	seen := map[*cfg.Block]bool{}
	queue := []*cfg.Block{entry}
	for len(queue) != 0 {
		block := queue[0]
		queue = queue[1:]
		if block == domBlock || seen[block] {
			continue
		}
		if block == targetBlock {
			return false
		}
		seen[block] = true
		queue = append(queue, ps6122Successors(pass, flow, block, token.NoPos)...)
	}
	return true
}

func ps6124SnapshotStable(pass *analysis.Pass, body *ast.BlockStmt, parents map[ast.Node]ast.Node, snapshot types.Object, restoreIndex int) bool {
	if snapshot == nil || restoreIndex < 0 {
		return false
	}
	stable := true
	ast.Inspect(body, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[id] != snapshot {
			return true
		}
		parent := parents[id]
		if assignment, ok := parent.(*ast.AssignStmt); ok {
			for _, lhs := range assignment.Lhs {
				if lhs == id {
					return true
				}
			}
		}
		if call, ok := parent.(*ast.CallExpr); ok && call == body.List[restoreIndex].(*ast.DeferStmt).Call && len(call.Args) == 1 && call.Args[0] == id {
			return true
		}
		stable = false
		return false
	})
	return stable
}

func ps6124SetCall(pass *analysis.Pass, call *ast.CallExpr, id string) bool {
	callee := ps6071CalledFunction(pass, call)
	return callee != nil && ps6090FunctionID(callee.Origin()) == id
}

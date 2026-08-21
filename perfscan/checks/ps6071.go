package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6071 implements owner issue #788. It connects repeated synchronized
// registry reads, virtual capability probes over stable keys, route-context
// construction, and static preferred-backend re-entry across package
// boundaries. The object facts keep the cross-package half type-safe.
var PS6071 = register(&lint.Check{
	ID:       "PS6071",
	Category: "verify",
	Slug:     "repeated-immutable-backend-resolution",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a hot dispatcher repeatedly resolves an immutable backend, operation, and dtype route",
		Text: `A central execution choke point can spend a material share of every
small operation rediscovering a route that is stable after backend
initialization. A read lock, registry lookup, repeated virtual capability
probes, and construction of an immutable context are individually cheap; the
composition is not cheap when every tensor operation pays it.

This check implements owner issue #788. It reports a dispatcher only when the
same function contains the complete high-signal composition:

  - at least two calls to package accessors that protect package-owned backend
    registry state with sync.Mutex or sync.RWMutex;
  - at least two virtual Kernel/Resolve/Capability/Implementation/Supports
    probes on an interface using the same two key objects;
  - no assignment to either key from the first repeated probe onward; and
  - construction or derivation of a backend/device/recorder route context.

Registry accessors and affected dispatchers are propagated as typed analysis
facts, so aliases and cross-package selected implementations remain visible.
The check also reports a callable implementation factory that selects a
compile-time-named preferred backend through such a registry/capability helper
and directly re-enters the marked dispatcher. It requires the re-entry to be
guarded only by that static resolution result. Input-size/layout-dependent
routing, such as a nested measuredHostCandidate predicate, stays silent because
it cannot be hoisted into a route keyed only by backend/op/dtype.

A generation/version-aware cache read suppresses the dispatcher finding when
the function also names the route/kernel cache. A
//perfscan:backend-resolution-cache-validated annotation records an externally
reviewed equivalent. Same-named user lock methods, concrete capability calls,
one-off probes, mutable keys, lookup-only helpers, and cached dispatchers stay
silent.

Cache the resolved kernel and immutable execution context in a dense structure
indexed by operation and dtype, but invalidate every registry and preference
mutation. Share entries only for exact registered backend identity: equal
backend names are insufficient when wrappers can reuse a name. Dynamic per-op
routes must bypass the default cache. Preserve validation before resolution,
fallback order, and recorder/profiling at the outer dispatch boundary. A
statically CPU-preferred implementation should decline support so the outer
dispatcher chooses CPU once instead of recursively entering itself.

There is NO automatic fix. Perfscan cannot infer the complete mutation set,
backend identity contract, dynamic override semantics, or the required outer
recording/profiling boundary.`,
		Before: `func Execute(ctx *Context, op Op, in []*Tensor, attrs Attrs) ([]*Tensor, error) {
	dtype := in[0].Dtype()
	k, ok := ctx.Backend.Kernel(op, dtype)
	if !ok {
		cpu, _ := Get(CPU) // RWMutex + map lookup on every dispatch
		if _, has := cpu.Kernel(op, dtype); has { k, _ = cpu.Kernel(op, dtype) }
		ctx = ctx.WithBackend(cpu)
	}
	return k(ctx, in, attrs)
}`,
		After: `// Dense cache key: exact backend identity + op + dtype + generation.
// Miss path preserves validation and fallback order; registry/preference
// mutations advance generation. Dynamic per-op routes bypass this cache.
resolved := routeCache.Load(exactBackend, op, dtype, registryGeneration)
return resolved.kernel(resolved.context, in, attrs)`,
		MeasuredWin: `Final owner validation landed in GoAI PR #1122 (merged commit
e55bf65db97770231c882324654dc8e95eacd684; exact tested head
c4b6e066b7653d956793305ec5157f8de66518ba) with both 15/15 CI matrices green.
On Apple M2 Pro, the same-binary historical nested-dispatch control moved from
222.0 to 155.7 ns/op (1.426x). Three independent seven-process fallback
campaigns measured 1.378x, 1.417x, and 1.408x; every one of 21 samples exceeded
1.25x. Warm fallback allocation fell from 384 B/6 allocs to 336 B/5 allocs.
The retained design keys the immutable resolution cache by exact backend
identity plus registry generation and leaves unregistered same-name wrappers
on the live uncached path.`,
	},
	Analyzer: &analysis.Analyzer{
		Name:      "PS6071",
		Doc:       "repeated synchronized backend resolution and static preferred-backend dispatcher re-entry",
		Run:       runPS6071,
		FactTypes: []analysis.Fact{new(ps6071RegistryFact), new(ps6071DispatcherFact)},
	},
})

type ps6071RegistryFact struct{}

func (*ps6071RegistryFact) AFact()         {}
func (*ps6071RegistryFact) String() string { return "registry-accessor" }

type ps6071DispatcherFact struct{}

func (*ps6071DispatcherFact) AFact()         {}
func (*ps6071DispatcherFact) String() string { return "backend-dispatcher" }

type ps6071CapabilityGroup struct {
	name  string
	left  types.Object
	right types.Object
	calls []*ast.CallExpr
}

type ps6071DispatchFinding struct {
	registryReads int
	contexts      int
	capability    ps6071CapabilityGroup
}

type ps6071Package struct {
	pass         *analysis.Pass
	declarations map[*types.Func]*ast.FuncDecl
	registry     map[*types.Func]bool
	dispatchers  map[*types.Func]bool
	preferred    map[*types.Func]bool
}

func runPS6071(pass *analysis.Pass) (any, error) {
	pkg := &ps6071Package{
		pass:         pass,
		declarations: make(map[*types.Func]*ast.FuncDecl),
		registry:     make(map[*types.Func]bool),
		dispatchers:  make(map[*types.Func]bool),
		preferred:    make(map[*types.Func]bool),
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			if object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func); ok {
				pkg.declarations[object] = function
			}
		}
	}

	// Export the low-level synchronization facts first. Importers can then
	// recognize Get-like helpers without guessing from their spelling.
	for object, function := range pkg.declarations {
		if ps6071SynchronizedRegistryAccessor(pass, function) {
			pkg.registry[object] = true
			pass.ExportObjectFact(object, new(ps6071RegistryFact))
		}
	}

	for object, function := range pkg.declarations {
		finding, ok := pkg.dispatchFinding(function)
		if !ok {
			continue
		}
		pkg.dispatchers[object] = true
		pass.ExportObjectFact(object, new(ps6071DispatcherFact))
		if ps6071Validated(function) || ps6071GenerationCache(pass, function.Body) {
			continue
		}
		pass.Reportf(function.Name.Pos(), "%s performs %d synchronized registry reads, %d virtual %s probes over stable (%s, %s), and %d route-context derivations on each dispatch; cache the resolved kernel/context by exact backend identity + operation + dtype with generation invalidation, bypass dynamic per-op routes, and preserve validation plus recorder/profiling at the outer boundary (advisory, no automatic fix)",
			function.Name.Name,
			finding.registryReads,
			len(finding.capability.calls),
			finding.capability.name,
			finding.capability.left.Name(),
			finding.capability.right.Name(),
			finding.contexts,
		)
	}

	for object, function := range pkg.declarations {
		if pkg.preferredResolver(function) {
			pkg.preferred[object] = true
		}
	}
	for _, function := range pkg.declarations {
		if ps6071Validated(function) || !ps6071ReturnsCallable(pass, function) {
			continue
		}
		pkg.reportStaticReentry(function)
	}
	return nil, nil
}

func ps6071SynchronizedRegistryAccessor(pass *analysis.Pass, function *ast.FuncDecl) bool {
	hasLock := false
	hasSharedRead := false
	ps6071InspectOwnBody(function.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.CallExpr:
			if ps6071SyncLockCall(pass, value) {
				hasLock = true
			}
		case *ast.IndexExpr:
			if ps6071PackageMap(pass, value.X) {
				hasSharedRead = true
			}
		case *ast.RangeStmt:
			if ps6071PackageMap(pass, value.X) {
				hasSharedRead = true
			}
		case *ast.ReturnStmt:
			for _, result := range value.Results {
				if ps6071PackageVariable(pass, result) {
					hasSharedRead = true
				}
			}
		}
		return !(hasLock && hasSharedRead)
	})
	return hasLock && hasSharedRead
}

func ps6071SyncLockCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	function := ps6071CalledFunction(pass, call)
	if function == nil || function.Pkg() == nil || function.Pkg().Path() != "sync" {
		return false
	}
	switch function.Name() {
	case "Lock", "RLock", "Unlock", "RUnlock":
		return true
	default:
		return false
	}
}

func ps6071PackageMap(pass *analysis.Pass, expression ast.Expr) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return false
	}
	variable, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
	if !ok || variable.Parent() != pass.Pkg.Scope() {
		return false
	}
	_, ok = variable.Type().Underlying().(*types.Map)
	return ok
}

func ps6071PackageVariable(pass *analysis.Pass, expression ast.Expr) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return false
	}
	variable, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
	return ok && variable.Parent() == pass.Pkg.Scope()
}

func (pkg *ps6071Package) dispatchFinding(function *ast.FuncDecl) (ps6071DispatchFinding, bool) {
	var finding ps6071DispatchFinding
	if !ps6071DispatcherName(function.Name.Name) {
		return finding, false
	}
	groups := make(map[[2]types.Object]*ps6071CapabilityGroup)
	writes := ps6071Writes(pkg.pass, function.Body)
	ps6071InspectOwnBody(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if pkg.registryCall(call) {
			finding.registryReads++
		}
		if ps6071RouteContextCall(pkg.pass, call) {
			finding.contexts++
		}
		name, left, right, ok := ps6071CapabilityCall(pkg.pass, call)
		if !ok {
			return true
		}
		key := [2]types.Object{left, right}
		group := groups[key]
		if group == nil {
			group = &ps6071CapabilityGroup{name: name, left: left, right: right}
			groups[key] = group
		}
		group.calls = append(group.calls, call)
		return true
	})
	if finding.registryReads < 2 || finding.contexts == 0 {
		return finding, false
	}
	for _, group := range groups {
		if len(group.calls) < 2 || !ps6071StableFromFirstProbe(group, writes) {
			continue
		}
		if len(group.calls) > len(finding.capability.calls) {
			finding.capability = *group
		}
	}
	return finding, len(finding.capability.calls) >= 2
}

func ps6071DispatcherName(name string) bool {
	lower := strings.ToLower(name)
	for _, fragment := range []string{"execute", "dispatch", "invoke", "submit", "forward"} {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

func (pkg *ps6071Package) registryCall(call *ast.CallExpr) bool {
	function := ps6071CalledFunction(pkg.pass, call)
	if function == nil {
		return false
	}
	if pkg.registry[function] {
		return true
	}
	var fact ps6071RegistryFact
	return pkg.pass.ImportObjectFact(function, &fact)
}

func (pkg *ps6071Package) dispatcherCall(call *ast.CallExpr) *types.Func {
	function := ps6071CalledFunction(pkg.pass, call)
	if function == nil {
		return nil
	}
	if pkg.dispatchers[function] {
		return function
	}
	var fact ps6071DispatcherFact
	if pkg.pass.ImportObjectFact(function, &fact) {
		return function
	}
	return nil
}

func ps6071CapabilityCall(pass *analysis.Pass, call *ast.CallExpr) (string, types.Object, types.Object, bool) {
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || len(call.Args) < 2 {
		return "", nil, nil, false
	}
	selection := pass.TypesInfo.Selections[selector]
	method, _ := selectionObject(selection).(*types.Func)
	if selection == nil || !ps6071InterfaceType(selection.Recv()) || !ps6071CapabilityMethod(method) {
		return "", nil, nil, false
	}
	left := ps6071KeyObject(pass, call.Args[0])
	right := ps6071KeyObject(pass, call.Args[1])
	if left == nil || right == nil || left == right {
		return "", nil, nil, false
	}
	return method.Name(), left, right, true
}

func ps6071InterfaceType(value types.Type) bool {
	if value == nil {
		return false
	}
	_, ok := value.Underlying().(*types.Interface)
	return ok
}

func ps6071CapabilityMethod(object *types.Func) bool {
	if object == nil {
		return false
	}
	lower := strings.ToLower(object.Name())
	switch lower {
	case "kernel", "resolve", "capability", "implementation", "supports":
		return true
	}
	return strings.HasPrefix(lower, "supports") || strings.HasPrefix(lower, "canhandle")
}

func ps6071KeyObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok || identifier.Name == "_" {
		return nil
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	if object == nil {
		object = pass.TypesInfo.Defs[identifier]
	}
	return object
}

func ps6071Writes(pass *analysis.Pass, body *ast.BlockStmt) map[types.Object]token.Pos {
	writes := make(map[types.Object]token.Pos)
	ps6071InspectOwnBody(body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, target := range value.Lhs {
				if object := ps6071KeyObject(pass, target); object != nil && target.Pos() > writes[object] {
					writes[object] = target.Pos()
				}
			}
		case *ast.IncDecStmt:
			if object := ps6071KeyObject(pass, value.X); object != nil && value.Pos() > writes[object] {
				writes[object] = value.Pos()
			}
		}
		return true
	})
	return writes
}

func ps6071StableFromFirstProbe(group *ps6071CapabilityGroup, writes map[types.Object]token.Pos) bool {
	first := group.calls[0].Pos()
	for _, call := range group.calls[1:] {
		if call.Pos() < first {
			first = call.Pos()
		}
	}
	return writes[group.left] < first && writes[group.right] < first
}

func ps6071RouteContextCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	function := ps6071CalledFunction(pass, call)
	if function == nil {
		return false
	}
	lower := strings.ToLower(function.Name())
	switch lower {
	case "withbackend", "withdevice", "withrecorder", "withimplementation", "withkernel", "newcontext", "newroute":
		return true
	}
	return strings.HasPrefix(lower, "new") && (strings.Contains(lower, "context") || strings.Contains(lower, "route"))
}

func ps6071GenerationCache(pass *analysis.Pass, body *ast.BlockStmt) bool {
	hasCache := false
	hasGeneration := false
	hasLoad := false
	ps6071InspectOwnBody(body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.Ident:
			lower := strings.ToLower(value.Name)
			if strings.Contains(lower, "cache") && (strings.Contains(lower, "route") || strings.Contains(lower, "kernel") || strings.Contains(lower, "resolution")) {
				hasCache = true
			}
			if strings.Contains(lower, "generation") || strings.Contains(lower, "version") || strings.HasSuffix(lower, "gen") {
				hasGeneration = true
			}
		case *ast.CallExpr:
			function := ps6071CalledFunction(pass, value)
			if function != nil && function.Name() == "Load" && function.Pkg() != nil && function.Pkg().Path() == "sync/atomic" {
				hasLoad = true
			}
		}
		return !(hasCache && hasGeneration && hasLoad)
	})
	return hasCache && hasGeneration && hasLoad
}

func (pkg *ps6071Package) preferredResolver(function *ast.FuncDecl) bool {
	lower := strings.ToLower(function.Name.Name)
	if !strings.Contains(lower, "prefer") && !strings.Contains(lower, "preferred") && !strings.Contains(lower, "cpu") && !strings.Contains(lower, "host") {
		return false
	}
	hasStaticRegistryRead := false
	hasCapability := false
	ps6071InspectOwnBody(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if pkg.registryCall(call) && len(call.Args) > 0 && ps6071ConstantExpression(pkg.pass, call.Args[0]) {
			hasStaticRegistryRead = true
		}
		selector, selected := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
		if selected {
			selection := pkg.pass.TypesInfo.Selections[selector]
			method, _ := selectionObject(selection).(*types.Func)
			if selection != nil && ps6071InterfaceType(selection.Recv()) && ps6071CapabilityMethod(method) {
				hasCapability = true
			}
		}
		return !(hasStaticRegistryRead && hasCapability)
	})
	return hasStaticRegistryRead && hasCapability
}

func ps6071ConstantExpression(pass *analysis.Pass, expression ast.Expr) bool {
	value, ok := pass.TypesInfo.Types[expression]
	return ok && value.Value != nil && value.Value.Kind() != constant.Unknown
}

func ps6071ReturnsCallable(pass *analysis.Pass, function *ast.FuncDecl) bool {
	object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
	if !ok {
		return false
	}
	signature, ok := object.Type().(*types.Signature)
	if !ok {
		return false
	}
	for index := 0; index < signature.Results().Len(); index++ {
		if _, ok := signature.Results().At(index).Type().Underlying().(*types.Signature); ok {
			return true
		}
	}
	return false
}

func (pkg *ps6071Package) reportStaticReentry(function *ast.FuncDecl) {
	parents := ps6071Parents(function.Body)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		conditional, ok := node.(*ast.IfStmt)
		if !ok || ps6071DynamicallyEnclosed(conditional, parents) {
			return true
		}
		backend, resolver, ok := pkg.staticPreferredBinding(conditional)
		if !ok {
			return true
		}
		call, dispatcher := pkg.directDispatcherReentry(conditional.Body, backend)
		if call == nil {
			return true
		}
		pass := pkg.pass
		pass.Reportf(call.Pos(), "%s selects a compile-time preferred backend through %s and re-enters %s from the selected callable implementation; make that implementation decline the statically preferred operation/dtype so the outer dispatcher selects the backend once, while preserving validation and recorder/profiling at the outer boundary (advisory, no automatic fix)",
			function.Name.Name, resolver.Name(), dispatcher.Name())
		return true
	})
}

func (pkg *ps6071Package) staticPreferredBinding(conditional *ast.IfStmt) (types.Object, *types.Func, bool) {
	assignment, ok := conditional.Init.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) < 2 || len(assignment.Rhs) != 1 {
		return nil, nil, false
	}
	call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if !ok {
		return nil, nil, false
	}
	resolver := ps6071CalledFunction(pkg.pass, call)
	if resolver == nil || !pkg.preferred[resolver] {
		return nil, nil, false
	}
	backend := ps6071KeyObject(pkg.pass, assignment.Lhs[0])
	okObject := ps6071KeyObject(pkg.pass, assignment.Lhs[1])
	if backend == nil || okObject == nil || !ps6071ConditionUsesObject(pkg.pass, conditional.Cond, okObject) {
		return nil, nil, false
	}
	return backend, resolver, true
}

func ps6071ConditionUsesObject(pass *analysis.Pass, expression ast.Expr, target types.Object) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.ObjectOf(identifier) == target {
			found = true
			return false
		}
		return !found
	})
	return found
}

func (pkg *ps6071Package) directDispatcherReentry(body *ast.BlockStmt, backend types.Object) (*ast.CallExpr, *types.Func) {
	var found *ast.CallExpr
	var dispatcher *types.Func
	ast.Inspect(body, func(node ast.Node) bool {
		if found != nil {
			return false
		}
		if node != body {
			switch node.(type) {
			case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt, *ast.FuncLit:
				return false
			}
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee := pkg.dispatcherCall(call)
		if callee == nil || !ps6071CallRoutesBackend(pkg.pass, call, backend) {
			return true
		}
		found, dispatcher = call, callee
		return false
	})
	return found, dispatcher
}

func ps6071CallRoutesBackend(pass *analysis.Pass, dispatcher *ast.CallExpr, backend types.Object) bool {
	for _, argument := range dispatcher.Args {
		matched := false
		ast.Inspect(argument, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return !matched
			}
			function := ps6071CalledFunction(pass, call)
			if function != nil && function.Name() == "WithBackend" && len(call.Args) == 1 && ps6071KeyObject(pass, call.Args[0]) == backend {
				matched = true
				return false
			}
			return !matched
		})
		if matched {
			return true
		}
	}
	return false
}

func ps6071Parents(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) != 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return parents
}

func ps6071DynamicallyEnclosed(node ast.Node, parents map[ast.Node]ast.Node) bool {
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		switch parent.(type) {
		case *ast.FuncLit, *ast.FuncDecl:
			return false
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			return true
		}
	}
	return false
}

func ps6071CalledFunction(pass *analysis.Pass, call *ast.CallExpr) *types.Func {
	switch function := ps2110Unparen(call.Fun).(type) {
	case *ast.Ident:
		object, _ := pass.TypesInfo.ObjectOf(function).(*types.Func)
		return object
	case *ast.SelectorExpr:
		if selection := pass.TypesInfo.Selections[function]; selection != nil {
			object, _ := selection.Obj().(*types.Func)
			return object
		}
		object, _ := pass.TypesInfo.ObjectOf(function.Sel).(*types.Func)
		return object
	default:
		return nil
	}
}

func selectionObject(selection *types.Selection) types.Object {
	if selection == nil {
		return nil
	}
	return selection.Obj()
}

func ps6071InspectOwnBody(body *ast.BlockStmt, visit func(ast.Node) bool) {
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		return visit(node)
	})
}

func ps6071Validated(function *ast.FuncDecl) bool {
	if function.Doc == nil {
		return false
	}
	for _, comment := range function.Doc.List {
		if strings.Contains(comment.Text, "perfscan:backend-resolution-cache-validated") {
			return true
		}
	}
	return false
}

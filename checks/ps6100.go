package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/lint"
)

// PS6100 implements owner issue #901. It finds iterative loops that repeat
// full scans of the same collection, derive a small finite predicate state,
// and mutate the predicate inputs at only a few explicit element indexes.
var PS6100 = register(&lint.Check{
	ID:       "PS6100",
	Category: "cpu",
	Slug:     "iterative-finite-state-rescan",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "iterative full scans repeatedly derive finite per-element membership",
		Text: `Iterative solvers, active-set algorithms, eligibility passes, and
bucketed searches often scan the complete state more than once per step. When
each step changes only a few elements, recomputing the same small family of
boolean membership predicates reloads and branches over almost entirely
unchanged data.

PS6100 recognizes two or more canonical full scans nested in one iterative
loop. The scans must range over the same slice/array (or count from zero to its
length), their boolean conditions must read a shared per-element input, and the
outer iteration may contain at most four explicit writes to indexed elements
of those inputs. Direct and generic package-local single-result helpers are
followed through multiple call layers, so predicates expressed as
helper(slice, index) or helper(slice[index]) remain visible.
Stable slice, map, and pointer aliases are resolved by object identity across
direct, generic, and method helpers; branch-ambiguous helper aliases and
dynamic dispatch remain explicit proof obligations. Simultaneous alias
assignments retain Go's right-hand-side snapshot semantics, slice-view writes
retain their element offsets, and reachable stored closures invalidate aliases
whose captured headers they may rebind. Bounded local callback graphs and
stored function or method values contribute their captured receiver, argument,
and mutation effects. Per-iteration alias rebindings flow into captured
variables, and stored-call targets are resolved from the assignments that can
reach each call site; opaque, recursive, and overflowed targets remain hazards.
A deferred body is applied at the return of its actual frame, so an outer-frame
defer stays delayed while a defer inside an invoked helper or closure is visible
before control returns to the scans.

Only scans that can coexist on one control-flow path are combined. Mutually
exclusive branches, source-proven zero-trip loops (including constant-length
make expressions), and scans inside unreachable constant branches or switch
cases stay silent; fallthrough only makes a case live when its selected
predecessor can execute. An early break/return/goto,
reassignment or address escape of the scan index/range value, and collections
rebuilt inside the outer step likewise stay silent because they are not
canonical repeated full scans.

The diagnostic names the full scans, predicate inputs, bounded mutation sites,
estimated status bits, and every visible return/break/continue/goto path that a
refresh must cover. Scalar and configuration dependencies are named so they
can be proved invariant or trigger a full status rebuild. Passing a tracked
collection to an opaque call, taking an element address, or bulk-mutating the
collection is reported as an unsafe incremental-refresh hazard.
Address escape or rebinding of a tracked root/selector prefix is likewise
reported, including aliases established before the iterative loop.
Reference-backed inputs are also called out for an alias proof.
Predicates with unknown calls, recursion, or channel receives stay silent
because reducing their evaluation count could change observable behavior.

There is NO automatic fix. A correct refactor initializes a compact byte or
bitset once, refreshes exactly the changed indexes on every path, and preserves
scan order, tie handling, floating-point arithmetic, and termination. Moving a
refresh across a continue or hidden alias write silently changes membership,
so the analyzer only exposes the candidate and required proof obligations.`,
		Before: `for !converged {
    for i := range alpha { if isUpper(y[i], alpha[i], c) { /* ... */ } }
    for i := range alpha { if isLower(y[i], alpha[i], c) { /* ... */ } }
    alpha[left] = nextLeft
    alpha[right] = nextRight
}`,
		After: `status := make([]uint8, len(alpha))
for i := range status { status[i] = classify(y[i], alpha[i], c) }
for !converged {
    for i := range status { if status[i]&upperBit != 0 { /* ... */ } }
    for i := range status { if status[i]&lowerBit != 0 { /* ... */ } }
    alpha[left], alpha[right] = nextLeft, nextRight
    status[left], status[right] = classify(y[left], alpha[left], c), classify(y[right], alpha[right], c)
}`,
		MeasuredWin: `Owner issue #901 measured the exact two-bit status cache in
GoAI's SVC SMO loop. Fourteen balanced frozen-binary runs of 300 complete
4000x20 RBF fits improved the median from 6.702 ms to 6.230 ms (1.0757x), with
10/14 paired wins. The rewrite preserved 79 solver steps, 42 support vectors,
decision signs, and a 3.33e-15 scalar/SIMD decision delta; allocations remained
1038 per fit and status storage added about 4 KiB per fit.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6100",
		Doc:  "iterative full scans repeatedly derive finite per-element membership",
		Run:  runPS6100,
	},
})

const (
	ps6100MaxScansPerIteration     = 16
	ps6100MaxMutationsPerIteration = 4
	ps6100MaxPredicateBits         = 8
	ps6100MaxHelperDepth           = 32
	ps6100MaxCallableWork          = 1024
)

type ps6100Storage struct {
	key    string
	name   string
	typ    types.Type
	object types.Object
}

type ps6100Loop struct {
	node       ast.Node
	body       *ast.BlockStmt
	index      types.Object
	rangeValue types.Object
	bound      ps6100Storage
}

type ps6100Scan struct {
	loop       *ps6100Loop
	inputs     map[string]ps6100Storage
	invariants map[string]ps6100Storage
	predicates map[string]bool
}

type ps6100Mutation struct {
	storage  ps6100Storage
	index    string
	identity string
	node     ast.Node
}

type ps6100Hazard struct {
	node   ast.Node
	reason string
}

type ps6100AddressExposure struct {
	node    ast.Node
	storage ps6100Storage
}

type ps6100MutationFacts struct {
	mutations []ps6100Mutation
	hazards   []ps6100Hazard
}

type ps6100Helpers map[*types.Func]*ast.FuncDecl

type ps6100CallableTarget struct {
	function         *types.Func
	literal          *ast.FuncLit
	receiver         ast.Expr
	methodExpression bool
}

type ps6100CallableSource struct {
	expression ast.Expr
	target     *ps6100CallableTarget
	unknown    bool
}

type ps6100CallableBinding struct {
	sources []ps6100CallableSource
	unknown bool
}

type ps6100CallableState map[types.Object]ps6100CallableBinding

type ps6100CallableBindings struct {
	all     ps6100CallableState
	parents map[ast.Node]ast.Node
	before  map[ast.Node]ps6100CallableState
}

type ps6100CallableResolution struct {
	targets []ps6100CallableTarget
	unknown bool
}

func runPS6100(pass *analysis.Pass) (any, error) {
	helpers := ps6100LocalFunctions(pass)
	callables := ps6100LocalCallableBindings(pass)
	addresses := ps6100Addresses(pass)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ps6100Function(pass, function, helpers, callables, addresses)
		}
	}
	return nil, nil
}

func ps6100LocalFunctions(pass *analysis.Pass) ps6100Helpers {
	result := make(ps6100Helpers)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			if object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func); ok {
				result[object.Origin()] = function
			}
		}
	}
	return result
}

func ps6100LocalCallableBindings(pass *analysis.Pass) ps6100CallableBindings {
	result := ps6100CallableBindings{
		all: make(ps6100CallableState), parents: make(map[ast.Node]ast.Node), before: make(map[ast.Node]ps6100CallableState),
	}
	var inspectFrame func(*ast.BlockStmt)
	inspectFrame = func(body *ast.BlockStmt) {
		if body == nil {
			return
		}
		var nested []*ast.FuncLit
		hasCallableBinding := false
		ast.Inspect(body, func(node ast.Node) bool {
			if literal, ok := node.(*ast.FuncLit); ok {
				nested = append(nested, literal)
				return false
			}
			switch value := node.(type) {
			case *ast.AssignStmt:
				for _, left := range value.Lhs {
					identifier, ok := ps2110Unparen(left).(*ast.Ident)
					if ok && identifier.Name != "_" {
						object := ps6100AssignedObject(pass, identifier, value.Tok)
						hasCallableBinding = hasCallableBinding || object != nil && ps6100FunctionValueType(object.Type())
					}
				}
			case *ast.ValueSpec:
				for _, name := range value.Names {
					object := pass.TypesInfo.Defs[name]
					hasCallableBinding = hasCallableBinding || object != nil && ps6100FunctionValueType(object.Type())
				}
			}
			return true
		})
		for _, literal := range nested {
			inspectFrame(literal.Body)
		}
		if !hasCallableBinding {
			return
		}
		for child, parent := range ps6087Parents(body) {
			result.parents[child] = parent
		}
		aliasFlow := ps6100HelperAliasFlow(pass, body, nil)
		reachability := ps6100ReachableNodes(pass, body)
		recordAll := func(identifier *ast.Ident, assignment token.Token, source ast.Expr) {
			if identifier == nil || identifier.Name == "_" {
				return
			}
			object := ps6100AssignedObject(pass, identifier, assignment)
			if object == nil || !ps6100FunctionValueType(object.Type()) {
				return
			}
			binding := result.all[object]
			if source == nil {
				binding.unknown = true
				result.all[object] = binding
				return
			}
			callable := ps6100CallableSourceAt(pass, source, aliasFlow)
			if callable.unknown {
				binding.unknown = true
			}
			if len(binding.sources) >= ps6100MaxAliasValues {
				binding.unknown = true
			} else {
				binding.sources = append(binding.sources, callable)
			}
			result.all[object] = binding
		}
		ast.Inspect(body, func(node ast.Node) bool {
			if node == nil {
				return true
			}
			if node != body && !reachability.mayExecute(node) {
				return false
			}
			if _, ok := node.(*ast.FuncLit); ok {
				return false
			}
			switch value := node.(type) {
			case *ast.AssignStmt:
				for index, left := range value.Lhs {
					identifier, ok := ps2110Unparen(left).(*ast.Ident)
					if !ok {
						continue
					}
					var source ast.Expr
					if len(value.Lhs) == len(value.Rhs) {
						source = value.Rhs[index]
					}
					recordAll(identifier, value.Tok, source)
				}
			case *ast.ValueSpec:
				for index, name := range value.Names {
					var source ast.Expr
					if len(value.Names) == len(value.Values) {
						source = value.Values[index]
					}
					recordAll(name, token.DEFINE, source)
				}
			}
			return true
		})

		flow := ps6100CallableFlowForBody(pass, body, nil, aliasFlow)
		for node, state := range flow.before {
			result.before[node] = state
		}
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok {
				inspectFrame(function.Body)
			}
		}
	}
	return result
}

func ps6100TransferCallableNode(pass *analysis.Pass, node ast.Node, state ps6100CallableState, aliases *ps6100AliasFlow) {
	var transfers []struct {
		object  types.Object
		binding ps6100CallableBinding
	}
	record := func(object types.Object, source ast.Expr) {
		if object == nil || !ps6100FunctionValueType(object.Type()) {
			return
		}
		transfers = append(transfers, struct {
			object  types.Object
			binding ps6100CallableBinding
		}{object: object, binding: ps6100CallableBindingFromSource(pass, source, state, aliases)})
	}
	switch value := node.(type) {
	case *ast.AssignStmt:
		for index, left := range value.Lhs {
			identifier, ok := ps2110Unparen(left).(*ast.Ident)
			if !ok || identifier.Name == "_" {
				continue
			}
			var source ast.Expr
			if len(value.Lhs) == len(value.Rhs) {
				source = value.Rhs[index]
			}
			record(ps6100AssignedObject(pass, identifier, value.Tok), source)
		}
	case *ast.ValueSpec:
		for index, name := range value.Names {
			var source ast.Expr
			if len(value.Names) == len(value.Values) {
				source = value.Values[index]
			}
			record(pass.TypesInfo.Defs[name], source)
		}
	}
	// All right-hand sides are evaluated against the incoming state before any
	// left-hand side is assigned, preserving tuple-snapshot semantics.
	for _, transfer := range transfers {
		state[transfer.object] = transfer.binding
	}
}

func ps6100CallableBindingFromSource(pass *analysis.Pass, source ast.Expr, state ps6100CallableState, aliases *ps6100AliasFlow) ps6100CallableBinding {
	if source == nil {
		return ps6100CallableBinding{unknown: true}
	}
	callable := ps6100CallableSourceAt(pass, source, aliases)
	if callable.unknown {
		return ps6100CallableBinding{unknown: true}
	}
	if callable.target != nil {
		return ps6100CallableBinding{sources: []ps6100CallableSource{callable}}
	}
	if identifier, ok := ps2110Unparen(callable.expression).(*ast.Ident); ok {
		if binding, found := state[pass.TypesInfo.ObjectOf(identifier)]; found {
			return ps6100CloneCallableBinding(binding)
		}
	}
	return ps6100CallableBinding{sources: []ps6100CallableSource{callable}}
}

func ps6100CloneCallableBinding(binding ps6100CallableBinding) ps6100CallableBinding {
	return ps6100CallableBinding{sources: slices.Clone(binding.sources), unknown: binding.unknown}
}

func ps6100CloneCallableState(state ps6100CallableState) ps6100CallableState {
	result := make(ps6100CallableState, len(state))
	for object, binding := range state {
		result[object] = ps6100CloneCallableBinding(binding)
	}
	return result
}

func ps6100MergeCallableStates(current, incoming ps6100CallableState) (ps6100CallableState, bool) {
	if current == nil {
		return ps6100CloneCallableState(incoming), true
	}
	changed := false
	for object, binding := range incoming {
		merged, bindingChanged := ps6100MergeCallableBindings(current[object], binding)
		if bindingChanged {
			current[object] = merged
			changed = true
		}
	}
	return current, changed
}

func ps6100MergeCallableBindings(current, incoming ps6100CallableBinding) (ps6100CallableBinding, bool) {
	changed := false
	if incoming.unknown && !current.unknown {
		current.unknown = true
		changed = true
	}
	seen := make(map[string]bool, len(current.sources))
	for _, source := range current.sources {
		seen[ps6100CallableSourceKey(source)] = true
	}
	for _, source := range incoming.sources {
		key := ps6100CallableSourceKey(source)
		if seen[key] {
			continue
		}
		if len(current.sources) >= ps6100MaxAliasValues {
			if !current.unknown {
				current.unknown = true
				changed = true
			}
			continue
		}
		seen[key] = true
		current.sources = append(current.sources, source)
		changed = true
	}
	return current, changed
}

func ps6100CallableSourceKey(source ps6100CallableSource) string {
	if source.target != nil {
		return ps6100CallableTargetKey(*source.target)
	}
	if source.expression != nil {
		return "expression:" + strconv.Itoa(int(source.expression.Pos())) + ":" + exprTextRendered(source.expression)
	}
	if source.unknown {
		return "unknown"
	}
	return "empty"
}

func (bindings ps6100CallableBindings) stateAt(node ast.Node) ps6100CallableState {
	for current := node; current != nil; current = bindings.parents[current] {
		if state, found := bindings.before[current]; found {
			return state
		}
	}
	return nil
}

func (bindings ps6100CallableBindings) bindingAt(object types.Object, node ast.Node) (ps6100CallableBinding, bool) {
	if binding, found := bindings.stateAt(node)[object]; found {
		return binding, true
	}
	binding, found := bindings.all[object]
	return binding, found
}

func ps6100CallableFlowForBody(pass *analysis.Pass, body *ast.BlockStmt, seed ps6100CallableState, aliases *ps6100AliasFlow) ps6100CallableBindings {
	result := ps6100CallableBindings{parents: ps6087Parents(body), before: make(map[ast.Node]ps6100CallableState)}
	if body == nil {
		return result
	}
	graph := cfg.New(body, func(call *ast.CallExpr) bool { return ps6100CallMayReturn(pass, call) })
	if len(graph.Blocks) == 0 {
		return result
	}
	reachability := ps6100ReachableNodes(pass, body)
	inputs := map[*cfg.Block]ps6100CallableState{graph.Blocks[0]: ps6100CloneCallableState(seed)}
	queue := []*cfg.Block{graph.Blocks[0]}
	queued := map[*cfg.Block]bool{graph.Blocks[0]: true}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		queued[block] = false
		state := ps6100CloneCallableState(inputs[block])
		for _, node := range block.Nodes {
			if !reachability.mayExecute(node) {
				continue
			}
			result.before[node], _ = ps6100MergeCallableStates(result.before[node], state)
			ps6100TransferCallableNode(pass, node, state, aliases)
		}
		for _, successor := range ps6100CFGSuccessors(pass, result.parents, block) {
			merged, changed := ps6100MergeCallableStates(inputs[successor], state)
			inputs[successor] = merged
			if changed && !queued[successor] {
				queue = append(queue, successor)
				queued[successor] = true
			}
		}
	}
	return result
}

func ps6100CallableSourceAt(pass *analysis.Pass, expression ast.Expr, flow *ps6100AliasFlow) ps6100CallableSource {
	expression = ps2110Unparen(expression)
	if expression == nil {
		return ps6100CallableSource{unknown: true}
	}
	if call, ok := expression.(*ast.CallExpr); ok && len(call.Args) == 1 && pass.TypesInfo.Types[ps2110Unparen(call.Fun)].IsType() && ps6100FunctionValueType(pass.TypesInfo.TypeOf(call)) {
		return ps6100CallableSourceAt(pass, call.Args[0], flow)
	}
	if literal, ok := expression.(*ast.FuncLit); ok {
		return ps6100CallableSource{target: &ps6100CallableTarget{literal: literal}}
	}
	if function, _, ok := typedCallee(pass, expression); ok {
		target := ps6100CallableTarget{function: function.Origin()}
		callable := expression
		switch value := ps2110Unparen(callable).(type) {
		case *ast.IndexExpr:
			callable = ps2110Unparen(value.X)
		case *ast.IndexListExpr:
			callable = ps2110Unparen(value.X)
		}
		if selector, ok := ps2110Unparen(callable).(*ast.SelectorExpr); ok {
			if selection := pass.TypesInfo.Selections[selector]; selection != nil {
				switch selection.Kind() {
				case types.MethodVal:
					receiver, precise := ps6100CapturedCallableReceiver(pass, selector.X, flow, expression)
					if !precise {
						return ps6100CallableSource{unknown: true}
					}
					target.receiver = receiver
				case types.MethodExpr:
					target.methodExpression = true
				}
			}
		}
		return ps6100CallableSource{target: &target}
	}
	if ps6100FunctionValueType(pass.TypesInfo.TypeOf(expression)) {
		return ps6100CallableSource{expression: expression}
	}
	return ps6100CallableSource{unknown: true}
}

func ps6100CapturedCallableReceiver(pass *analysis.Pass, receiver ast.Expr, flow *ps6100AliasFlow, node ast.Node) (ast.Expr, bool) {
	receiver = ps2110Unparen(receiver)
	if flow == nil {
		return receiver, receiver != nil
	}
	identifier, ok := receiver.(*ast.Ident)
	if !ok {
		return ps6100ResolveExpression(pass, receiver, flow.environmentAt(node), make(map[types.Object]bool)), true
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	if values, found := flow.stateAt(node)[object]; found {
		if values.unknown || len(values.expressions) != 1 {
			return nil, false
		}
		for _, expression := range values.expressions {
			if expression == nil {
				return nil, false
			}
			return expression, true
		}
	}
	return receiver, true
}

func ps6100ResolveCallables(pass *analysis.Pass, expression ast.Expr, environment map[types.Object]ast.Expr, bindings ps6100CallableBindings, state ps6100CallableState) ps6100CallableResolution {
	site := expression
	work := ps6100MaxCallableWork
	result := ps6100CallableResolution{}
	seenTargets := make(map[string]bool)
	var resolve func(ast.Expr, map[types.Object]bool)
	add := func(target ps6100CallableTarget) {
		key := ps6100CallableTargetKey(target)
		if seenTargets[key] {
			return
		}
		if len(result.targets) >= ps6100MaxAliasValues {
			result.unknown = true
			return
		}
		seenTargets[key] = true
		result.targets = append(result.targets, target)
	}
	resolve = func(source ast.Expr, seen map[types.Object]bool) {
		if work == 0 {
			result.unknown = true
			return
		}
		work--
		source = ps2110Unparen(source)
		if source == nil {
			result.unknown = true
			return
		}
		if identifier, ok := source.(*ast.Ident); ok {
			object := pass.TypesInfo.ObjectOf(identifier)
			if actual := environment[object]; actual != nil {
				if seen[object] {
					result.unknown = true
					return
				}
				seen[object] = true
				resolve(actual, seen)
				delete(seen, object)
				return
			}
		}
		callable := ps6100CallableSourceAt(pass, source, nil)
		if callable.target != nil {
			add(*callable.target)
			return
		}
		if callable.unknown {
			result.unknown = true
			return
		}
		identifier, ok := ps2110Unparen(callable.expression).(*ast.Ident)
		if !ok {
			result.unknown = true
			return
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		if object == nil || seen[object] {
			result.unknown = true
			return
		}
		binding, found := state[object]
		if !found {
			binding, found = bindings.bindingAt(object, site)
		}
		if !found {
			result.unknown = true
			return
		}
		result.unknown = result.unknown || binding.unknown
		seen[object] = true
		for _, next := range binding.sources {
			result.unknown = result.unknown || next.unknown
			switch {
			case next.target != nil:
				add(*next.target)
			case next.expression != nil:
				resolve(next.expression, seen)
			default:
				result.unknown = true
			}
		}
		delete(seen, object)
	}
	resolve(expression, make(map[types.Object]bool))
	slices.SortFunc(result.targets, func(left, right ps6100CallableTarget) int {
		return strings.Compare(ps6100CallableTargetKey(left), ps6100CallableTargetKey(right))
	})
	return result
}

func ps6100CallableTargetKey(target ps6100CallableTarget) string {
	if target.literal != nil {
		return "literal:" + strconv.Itoa(int(target.literal.Pos()))
	}
	key := "function:" + ps6100ObjectKey(target.function)
	if target.methodExpression {
		return key + ":expression"
	}
	if target.receiver != nil {
		return key + ":value:" + strconv.Itoa(int(target.receiver.Pos())) + ":" + exprTextRendered(target.receiver)
	}
	return key
}

func ps6100CallableActiveKey(target ps6100CallableTarget) string {
	if target.literal != nil {
		return "literal:" + strconv.Itoa(int(target.literal.Pos()))
	}
	return "function:" + ps6100ObjectKey(target.function)
}

func ps6100CallableInvocation(
	pass *analysis.Pass,
	target ps6100CallableTarget,
	call *ast.CallExpr,
	helpers ps6100Helpers,
	environment map[types.Object]ast.Expr,
) (*ast.BlockStmt, map[types.Object]ast.Expr, bool) {
	if call == nil {
		return nil, nil, false
	}
	var body *ast.BlockStmt
	var signature *types.Signature
	if target.literal != nil {
		body = target.literal.Body
		if typ := pass.TypesInfo.TypeOf(target.literal); typ != nil {
			signature, _ = types.Unalias(typ).Underlying().(*types.Signature)
		}
	} else if target.function != nil {
		declaration := helpers[target.function.Origin()]
		if declaration == nil {
			return nil, nil, false
		}
		body = declaration.Body
		signature, _ = target.function.Type().(*types.Signature)
	}
	if body == nil || signature == nil || signature.Variadic() {
		return nil, nil, false
	}
	arguments := call.Args
	next := make(map[types.Object]ast.Expr, len(environment)+len(arguments)+1)
	for object, expression := range environment {
		next[object] = expression
	}
	if signature.Recv() != nil {
		var receiver ast.Expr
		switch {
		case target.methodExpression && len(arguments) != 0:
			receiver, arguments = arguments[0], arguments[1:]
		case target.receiver != nil:
			receiver = target.receiver
		}
		if receiver == nil {
			return nil, nil, false
		}
		next[signature.Recv()] = ps6100ResolveExpression(pass, receiver, environment, make(map[types.Object]bool))
	}
	if len(arguments) != signature.Params().Len() {
		return nil, nil, false
	}
	for index, argument := range arguments {
		next[signature.Params().At(index)] = ps6100ResolveExpression(pass, argument, environment, make(map[types.Object]bool))
	}
	return body, next, true
}

func ps6100Function(pass *analysis.Pass, function *ast.FuncDecl, helpers ps6100Helpers, callables ps6100CallableBindings, addresses []ps6100AddressExposure) {
	parents := ps6087Parents(function.Body)
	aliases := ps6100StableAliases(pass, function.Body, helpers, callables, addresses)
	var ordered []*ps6100Loop
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if literal, ok := node.(*ast.FuncLit); ok && literal != nil {
			return false
		}
		if loop := ps6100CanonicalScan(pass, node, addresses, aliases); loop != nil {
			ordered = append(ordered, loop)
		}
		return true
	})
	if len(ordered) < 2 {
		return
	}

	byOuter := make(map[ast.Node][]ps6100Scan)
	for _, loop := range ordered {
		// The enclosing iterative loop may be live while this particular scan
		// sits in a source-proven dead branch, switch case, or zero-trip loop.
		// The CFG package deliberately does not fold those inner constructs, so
		// reject them before control-flow compatibility grouping.
		if !ps6100NodeMayExecute(pass, loop.node, parents) {
			continue
		}
		scan := ps6100ScanPredicates(pass, loop, helpers, aliases)
		if len(scan.inputs) == 0 || len(scan.predicates) == 0 {
			continue
		}
		for parent := parents[loop.node]; parent != nil; parent = parents[parent] {
			switch parent.(type) {
			case *ast.ForStmt, *ast.RangeStmt:
				byOuter[parent] = append(byOuter[parent], scan)
				parent = nil
			}
			if parent == nil {
				break
			}
		}
	}

	if len(byOuter) == 0 {
		return
	}
	aliasFlow := ps6100HelperAliasFlow(pass, function.Body, nil)
	liveOuter := ps6100LiveLoopBodies(pass, function.Body)
	for outerNode, scans := range byOuter {
		if len(scans) < 2 {
			continue
		}
		if reachable, known := liveOuter[outerNode]; (known && !reachable) || !ps6100NodeMayExecute(pass, outerNode, parents) {
			continue
		}
		outerBody := ps6100LoopBody(outerNode)
		if outerBody == nil {
			continue
		}
		ps6100Outer(pass, outerNode, outerBody, scans, helpers, callables, addresses, aliases, aliasFlow.stateWithin(outerBody))
	}
}

func ps6100NodeMayExecute(pass *analysis.Pass, node ast.Node, parents map[ast.Node]ast.Node) bool {
	child := node
	for parent := parents[child]; parent != nil; child, parent = parent, parents[parent] {
		switch value := parent.(type) {
		case *ast.BinaryExpr:
			if child != value.Y || value.Op != token.LAND && value.Op != token.LOR {
				continue
			}
			left := ps6100Constant(pass, value.X)
			if left == nil || left.Kind() != constant.Bool {
				continue
			}
			truth := constant.BoolVal(left)
			if value.Op == token.LAND && !truth || value.Op == token.LOR && truth {
				return false
			}
		case *ast.IfStmt:
			condition := ps6100Constant(pass, value.Cond)
			if condition == nil || condition.Kind() != constant.Bool {
				continue
			}
			truth := constant.BoolVal(condition)
			if (child == value.Body && !truth) || (child == value.Else && truth) {
				return false
			}
		case *ast.ForStmt:
			if child == value.Body && !ps6100LoopMayExecute(pass, value) {
				return false
			}
		case *ast.RangeStmt:
			if child == value.Body && !ps6100LoopMayExecute(pass, value) {
				return false
			}
		case *ast.CaseClause:
			if !ps6100CaseMayExecute(pass, value, parents) {
				return false
			}
		}
	}
	return true
}

func ps6100CaseMayExecute(pass *analysis.Pass, clause *ast.CaseClause, parents map[ast.Node]ast.Node) bool {
	if clause == nil {
		return true
	}
	body, ok := parents[clause].(*ast.BlockStmt)
	if !ok {
		return true
	}
	switchStatement, ok := parents[body].(*ast.SwitchStmt)
	if !ok {
		return true
	}
	selected, known := ps6100SelectedSwitchCase(pass, switchStatement)
	if !known {
		return true
	}
	target := slices.Index(body.List, ast.Stmt(clause))
	if selected < 0 || target < selected {
		return false
	}
	for index := selected; index <= target; index++ {
		if index == target {
			return true
		}
		current, ok := body.List[index].(*ast.CaseClause)
		if !ok || !ps6100ClauseFallsThrough(pass, current) {
			return false
		}
	}
	return false
}

func ps6100SelectedSwitchCase(pass *analysis.Pass, statement *ast.SwitchStmt) (int, bool) {
	if statement == nil || statement.Body == nil {
		return -1, false
	}
	tag := constant.MakeBool(true)
	if statement.Tag != nil {
		tag = ps6100Constant(pass, statement.Tag)
		if tag == nil {
			return -1, false
		}
	}
	defaultIndex := -1
	for index, item := range statement.Body.List {
		clause, ok := item.(*ast.CaseClause)
		if !ok {
			return -1, false
		}
		if len(clause.List) == 0 {
			defaultIndex = index
			continue
		}
		for _, expression := range clause.List {
			value := ps6100Constant(pass, expression)
			if value == nil {
				return -1, false
			}
			if constant.Compare(tag, token.EQL, value) {
				return index, true
			}
		}
	}
	return defaultIndex, true
}

func ps6100ClauseFallsThrough(pass *analysis.Pass, clause *ast.CaseClause) bool {
	if clause == nil {
		return false
	}
	for index := len(clause.Body) - 1; index >= 0; index-- {
		if _, empty := clause.Body[index].(*ast.EmptyStmt); empty {
			continue
		}
		branch, ok := clause.Body[index].(*ast.BranchStmt)
		return ok && branch.Tok == token.FALLTHROUGH && ps6100StatementsMayReachEnd(pass, clause.Body[:index])
	}
	return false
}

// ps6100StatementsMayReachEnd proves only source-level terminators. Returning
// true on an unfamiliar statement is deliberate: a case is propagated through
// fallthrough whenever the transfer may execute, and rejected only when its
// prefix is source-proven unable to reach the final fallthrough statement.
func ps6100StatementsMayReachEnd(pass *analysis.Pass, statements []ast.Stmt) bool {
	for _, statement := range statements {
		if !ps6100StatementMayReachEnd(pass, statement) {
			return false
		}
	}
	return true
}

func ps6100StatementMayReachEnd(pass *analysis.Pass, statement ast.Stmt) bool {
	switch value := statement.(type) {
	case *ast.ReturnStmt, *ast.BranchStmt:
		return false
	case *ast.ExprStmt:
		call, ok := ps2110Unparen(value.X).(*ast.CallExpr)
		return !ok || ps6100CallMayReturn(pass, call)
	case *ast.BlockStmt:
		return ps6100StatementsMayReachEnd(pass, value.List)
	case *ast.LabeledStmt:
		return ps6100StatementMayReachEnd(pass, value.Stmt)
	case *ast.IfStmt:
		condition := ps6100Constant(pass, value.Cond)
		if condition != nil && condition.Kind() == constant.Bool {
			if constant.BoolVal(condition) {
				return ps6100StatementsMayReachEnd(pass, value.Body.List)
			}
			if value.Else == nil {
				return true
			}
			return ps6100StatementMayReachEnd(pass, value.Else)
		}
		// An unknown branch reaches its successor when either arm can do so.
		if value.Else == nil || ps6100StatementsMayReachEnd(pass, value.Body.List) {
			return true
		}
		return ps6100StatementMayReachEnd(pass, value.Else)
	}
	return true
}

type ps6100Reachability struct {
	pass    *analysis.Pass
	parents map[ast.Node]ast.Node
	known   map[ast.Node]bool
	live    map[ast.Node]bool
}

// ps6100ReachableNodes augments x/tools/cfg liveness with source constants for
// if and for conditions. The CFG deliberately keeps both constant arms live;
// selecting their real edge here lets callers also reject statements after a
// source-proven terminator without building a second bespoke control walker.
func ps6100ReachableNodes(pass *analysis.Pass, body *ast.BlockStmt) *ps6100Reachability {
	result := &ps6100Reachability{
		pass: pass, parents: ps6087Parents(body), known: make(map[ast.Node]bool), live: make(map[ast.Node]bool),
	}
	if body == nil {
		return result
	}
	graph := cfg.New(body, func(call *ast.CallExpr) bool { return ps6100CallMayReturn(pass, call) })
	for _, block := range graph.Blocks {
		for _, node := range block.Nodes {
			result.known[node] = true
		}
	}
	if len(graph.Blocks) == 0 {
		return result
	}
	queue := []*cfg.Block{graph.Blocks[0]}
	seen := make(map[*cfg.Block]bool, len(graph.Blocks))
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == nil || seen[block] {
			continue
		}
		seen[block] = true
		for _, node := range block.Nodes {
			result.live[node] = true
		}
		successors := ps6100CFGSuccessors(pass, result.parents, block)
		queue = append(queue, successors...)
	}
	return result
}

func ps6100CFGSuccessors(pass *analysis.Pass, parents map[ast.Node]ast.Node, block *cfg.Block) []*cfg.Block {
	if block == nil {
		return nil
	}
	successors := block.Succs
	if len(successors) != 2 || len(block.Nodes) == 0 {
		return successors
	}
	condition, ok := block.Nodes[len(block.Nodes)-1].(ast.Expr)
	if !ok || !ps6100CFGCondition(parents, condition) {
		return successors
	}
	value := ps6100Constant(pass, condition)
	if value == nil || value.Kind() != constant.Bool {
		return successors
	}
	if constant.BoolVal(value) {
		return successors[:1]
	}
	return successors[1:]
}

func ps6100CFGCondition(parents map[ast.Node]ast.Node, expression ast.Expr) bool {
	for child, parent := ast.Node(expression), parents[expression]; parent != nil; child, parent = parent, parents[parent] {
		switch statement := parent.(type) {
		case *ast.IfStmt:
			return statement.Cond == child
		case *ast.ForStmt:
			return statement.Cond == child
		case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.CaseClause:
			return false
		}
	}
	return false
}

func ps6100CallMayReturn(pass *analysis.Pass, call *ast.CallExpr) bool {
	return !typedBuiltinName(pass, call.Fun, "panic")
}

func (reachability *ps6100Reachability) mayExecute(node ast.Node) bool {
	if reachability == nil || node == nil {
		return true
	}
	if !ps6100NodeMayExecute(reachability.pass, node, reachability.parents) {
		return false
	}
	for current := node; current != nil; current = reachability.parents[current] {
		if reachability.known[current] {
			return reachability.live[current]
		}
	}
	return true
}

func ps6100LiveLoopBodies(pass *analysis.Pass, body *ast.BlockStmt) map[ast.Node]bool {
	graph := cfg.New(body, func(call *ast.CallExpr) bool { return ps6100CallMayReturn(pass, call) })
	live := make(map[ast.Node]bool, len(graph.Blocks))
	for _, block := range graph.Blocks {
		if block.Stmt == nil {
			continue
		}
		switch block.Kind {
		case cfg.KindForBody, cfg.KindRangeBody:
			live[block.Stmt] = live[block.Stmt] || block.Live
		}
	}
	return live
}

func ps6100CanonicalScan(pass *analysis.Pass, node ast.Node, addresses []ps6100AddressExposure, aliases map[types.Object]ast.Expr) *ps6100Loop {
	switch loop := node.(type) {
	case *ast.RangeStmt:
		if loop.Body == nil {
			return nil
		}
		var index types.Object
		if loop.Key != nil {
			identifier, ok := ps2110Unparen(loop.Key).(*ast.Ident)
			if !ok {
				return nil
			}
			if identifier.Name != "_" {
				index = ps6100AssignedObject(pass, identifier, loop.Tok)
				if index == nil {
					return nil
				}
			}
		}
		bound, ok := ps6100StorageOf(pass, loop.X, aliases)
		if !ok || !ps6100Sequence(bound.typ) {
			return nil
		}
		var rangeValue types.Object
		if value, ok := ps2110Unparen(loop.Value).(*ast.Ident); ok && value.Name != "_" {
			rangeValue = ps6100AssignedObject(pass, value, loop.Tok)
		}
		if index == nil && rangeValue == nil {
			return nil
		}
		if !ps6100LoopVariableStable(pass, loop.Body, index, addresses) || !ps6100LoopVariableStable(pass, loop.Body, rangeValue, addresses) || !ps6100ScanTraversalSafe(pass, loop.Body) {
			return nil
		}
		return &ps6100Loop{node: loop, body: loop.Body, index: index, rangeValue: rangeValue, bound: bound}
	case *ast.ForStmt:
		if loop.Body == nil {
			return nil
		}
		initializer, ok := loop.Init.(*ast.AssignStmt)
		if !ok || len(initializer.Lhs) != 1 || len(initializer.Rhs) != 1 || !ps6100Zero(pass, initializer.Rhs[0]) {
			return nil
		}
		identifier, ok := ps2110Unparen(initializer.Lhs[0]).(*ast.Ident)
		if !ok || identifier.Name == "_" {
			return nil
		}
		index := ps6100AssignedObject(pass, identifier, initializer.Tok)
		condition, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
		if index == nil || !ok || condition.Op != token.LSS || !ps6100Object(pass, condition.X, index) {
			return nil
		}
		length, ok := ps2110Unparen(condition.Y).(*ast.CallExpr)
		if !ok || len(length.Args) != 1 || !typedBuiltinName(pass, length.Fun, "len") {
			return nil
		}
		bound, ok := ps6100StorageOf(pass, length.Args[0], aliases)
		if !ok || !ps6100Sequence(bound.typ) || !ps6100UnitIncrement(pass, loop.Post, index) {
			return nil
		}
		if !ps6100LoopVariableStable(pass, loop.Body, index, addresses) || !ps6100ScanTraversalSafe(pass, loop.Body) {
			return nil
		}
		return &ps6100Loop{node: loop, body: loop.Body, index: index, bound: bound}
	}
	return nil
}

func ps6100LoopVariableStable(pass *analysis.Pass, body ast.Node, object types.Object, addresses []ps6100AddressExposure) bool {
	if object == nil {
		return true
	}
	block, _ := body.(*ast.BlockStmt)
	reachability := ps6100ReachableNodes(pass, block)
	stable := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !stable {
			return false
		}
		if node != body && !reachability.mayExecute(node) {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				if ps6100Object(pass, left, object) {
					stable = false
					return false
				}
			}
		case *ast.IncDecStmt:
			if ps6100Object(pass, value.X, object) {
				stable = false
				return false
			}
		}
		return true
	})
	if !stable {
		return false
	}
	// An address captured anywhere can be written indirectly in the scan body,
	// which defeats canonical traversal.
	for _, address := range addresses {
		if address.storage.object == object {
			return false
		}
	}
	return true
}

func ps6100ScanTraversalSafe(pass *analysis.Pass, body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}
	parents := ps6087Parents(body)
	reachability := ps6100ReachableNodes(pass, body)
	labels := make(map[string]ast.Stmt)
	ast.Inspect(body, func(node ast.Node) bool {
		if node != body && !reachability.mayExecute(node) {
			return false
		}
		if literal, ok := node.(*ast.FuncLit); ok && literal != nil {
			return false
		}
		if labeled, ok := node.(*ast.LabeledStmt); ok {
			labels[labeled.Label.Name] = labeled.Stmt
		}
		return true
	})
	safe := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !safe {
			return false
		}
		if node != body && !reachability.mayExecute(node) {
			return false
		}
		if literal, ok := node.(*ast.FuncLit); ok && literal != nil {
			return false
		}
		switch statement := node.(type) {
		case *ast.CallExpr:
			if !ps6100CallMayReturn(pass, statement) {
				safe = false
				return false
			}
		case *ast.ReturnStmt:
			safe = false
			return false
		case *ast.BranchStmt:
			switch statement.Tok {
			case token.GOTO:
				safe = false
				return false
			case token.BREAK:
				if statement.Label != nil {
					target := labels[statement.Label.Name]
					switch target.(type) {
					case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
					default:
						safe = false
					}
					return safe
				}
				if !ps6100HasEnclosingBreakTarget(statement, body, parents) {
					safe = false
					return false
				}
			case token.CONTINUE:
				if statement.Label != nil {
					switch labels[statement.Label.Name].(type) {
					case *ast.ForStmt, *ast.RangeStmt:
					default:
						safe = false
					}
					return safe
				}
			}
		}
		return true
	})
	return safe
}

func ps6100HasEnclosingBreakTarget(node ast.Node, body *ast.BlockStmt, parents map[ast.Node]ast.Node) bool {
	for node = parents[node]; node != nil && node != body; node = parents[node] {
		switch node.(type) {
		case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			return true
		}
	}
	return false
}

func ps6100AssignedObject(pass *analysis.Pass, identifier *ast.Ident, assignment token.Token) types.Object {
	if identifier == nil {
		return nil
	}
	if assignment == token.DEFINE {
		if object := pass.TypesInfo.Defs[identifier]; object != nil {
			return object
		}
	}
	return pass.TypesInfo.Uses[identifier]
}

func ps6100Zero(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[expression].Value
	return value != nil && value.Kind() == constant.Int && constant.Sign(value) == 0
}

func ps6100UnitIncrement(pass *analysis.Pass, statement ast.Stmt, index types.Object) bool {
	switch value := statement.(type) {
	case *ast.IncDecStmt:
		return value.Tok == token.INC && ps6100Object(pass, value.X, index)
	case *ast.AssignStmt:
		return value.Tok == token.ADD_ASSIGN && len(value.Lhs) == 1 && len(value.Rhs) == 1 &&
			ps6100Object(pass, value.Lhs[0], index) && ps6100One(pass, value.Rhs[0])
	}
	return false
}

func ps6100One(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[expression].Value
	return value != nil && constant.Compare(value, token.EQL, constant.MakeInt64(1))
}

func ps6100Object(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && pass.TypesInfo.ObjectOf(identifier) == object
}

func ps6100Sequence(typ types.Type) bool {
	if typ == nil {
		return false
	}
	switch types.Unalias(typ).Underlying().(type) {
	case *types.Array, *types.Slice:
		return true
	}
	return false
}

func ps6100LoopBody(node ast.Node) *ast.BlockStmt {
	switch loop := node.(type) {
	case *ast.ForStmt:
		return loop.Body
	case *ast.RangeStmt:
		return loop.Body
	}
	return nil
}

func ps6100ScanPredicates(pass *analysis.Pass, loop *ps6100Loop, helpers ps6100Helpers, aliases map[types.Object]ast.Expr) ps6100Scan {
	scan := ps6100Scan{
		loop: loop, inputs: make(map[string]ps6100Storage),
		invariants: make(map[string]ps6100Storage), predicates: make(map[string]bool),
	}
	reachability := ps6100ReachableNodes(pass, loop.body)
	ast.Inspect(loop.body, func(node ast.Node) bool {
		if node != loop.body {
			switch node.(type) {
			case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
				return false
			}
		}
		statement, ok := node.(*ast.IfStmt)
		if !ok || statement.Cond == nil {
			return true
		}
		if !reachability.mayExecute(statement) || !reachability.mayExecute(statement.Cond) {
			return false
		}
		inputs := make(map[string]ps6100Storage)
		invariants := make(map[string]ps6100Storage)
		indexes := []types.Object{loop.index, loop.rangeValue}
		if !ps6100PredicateInputs(pass, statement.Cond, indexes, helpers, aliases, make(map[*types.Func]bool), make(map[*ast.CallExpr]bool), inputs, invariants, 0) {
			return true
		}
		if loop.rangeValue != nil && ps6100Mentions(pass, statement.Cond, loop.rangeValue, aliases, make(map[types.Object]bool)) {
			inputs[loop.bound.key] = loop.bound
		}
		if len(inputs) == 0 {
			return true
		}
		for key, input := range inputs {
			scan.inputs[key] = input
			delete(invariants, key)
		}
		for key, invariant := range invariants {
			scan.invariants[key] = invariant
		}
		scan.predicates[ps6100PredicateShape(pass, statement.Cond, loop.index, loop.rangeValue)] = true
		return true
	})
	return scan
}

func ps6100PredicateInputs(
	pass *analysis.Pass,
	expression ast.Expr,
	indexes []types.Object,
	helpers ps6100Helpers,
	environment map[types.Object]ast.Expr,
	active map[*types.Func]bool,
	evaluated map[*ast.CallExpr]bool,
	inputs map[string]ps6100Storage,
	invariants map[string]ps6100Storage,
	depth int,
) bool {
	if expression == nil || depth >= ps6100MaxHelperDepth {
		return expression == nil
	}
	expression = ps6100ResolveExpression(pass, expression, environment, make(map[types.Object]bool))
	safe := true
	parents := ps6087Parents(expression)
	ast.Inspect(expression, func(node ast.Node) bool {
		if !safe {
			return false
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.Ident:
			if selector, ok := parents[value].(*ast.SelectorExpr); ok && selector.Sel == value {
				return true
			}
			resolved := ps6100ResolveExpression(pass, value, environment, make(map[types.Object]bool))
			if identifier, ok := resolved.(*ast.Ident); ok && ps6100IsTarget(pass.TypesInfo.ObjectOf(identifier), indexes) {
				return true
			}
			storage, ok := ps6100StorageOf(pass, resolved, nil)
			if _, variable := pass.TypesInfo.ObjectOf(value).(*types.Var); variable && ok {
				invariants[storage.key] = storage
			}
		case *ast.IndexExpr:
			if ps6100MentionsAny(pass, value.Index, indexes, environment) {
				if storage, ok := ps6100StorageOf(pass, value.X, environment); ok && ps6100Sequence(storage.typ) {
					inputs[storage.key] = storage
				}
			}
		case *ast.UnaryExpr:
			if value.Op == token.ARROW {
				safe = false
				return false
			}
		case *ast.CallExpr:
			function, signature, ok := typedCallee(pass, value.Fun)
			if !ok || signature.Recv() != nil || signature.Variadic() || len(value.Args) != signature.Params().Len() {
				if !ps6100SafePredicateCall(pass, value) {
					safe = false
					return false
				}
				return true
			}
			function = function.Origin()
			if active[function] {
				if evaluated[value] {
					return false
				}
				safe = false
				return false
			}
			declaration := helpers[function]
			if declaration == nil || len(declaration.Body.List) != 1 {
				safe = false
				return false
			}
			returned, ok := declaration.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(returned.Results) != 1 || signature.Results().Len() != 1 {
				safe = false
				return false
			}
			for _, argument := range value.Args {
				if !ps6100PredicateInputs(pass, argument, indexes, helpers, environment, active, evaluated, inputs, invariants, depth+1) {
					safe = false
					return false
				}
			}
			next := make(map[types.Object]ast.Expr, len(environment)+len(value.Args))
			for object, actual := range environment {
				next[object] = actual
			}
			for position, argument := range value.Args {
				next[signature.Params().At(position)] = ps6100ResolveExpression(pass, argument, environment, make(map[types.Object]bool))
			}
			active[function] = true
			safe = ps6100PredicateInputs(pass, returned.Results[0], indexes, helpers, next, active, evaluated, inputs, invariants, depth+1)
			delete(active, function)
			if safe {
				evaluated[value] = true
			}
			return false
		}
		return true
	})
	return safe
}

func ps6100MentionsAny(pass *analysis.Pass, expression ast.Expr, targets []types.Object, environment map[types.Object]ast.Expr) bool {
	for _, target := range targets {
		if target != nil && ps6100Mentions(pass, expression, target, environment, make(map[types.Object]bool)) {
			return true
		}
	}
	return false
}

func ps6100SafePredicateCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	if pass.TypesInfo.Types[ps2110Unparen(call.Fun)].IsType() {
		return true
	}
	for _, name := range []string{"len", "cap", "min", "max"} {
		if typedBuiltinName(pass, call.Fun, name) {
			return true
		}
	}
	return false
}

func ps6100ResolveExpression(pass *analysis.Pass, expression ast.Expr, environment map[types.Object]ast.Expr, seen map[types.Object]bool) ast.Expr {
	for expression != nil {
		expression = ps2110Unparen(expression)
		identifier, ok := expression.(*ast.Ident)
		if !ok || environment == nil {
			return expression
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		actual := environment[object]
		if actual == nil || seen[object] {
			return expression
		}
		seen[object] = true
		expression = actual
	}
	return nil
}

func ps6100Mentions(pass *analysis.Pass, expression ast.Expr, target types.Object, environment map[types.Object]ast.Expr, seen map[types.Object]bool) bool {
	if expression == nil || target == nil {
		return false
	}
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		if object == target {
			found = true
			return false
		}
		if actual := environment[object]; actual != nil && !seen[object] {
			seen[object] = true
			found = ps6100Mentions(pass, actual, target, environment, seen)
			delete(seen, object)
		}
		return !found
	})
	return found
}

func ps6100StorageOf(pass *analysis.Pass, expression ast.Expr, environment map[types.Object]ast.Expr) (ps6100Storage, bool) {
	return ps6100StorageOfSeen(pass, expression, environment, make(map[types.Object]bool))
}

func ps6100StorageOfSeen(pass *analysis.Pass, expression ast.Expr, environment map[types.Object]ast.Expr, seen map[types.Object]bool) (ps6100Storage, bool) {
	expression = ps6100ResolveExpression(pass, expression, environment, seen)
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		if object == nil {
			return ps6100Storage{}, false
		}
		if seen[object] && environment[object] != nil {
			// Invocation environments may contain recursive slice-view or tuple
			// substitutions. A cycle is not a concrete storage identity.
			return ps6100Storage{}, false
		}
		return ps6100Storage{key: ps6100ObjectKey(object), name: value.Name, typ: object.Type(), object: object}, true
	case *ast.SelectorExpr:
		base, ok := ps6100StorageOfSeen(pass, value.X, environment, seen)
		if !ok {
			return ps6100Storage{}, false
		}
		object := pass.TypesInfo.Uses[value.Sel]
		if object == nil {
			return ps6100Storage{}, false
		}
		return ps6100Storage{
			key: base.key + "/" + ps6100ObjectKey(object), name: exprTextRendered(value),
			typ: pass.TypesInfo.TypeOf(value), object: base.object,
		}, true
	case *ast.IndexExpr:
		return ps6100StorageOfSeen(pass, value.X, environment, seen)
	case *ast.SliceExpr:
		return ps6100StorageOfSeen(pass, value.X, environment, seen)
	case *ast.StarExpr:
		return ps6100StorageOfSeen(pass, value.X, environment, seen)
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return ps6100StorageOfSeen(pass, value.X, environment, seen)
		}
	}
	return ps6100Storage{}, false
}

func ps6100ObjectKey(object types.Object) string {
	if object == nil {
		return ""
	}
	path := "local"
	if object.Pkg() != nil {
		path = object.Pkg().Path()
	}
	return path + ":" + strconv.Itoa(int(object.Pos())) + ":" + object.Name()
}

func ps6100PredicateShape(pass *analysis.Pass, expression ast.Expr, indexes ...types.Object) string {
	var result strings.Builder
	ast.Inspect(expression, func(node ast.Node) bool {
		if node == nil {
			result.WriteByte(')')
			return true
		}
		fmt.Fprintf(&result, "(%T", node)
		switch value := node.(type) {
		case *ast.Ident:
			if ps6100IsTarget(pass.TypesInfo.ObjectOf(value), indexes) {
				result.WriteString(":$index")
			} else if object := pass.TypesInfo.ObjectOf(value); object != nil {
				result.WriteByte(':')
				result.WriteString(ps6100ObjectKey(object))
			}
		case *ast.BasicLit:
			result.WriteByte(':')
			result.WriteString(value.Value)
		case *ast.BinaryExpr:
			result.WriteByte(':')
			result.WriteString(value.Op.String())
		case *ast.UnaryExpr:
			result.WriteByte(':')
			result.WriteString(value.Op.String())
		}
		return true
	})
	return result.String()
}

func ps6100IsTarget(object types.Object, targets []types.Object) bool {
	for _, target := range targets {
		if target != nil && object == target {
			return true
		}
	}
	return false
}

// ps6100CompatibleScans returns the largest source-ordered group of scans that
// can execute on the same path through the outer iteration. Scans in opposite
// arms of one if, switch, type switch, or select are alternatives rather than
// repeated work and must not be combined into one cache candidate.
func ps6100CompatibleScans(scans []ps6100Scan, blocks map[ast.Node][]*cfg.Block) []ps6100Scan {
	if len(scans) < 2 {
		return scans
	}
	var compatiblePairs [ps6100MaxScansPerIteration][ps6100MaxScansPerIteration]bool
	for first := range scans {
		compatiblePairs[first][first] = true
		for second := 0; second < first; second++ {
			compatible := ps6100BlocksReach(blocks[scans[first].loop.node], blocks[scans[second].loop.node]) ||
				ps6100BlocksReach(blocks[scans[second].loop.node], blocks[scans[first].loop.node])
			compatiblePairs[first][second] = compatible
			compatiblePairs[second][first] = compatible
		}
	}
	best := make([]int, 0, len(scans))
	selected := make([]int, 0, len(scans))
	var search func(int)
	search = func(next int) {
		if len(selected)+len(scans)-next <= len(best) {
			return
		}
		if next == len(scans) {
			best = append(best[:0], selected...)
			return
		}
		compatible := true
		for _, existing := range selected {
			if !compatiblePairs[existing][next] {
				compatible = false
				break
			}
		}
		if compatible {
			selected = append(selected, next)
			search(next + 1)
			selected = selected[:len(selected)-1]
		}
		search(next + 1)
	}
	search(0)
	result := make([]ps6100Scan, 0, len(best))
	for _, index := range best {
		result = append(result, scans[index])
	}
	return result
}

func ps6100BlocksReach(from, to []*cfg.Block) bool {
	if len(from) == 0 || len(to) == 0 {
		return false
	}
	seen := make(map[*cfg.Block]bool)
	queue := slices.Clone(from)
	for len(queue) != 0 {
		block := queue[0]
		queue = queue[1:]
		if seen[block] {
			continue
		}
		seen[block] = true
		for _, target := range to {
			if block == target {
				return true
			}
		}
		queue = append(queue, block.Succs...)
	}
	return false
}

func ps6100Outer(pass *analysis.Pass, outer ast.Node, body *ast.BlockStmt, scans []ps6100Scan, helpers ps6100Helpers, callables ps6100CallableBindings, addresses []ps6100AddressExposure, aliases map[types.Object]ast.Expr, aliasState ps6100AliasState) {
	if !ps6100LoopMayExecute(pass, outer) {
		return
	}
	graph := cfg.New(body, func(call *ast.CallExpr) bool { return ps6100CallMayReturn(pass, call) })
	blocks := make(map[ast.Node][]*cfg.Block)
	for _, block := range graph.Blocks {
		if block.Live && block.Stmt != nil {
			blocks[block.Stmt] = append(blocks[block.Stmt], block)
		}
	}
	byBound := make(map[string][]ps6100Scan)
	for _, scan := range scans {
		if len(byBound[scan.loop.bound.key]) < ps6100MaxScansPerIteration {
			byBound[scan.loop.bound.key] = append(byBound[scan.loop.bound.key], scan)
		}
	}
	for _, sameBound := range byBound {
		sameBound = ps6100CompatibleScans(sameBound, blocks)
		if len(sameBound) < 2 {
			continue
		}
		boundObject := sameBound[0].loop.bound.object
		if boundObject != nil && boundObject.Pos() > outer.Pos() && boundObject.Pos() < outer.End() {
			// A collection created inside the outer iteration cannot have its
			// status initialized once and incrementally maintained across steps.
			continue
		}
		allInputs := make(map[string]ps6100Storage)
		for _, scan := range sameBound {
			for key, input := range scan.inputs {
				allInputs[key] = input
			}
		}
		skip := make(map[ast.Node]bool, len(sameBound))
		for _, scan := range sameBound {
			skip[scan.loop.node] = true
		}
		callableWork := ps6100MaxCallableWork
		facts := ps6100CollectMutationFacts(pass, body, helpers, callables, allInputs, nil, aliases, aliasState, nil, make(map[string]bool), &callableWork, 0, false)
		facts.hazards = append(facts.hazards, ps6100AddressHazards(addresses, allInputs)...)
		mutated := make(map[string]bool, len(facts.mutations))
		for _, mutation := range facts.mutations {
			mutated[mutation.storage.key] = true
		}
		if len(mutated) == 0 {
			continue
		}
		selected := make([]ps6100Scan, 0, len(sameBound))
		for _, scan := range sameBound {
			for key := range scan.inputs {
				if mutated[key] {
					selected = append(selected, scan)
					break
				}
			}
		}
		if len(selected) < 2 {
			continue
		}
		mutations := ps6100RelevantMutations(facts.mutations, selected)
		if len(mutations) == 0 || len(mutations) > ps6100MaxMutationsPerIteration {
			continue
		}
		predicates := make(map[string]bool)
		inputs := make(map[string]ps6100Storage)
		invariants := make(map[string]ps6100Storage)
		for _, scan := range selected {
			for predicate := range scan.predicates {
				predicates[predicate] = true
			}
			for key, input := range scan.inputs {
				inputs[key] = input
			}
			for key, invariant := range scan.invariants {
				invariants[key] = invariant
			}
		}
		if len(predicates) == 0 || len(predicates) > ps6100MaxPredicateBits {
			continue
		}
		ps6100Report(pass, outer, selected, inputs, invariants, predicates, mutations, facts.hazards, ps6100Exits(pass, body, skip))
	}
}

// ps6100LoopMayExecute rejects only loops whose first condition or range
// cardinality is source-proven empty. A dead outer loop cannot repeatedly
// recompute membership, and treating its body as one abstract iteration would
// manufacture a performance candidate from unreachable work.
func ps6100LoopMayExecute(pass *analysis.Pass, node ast.Node) bool {
	switch loop := node.(type) {
	case *ast.ForStmt:
		if value := ps6100Constant(pass, loop.Cond); value != nil && value.Kind() == constant.Bool {
			return constant.BoolVal(value)
		}
		initializer, ok := loop.Init.(*ast.AssignStmt)
		if !ok || len(initializer.Lhs) != 1 || len(initializer.Rhs) != 1 {
			return true
		}
		identifier, ok := ps2110Unparen(initializer.Lhs[0]).(*ast.Ident)
		condition, okCondition := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
		if !ok || !okCondition {
			return true
		}
		object := ps6100AssignedObject(pass, identifier, initializer.Tok)
		initial := ps6100Constant(pass, initializer.Rhs[0])
		if object == nil || initial == nil {
			return true
		}
		left, right := ps6100Constant(pass, condition.X), ps6100Constant(pass, condition.Y)
		if ps6100Object(pass, condition.X, object) {
			left = initial
		} else if ps6100Object(pass, condition.Y, object) {
			right = initial
		} else {
			return true
		}
		if left == nil || right == nil {
			return true
		}
		switch condition.Op {
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			return constant.Compare(left, condition.Op, right)
		}
	case *ast.RangeStmt:
		if executable, known := ps6100RangeMayExecute(pass, loop.X); known {
			return executable
		}
	}
	return true
}

func ps6100RangeMayExecute(pass *analysis.Pass, expression ast.Expr) (bool, bool) {
	expression = ps2110Unparen(expression)
	if expression == nil {
		return false, false
	}
	if value := ps6100Constant(pass, expression); value != nil {
		switch value.Kind() {
		case constant.String:
			return len(constant.StringVal(value)) != 0, true
		case constant.Int:
			return constant.Sign(value) > 0, true
		}
	}
	typ := pass.TypesInfo.TypeOf(expression)
	if typ == nil {
		return false, false
	}
	underlying := types.Unalias(typ).Underlying()
	if pointer, ok := underlying.(*types.Pointer); ok {
		underlying = types.Unalias(pointer.Elem()).Underlying()
	}
	if array, ok := underlying.(*types.Array); ok {
		return array.Len() != 0, true
	}
	if literal, ok := expression.(*ast.CompositeLit); ok {
		switch underlying.(type) {
		case *types.Slice, *types.Map:
			return len(literal.Elts) != 0, true
		}
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return false, false
	}
	if typedBuiltinName(pass, call.Fun, "make") {
		switch underlying.(type) {
		case *types.Slice:
			if len(call.Args) < 2 {
				return false, false
			}
			length := ps6100Constant(pass, call.Args[1])
			if length == nil || length.Kind() != constant.Int {
				return false, false
			}
			return constant.Sign(length) > 0, true
		case *types.Map, *types.Chan:
			// A fresh map has no entries, and a fresh channel has no sender.
			// Their size arguments are still evaluated before the range begins.
			return false, true
		}
		return false, false
	}
	if len(call.Args) != 1 || !pass.TypesInfo.Types[ps2110Unparen(call.Fun)].IsType() {
		return false, false
	}
	argument := ps2110Unparen(call.Args[0])
	if identifier, ok := argument.(*ast.Ident); ok && identifier.Name == "nil" {
		switch underlying.(type) {
		case *types.Slice, *types.Map, *types.Chan, *types.Pointer:
			return false, true
		}
	}
	if !ps6100ConversionPreservesRangeEmptiness(pass.TypesInfo.TypeOf(argument), typ) {
		return false, false
	}
	return ps6100RangeMayExecute(pass, argument)
}

func ps6100ConversionPreservesRangeEmptiness(source, target types.Type) bool {
	if source == nil || target == nil {
		return false
	}
	source = types.Unalias(source).Underlying()
	target = types.Unalias(target).Underlying()
	if basic, ok := target.(*types.Basic); ok && basic.Kind() == types.String {
		switch source.(type) {
		case *types.Slice:
			return true
		}
		return ps6100StringType(source)
	}
	switch target.(type) {
	case *types.Slice:
		switch source.(type) {
		case *types.Slice, *types.Array:
			return true
		case *types.Pointer:
			_, ok := types.Unalias(source.(*types.Pointer).Elem()).Underlying().(*types.Array)
			return ok
		}
		return ps6100StringType(source)
	}
	return false
}

func ps6100StringType(typ types.Type) bool {
	basic, ok := types.Unalias(typ).Underlying().(*types.Basic)
	return ok && basic.Kind() == types.String
}

func ps6100Constant(pass *analysis.Pass, expression ast.Expr) constant.Value {
	if expression == nil {
		return nil
	}
	expression = ps2110Unparen(expression)
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		return value
	}
	if value, known := ps6100BooleanCondition(pass, expression); known {
		return constant.MakeBool(value)
	}
	return nil
}

func ps6100BooleanCondition(pass *analysis.Pass, expression ast.Expr) (bool, bool) {
	expression = ps2110Unparen(expression)
	if expression == nil {
		return false, false
	}
	if value := pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Bool {
		return constant.BoolVal(value), true
	}
	switch value := expression.(type) {
	case *ast.UnaryExpr:
		if value.Op == token.NOT {
			result, known := ps6100BooleanCondition(pass, value.X)
			return !result, known
		}
	case *ast.BinaryExpr:
		left, leftKnown := ps6100BooleanCondition(pass, value.X)
		right, rightKnown := ps6100BooleanCondition(pass, value.Y)
		switch value.Op {
		case token.LAND:
			if leftKnown && !left || rightKnown && !right {
				return false, true
			}
			if leftKnown && rightKnown {
				return left && right, true
			}
		case token.LOR:
			if leftKnown && left || rightKnown && right {
				return true, true
			}
			if leftKnown && rightKnown {
				return left || right, true
			}
		}
	}
	return false, false
}

func ps6100RelevantMutations(mutations []ps6100Mutation, scans []ps6100Scan) []ps6100Mutation {
	relevant := make(map[string]bool)
	for _, scan := range scans {
		for key := range scan.inputs {
			relevant[key] = true
		}
	}
	seen := make(map[string]bool)
	var result []ps6100Mutation
	for _, mutation := range mutations {
		identity := mutation.storage.key + "|" + mutation.identity + "|" + strconv.Itoa(int(mutation.node.Pos()))
		if relevant[mutation.storage.key] && !seen[identity] {
			seen[identity] = true
			result = append(result, mutation)
		}
	}
	return result
}

const ps6100MaxAliasValues = 8

type ps6100AliasValues struct {
	expressions map[string]ast.Expr
	unknown     bool
}

type ps6100AliasState map[types.Object]ps6100AliasValues

type ps6100AliasTransfer struct {
	object types.Object
	values ps6100AliasValues
}

type ps6100AliasFlow struct {
	base    map[types.Object]ast.Expr
	parents map[ast.Node]ast.Node
	before  map[ast.Node]ps6100AliasState
}

func ps6100HelperAliasFlow(pass *analysis.Pass, body *ast.BlockStmt, environment map[types.Object]ast.Expr) *ps6100AliasFlow {
	return ps6100AliasFlowWithState(pass, body, environment, nil)
}

func ps6100AliasFlowWithState(pass *analysis.Pass, body *ast.BlockStmt, environment map[types.Object]ast.Expr, seed ps6100AliasState) *ps6100AliasFlow {
	result := &ps6100AliasFlow{
		base: environment, parents: ps6087Parents(body), before: make(map[ast.Node]ps6100AliasState),
	}
	if body == nil {
		return result
	}
	initial := ps6100CloneAliasState(seed)
	for object, expression := range environment {
		if _, found := initial[object]; !found && ps6100ReferenceAliasType(object.Type()) {
			initial[object] = ps6100AliasExpressionValues(pass, expression, nil, environment, make(map[types.Object]bool))
		}
	}
	graph := cfg.New(body, func(call *ast.CallExpr) bool { return ps6100CallMayReturn(pass, call) })
	if len(graph.Blocks) == 0 {
		return result
	}
	reachability := ps6100ReachableNodes(pass, body)
	inputs := map[*cfg.Block]ps6100AliasState{graph.Blocks[0]: initial}
	queue := []*cfg.Block{graph.Blocks[0]}
	queued := map[*cfg.Block]bool{graph.Blocks[0]: true}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		queued[block] = false
		state := ps6100CloneAliasState(inputs[block])
		for _, node := range block.Nodes {
			if !reachability.mayExecute(node) {
				continue
			}
			result.before[node], _ = ps6100MergeAliasStates(result.before[node], state)
			ps6100TransferAliasNode(pass, node, state, environment)
		}
		for _, successor := range ps6100CFGSuccessors(pass, result.parents, block) {
			merged, changed := ps6100MergeAliasStates(inputs[successor], state)
			inputs[successor] = merged
			if changed && !queued[successor] {
				queue = append(queue, successor)
				queued[successor] = true
			}
		}
	}
	return result
}

func ps6100TransferAliasNode(pass *analysis.Pass, node ast.Node, state ps6100AliasState, base map[types.Object]ast.Expr) {
	switch value := node.(type) {
	case *ast.AssignStmt:
		transfers := make([]ps6100AliasTransfer, 0, len(value.Lhs))
		for index, left := range value.Lhs {
			identifier, ok := ps2110Unparen(left).(*ast.Ident)
			if !ok || identifier.Name == "_" {
				continue
			}
			object := ps6100AssignedObject(pass, identifier, value.Tok)
			if object == nil || !ps6100ReferenceAliasType(object.Type()) {
				continue
			}
			if len(value.Lhs) != len(value.Rhs) {
				transfers = append(transfers, ps6100AliasTransfer{object: object, values: ps6100AliasValues{unknown: true}})
				continue
			}
			// Go evaluates every right-hand side before assigning any left-hand
			// side. Resolve the complete tuple against the incoming snapshot so
			// swaps such as a, b = b, a do not lose either alias.
			transfers = append(transfers, ps6100AliasTransfer{
				object: object,
				values: ps6100AliasExpressionValues(pass, value.Rhs[index], state, base, make(map[types.Object]bool)),
			})
		}
		for _, transfer := range transfers {
			state[transfer.object] = transfer.values
		}
	case *ast.ValueSpec:
		transfers := make([]ps6100AliasTransfer, 0, len(value.Names))
		for index, name := range value.Names {
			object := pass.TypesInfo.Defs[name]
			if object == nil || !ps6100ReferenceAliasType(object.Type()) {
				continue
			}
			if len(value.Names) == len(value.Values) {
				transfers = append(transfers, ps6100AliasTransfer{
					object: object,
					values: ps6100AliasExpressionValues(pass, value.Values[index], state, base, make(map[types.Object]bool)),
				})
			} else if len(value.Values) == 0 {
				transfers = append(transfers, ps6100AliasTransfer{object: object, values: ps6100AliasValues{expressions: map[string]ast.Expr{"nil": nil}}})
			} else {
				transfers = append(transfers, ps6100AliasTransfer{object: object, values: ps6100AliasValues{unknown: true}})
			}
		}
		for _, transfer := range transfers {
			state[transfer.object] = transfer.values
		}
	}
}

func ps6100AliasExpressionValues(pass *analysis.Pass, expression ast.Expr, state ps6100AliasState, base map[types.Object]ast.Expr, seen map[types.Object]bool) ps6100AliasValues {
	expression = ps2110Unparen(expression)
	if expression == nil {
		return ps6100AliasValues{unknown: true}
	}
	switch value := expression.(type) {
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		if object != nil && !seen[object] {
			seen[object] = true
			if values, ok := state[object]; ok {
				return ps6100CloneAliasValues(values)
			}
			if actual := base[object]; actual != nil {
				return ps6100AliasExpressionValues(pass, actual, state, base, seen)
			}
		}
	case *ast.SliceExpr:
		underlying := ps6100AliasExpressionValues(pass, value.X, state, base, seen)
		if underlying.unknown || len(underlying.expressions) != 1 {
			// A view of multiple possible slice headers needs a relational
			// state to retain the offset for each base. Keep it explicitly
			// ambiguous instead of collapsing it to one root and inventing an
			// element index later.
			return ps6100AliasValues{unknown: true}
		}
		var resolvedBase ast.Expr
		for _, resolvedBase = range underlying.expressions {
		}
		if resolvedBase == nil {
			return ps6100AliasValues{unknown: true}
		}
		// Materialize the incoming base in the view. This is essential for
		// self-reslicing assignments such as alias = alias[1:]: retaining the
		// source identifier would make the new abstract value recursively
		// refer to itself after the transfer.
		view := &ast.SliceExpr{
			X:      resolvedBase,
			Lbrack: value.Lbrack,
			Low:    value.Low,
			High:   value.High,
			Max:    value.Max,
			Slice3: value.Slice3,
			Rbrack: value.Rbrack,
		}
		key := ps6100AliasExpressionKey(pass, view, base)
		return ps6100AliasValues{expressions: map[string]ast.Expr{key: view}}
	case *ast.CallExpr:
		if len(value.Args) == 1 && pass.TypesInfo.Types[ps2110Unparen(value.Fun)].IsType() && ps6100ReferenceAliasType(pass.TypesInfo.TypeOf(value)) {
			return ps6100AliasExpressionValues(pass, value.Args[0], state, base, seen)
		}
		if ps6100ReferenceAliasType(pass.TypesInfo.TypeOf(value)) && !typedBuiltinName(pass, value.Fun, "make") && !typedBuiltinName(pass, value.Fun, "new") {
			return ps6100AliasValues{unknown: true}
		}
	}
	key := ps6100AliasExpressionKey(pass, expression, base)
	return ps6100AliasValues{expressions: map[string]ast.Expr{key: expression}}
}

func ps6100AliasExpressionKey(pass *analysis.Pass, expression ast.Expr, environment map[types.Object]ast.Expr) string {
	if expression == nil {
		return "nil"
	}
	if identifier, ok := ps2110Unparen(expression).(*ast.Ident); ok && identifier.Name == "nil" {
		return "nil"
	}
	if slice, ok := ps2110Unparen(expression).(*ast.SliceExpr); ok {
		// Slice views share backing storage but not element zero. Retain the
		// view as a distinct abstract value so branch joins cannot silently
		// merge alpha and alpha[1:].
		return "slice:" + strconv.Itoa(int(slice.Lbrack)) + ":" + exprTextRendered(slice)
	}
	if storage, ok := ps6100StorageOf(pass, expression, environment); ok {
		return "storage:" + storage.key
	}
	return "expression:" + strconv.Itoa(int(expression.Pos())) + ":" + exprTextRendered(expression)
}

func ps6100CloneAliasValues(values ps6100AliasValues) ps6100AliasValues {
	result := ps6100AliasValues{unknown: values.unknown}
	if len(values.expressions) != 0 {
		result.expressions = make(map[string]ast.Expr, len(values.expressions))
		for key, expression := range values.expressions {
			result.expressions[key] = expression
		}
	}
	return result
}

func ps6100CloneAliasState(state ps6100AliasState) ps6100AliasState {
	result := make(ps6100AliasState, len(state))
	for object, values := range state {
		result[object] = ps6100CloneAliasValues(values)
	}
	return result
}

func ps6100MergeAliasStates(current, incoming ps6100AliasState) (ps6100AliasState, bool) {
	if current == nil {
		return ps6100CloneAliasState(incoming), true
	}
	changed := false
	for object, values := range incoming {
		existing, ok := current[object]
		if !ok {
			current[object] = ps6100CloneAliasValues(values)
			changed = true
			continue
		}
		if values.unknown && !existing.unknown {
			existing.unknown = true
			changed = true
		}
		if existing.expressions == nil && len(values.expressions) != 0 {
			existing.expressions = make(map[string]ast.Expr)
		}
		for key, expression := range values.expressions {
			if _, found := existing.expressions[key]; found {
				continue
			}
			if len(existing.expressions) >= ps6100MaxAliasValues {
				if !existing.unknown {
					existing.unknown = true
					changed = true
				}
				continue
			}
			existing.expressions[key] = expression
			changed = true
		}
		current[object] = existing
	}
	return current, changed
}

func (flow *ps6100AliasFlow) stateAt(node ast.Node) ps6100AliasState {
	if flow == nil {
		return nil
	}
	for current := node; current != nil; current = flow.parents[current] {
		if state, ok := flow.before[current]; ok {
			return state
		}
	}
	return nil
}

func (flow *ps6100AliasFlow) stateWithin(root ast.Node) ps6100AliasState {
	if flow == nil || root == nil {
		return nil
	}
	if state := flow.stateAt(root); state != nil {
		return state
	}
	var result ps6100AliasState
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil || result != nil {
			return false
		}
		if state, found := flow.before[node]; found {
			result = state
			return false
		}
		return true
	})
	return result
}

func (flow *ps6100AliasFlow) environmentAt(node ast.Node) map[types.Object]ast.Expr {
	if flow == nil {
		return nil
	}
	state := flow.stateAt(node)
	result := make(map[types.Object]ast.Expr, len(flow.base)+len(state))
	for object, expression := range flow.base {
		result[object] = expression
	}
	for object, values := range state {
		if values.unknown || len(values.expressions) != 1 {
			continue
		}
		for _, expression := range values.expressions {
			if expression != nil {
				result[object] = expression
			}
		}
	}
	return result
}

func (flow *ps6100AliasFlow) ambiguousRelevantAlias(pass *analysis.Pass, node ast.Node, relevant map[string]ps6100Storage) (ps6100Storage, bool) {
	if flow == nil || node == nil {
		return ps6100Storage{}, false
	}
	state := flow.stateAt(node)
	var matched ps6100Storage
	found := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := candidate.(*ast.Ident)
		if !ok {
			return true
		}
		values, ok := state[pass.TypesInfo.ObjectOf(identifier)]
		if !ok || !values.unknown && len(values.expressions) <= 1 {
			return true
		}
		for _, expression := range values.expressions {
			storage, ok := ps6100StorageOf(pass, expression, flow.base)
			if ok && ps6100TouchesRelevant(relevant, storage) {
				matched, found = storage, true
				return false
			}
		}
		if values.unknown && len(relevant) != 0 {
			for _, storage := range relevant {
				if !found || storage.key < matched.key {
					matched, found = storage, true
				}
			}
		}
		return !found
	})
	return matched, found
}

func ps6100CollectMutationFacts(
	pass *analysis.Pass,
	root ast.Node,
	helpers ps6100Helpers,
	callables ps6100CallableBindings,
	relevant map[string]ps6100Storage,
	skip map[ast.Node]bool,
	environment map[types.Object]ast.Expr,
	aliasState ps6100AliasState,
	callableState ps6100CallableState,
	active map[string]bool,
	callableWork *int,
	depth int,
	insideLoop bool,
) ps6100MutationFacts {
	var facts ps6100MutationFacts
	if root == nil {
		return facts
	}
	if depth >= ps6100MaxHelperDepth || callableWork == nil || *callableWork <= 0 {
		ps6100AppendCallableHazards(&facts, relevant, root, "callable analysis bound reached; may mutate or retain ")
		return facts
	}
	parents := ps6087Parents(root)
	block, _ := root.(*ast.BlockStmt)
	var aliasFlow *ps6100AliasFlow
	if block != nil {
		aliasFlow = ps6100AliasFlowWithState(pass, block, environment, aliasState)
	}
	var callableFlow ps6100CallableBindings
	if depth > 0 && block != nil {
		callableFlow = ps6100CallableFlowForBody(pass, block, callableState, aliasFlow)
	}
	reachability := ps6100ReachableNodes(pass, block)
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		if node != root && !reachability.mayExecute(node) {
			return false
		}
		if skip[node] {
			return false
		}
		nodeEnvironment := environment
		nodeAliasState := aliasState
		if aliasFlow != nil {
			nodeEnvironment = aliasFlow.environmentAt(node)
			nodeAliasState = aliasFlow.stateAt(node)
		}
		nodeCallableState := callableState
		if depth == 0 {
			if state := callables.stateAt(node); state != nil {
				nodeCallableState = state
			}
		} else if state := callableFlow.stateAt(node); state != nil {
			nodeCallableState = state
		}
		if literal, nested := node.(*ast.FuncLit); nested {
			if depth == 0 && ps6100DeferredLiteral(literal, parents) {
				return false
			}
			facts.hazards = append(facts.hazards, ps6100ClosureHazards(pass, literal, nodeEnvironment, relevant)...)
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				if storage, ambiguous := aliasFlow.ambiguousRelevantAlias(pass, left, relevant); ambiguous {
					facts.hazards = append(facts.hazards, ps6100Hazard{node: left, reason: "helper alias flow may mutate " + storage.name})
				}
				ps6100RecordMutation(pass, root, parents, left, nodeEnvironment, relevant, insideLoop, &facts)
				ps6100RecordBindingHazard(pass, left, nodeEnvironment, relevant, &facts)
			}
		case *ast.IncDecStmt:
			if storage, ambiguous := aliasFlow.ambiguousRelevantAlias(pass, value.X, relevant); ambiguous {
				facts.hazards = append(facts.hazards, ps6100Hazard{node: value.X, reason: "helper alias flow may mutate " + storage.name})
			}
			ps6100RecordMutation(pass, root, parents, value.X, nodeEnvironment, relevant, insideLoop, &facts)
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				if storage, ambiguous := aliasFlow.ambiguousRelevantAlias(pass, value.X, relevant); ambiguous {
					facts.hazards = append(facts.hazards, ps6100Hazard{node: value, reason: "helper alias flow may expose " + storage.name})
				}
				if storage, ok := ps6100ExposedStorage(pass, value.X, nodeEnvironment); ok && ps6100TouchesRelevant(relevant, storage) {
					facts.hazards = append(facts.hazards, ps6100Hazard{node: value, reason: "address exposing " + storage.name + " escapes local proof"})
				}
			}
		case *ast.CallExpr:
			mutationCount, hazardCount := len(facts.mutations), len(facts.hazards)
			asynchronous := ps6100AsyncCall(value, parents)
			if storage, ambiguous := aliasFlow.ambiguousRelevantAlias(pass, value, relevant); ambiguous {
				facts.hazards = append(facts.hazards, ps6100Hazard{node: value, reason: "helper alias flow may pass or mutate " + storage.name})
			}
			if depth == 0 && ps6100DeferredCall(value, parents) {
				// The deferred body belongs to the enclosing source function and
				// runs only when that frame returns. Nested calls used to evaluate
				// its receiver and arguments remain visible as AST children.
				return true
			}
			if ps6100SafeCall(pass, value) {
				return true
			}
			resolution := ps6100ResolveCallables(pass, value.Fun, nodeEnvironment, callables, nodeCallableState)
			complete := !resolution.unknown && len(resolution.targets) != 0
			localOpaque := resolution.unknown
			for _, target := range resolution.targets {
				body, next, callable := ps6100CallableInvocation(pass, target, value, helpers, nodeEnvironment)
				if !callable {
					complete = false
					if (target.function != nil && helpers[target.function.Origin()] != nil) || target.literal != nil {
						localOpaque = true
					}
					if target.receiver != nil {
						if storage, ok := ps6100ExposedStorage(pass, target.receiver, nodeEnvironment); ok && ps6100TouchesRelevant(relevant, storage) {
							facts.hazards = append(facts.hazards, ps6100Hazard{node: value, reason: "opaque method value may mutate " + storage.name})
						}
					}
					continue
				}
				key := ps6100CallableActiveKey(target)
				if active[key] || depth+1 >= ps6100MaxHelperDepth || *callableWork <= 0 {
					complete = false
					localOpaque = true
					continue
				}
				*callableWork = *callableWork - 1
				active[key] = true
				inner := ps6100CollectMutationFacts(
					pass, body, helpers, callables, relevant, nil, next, nodeAliasState, nodeCallableState, active, callableWork, depth+1,
					insideLoop || ps6100NestedLoopMutation(root, value, parents),
				)
				delete(active, key)
				facts.mutations = append(facts.mutations, inner.mutations...)
				facts.hazards = append(facts.hazards, inner.hazards...)
			}
			if complete {
				if asynchronous && (len(facts.mutations) != mutationCount || len(facts.hazards) != hazardCount) {
					ps6100AppendCallableHazards(&facts, relevant, value, "goroutine callable may mutate or retain ")
				}
				return true
			}
			for _, argument := range value.Args {
				if storage, ok := ps6100ExposedStorage(pass, argument, nodeEnvironment); ok && ps6100TouchesRelevant(relevant, storage) {
					facts.hazards = append(facts.hazards, ps6100Hazard{node: value, reason: "opaque call may bulk-mutate or retain " + storage.name})
				}
			}
			if selector, ok := ps2110Unparen(value.Fun).(*ast.SelectorExpr); ok {
				if storage, ok := ps6100ExposedStorage(pass, selector.X, nodeEnvironment); ok && ps6100TouchesRelevant(relevant, storage) {
					facts.hazards = append(facts.hazards, ps6100Hazard{node: value, reason: "opaque method may mutate " + storage.name})
				}
			}
			if localOpaque || ps6100IndirectCallable(pass, value.Fun) {
				ps6100AppendCallableHazards(&facts, relevant, value, "opaque function value may mutate or retain ")
			}
			if asynchronous && (localOpaque || len(facts.mutations) != mutationCount || len(facts.hazards) != hazardCount) {
				ps6100AppendCallableHazards(&facts, relevant, value, "goroutine callable may mutate or retain ")
			}
		}
		return true
	})
	return facts
}

func ps6100DeferredLiteral(literal *ast.FuncLit, parents map[ast.Node]ast.Node) bool {
	if literal == nil {
		return false
	}
	call, ok := parents[literal].(*ast.CallExpr)
	return ok && ps2110Unparen(call.Fun) == literal && ps6100DeferredCall(call, parents)
}

func ps6100AsyncCall(call *ast.CallExpr, parents map[ast.Node]ast.Node) bool {
	if call == nil {
		return false
	}
	_, asynchronous := parents[call].(*ast.GoStmt)
	return asynchronous
}

func ps6100IndirectCallable(pass *analysis.Pass, expression ast.Expr) bool {
	if _, _, ok := typedCallee(pass, expression); ok {
		return false
	}
	if identifier, ok := ps2110Unparen(expression).(*ast.Ident); ok {
		if _, builtin := pass.TypesInfo.ObjectOf(identifier).(*types.Builtin); builtin {
			return false
		}
	}
	return ps6100FunctionValueType(pass.TypesInfo.TypeOf(ps2110Unparen(expression)))
}

func ps6100AppendCallableHazards(facts *ps6100MutationFacts, relevant map[string]ps6100Storage, node ast.Node, prefix string) {
	if facts == nil || node == nil {
		return
	}
	names := make([]string, 0, len(relevant))
	byName := make(map[string]ps6100Storage, len(relevant))
	for _, storage := range relevant {
		names = append(names, storage.name)
		byName[storage.name] = storage
	}
	slices.Sort(names)
	for _, name := range slices.Compact(names) {
		facts.hazards = append(facts.hazards, ps6100Hazard{node: node, reason: prefix + byName[name].name})
	}
}

func ps6100RecordMutation(pass *analysis.Pass, root ast.Node, parents map[ast.Node]ast.Node, expression ast.Expr, environment map[types.Object]ast.Expr, relevant map[string]ps6100Storage, insideLoop bool, facts *ps6100MutationFacts) {
	expression = ps2110Unparen(expression)
	index, ok := ps6100MutationIndex(pass, expression, environment, make(map[types.Object]bool))
	if !ok {
		return
	}
	storage, display, identity, ok := ps6100IndexedPath(pass, index, environment)
	if !ok || !ps6100TouchesRelevant(relevant, storage) {
		return
	}
	if insideLoop || ps6100NestedLoopMutation(root, expression, parents) {
		facts.hazards = append(facts.hazards, ps6100Hazard{node: expression, reason: "loop-carried writes may bulk-mutate " + storage.name})
		return
	}
	facts.mutations = append(facts.mutations, ps6100Mutation{
		storage:  storage,
		index:    display,
		identity: identity,
		node:     expression,
	})
}

func ps6100MutationIndex(pass *analysis.Pass, expression ast.Expr, environment map[types.Object]ast.Expr, seen map[types.Object]bool) (*ast.IndexExpr, bool) {
	expression = ps6100ResolveExpression(pass, expression, environment, seen)
	switch value := ps2110Unparen(expression).(type) {
	case *ast.IndexExpr:
		return value, true
	case *ast.SelectorExpr:
		return ps6100MutationIndex(pass, value.X, environment, seen)
	case *ast.StarExpr:
		target := ps6100ResolveExpression(pass, value.X, environment, seen)
		if address, ok := ps2110Unparen(target).(*ast.UnaryExpr); ok && address.Op == token.AND {
			return ps6100MutationIndex(pass, address.X, environment, seen)
		}
	}
	return nil, false
}

func ps6100IndexedPath(pass *analysis.Pass, index *ast.IndexExpr, environment map[types.Object]ast.Expr) (ps6100Storage, string, string, bool) {
	if index == nil {
		return ps6100Storage{}, "", "", false
	}
	base, indexes, pendingOffset, ok := ps6100FlattenIndexedPath(pass, index, environment, make(map[types.Object]bool))
	if !ok || pendingOffset != nil || len(indexes) == 0 {
		return ps6100Storage{}, "", "", false
	}
	storage, ok := ps6100StorageOf(pass, base, environment)
	if !ok {
		return ps6100Storage{}, "", "", false
	}
	displays := make([]string, 0, len(indexes))
	identities := make([]string, 0, len(indexes))
	for _, expression := range indexes {
		displays = append(displays, exprTextRendered(expression))
		identities = append(identities, ps6100PredicateShape(pass, expression))
	}
	return storage, strings.Join(displays, "]["), strings.Join(identities, "/"), true
}

// ps6100FlattenIndexedPath expands helper-local aliases while retaining slice
// view offsets. indexes are returned in root-to-leaf order; pendingOffset is a
// low bound that must be applied to the next index at the current dimension.
func ps6100FlattenIndexedPath(pass *analysis.Pass, expression ast.Expr, environment map[types.Object]ast.Expr, seen map[types.Object]bool) (ast.Expr, []ast.Expr, ast.Expr, bool) {
	expression = ps2110Unparen(expression)
	if identifier, ok := expression.(*ast.Ident); ok && environment != nil {
		object := pass.TypesInfo.ObjectOf(identifier)
		if actual := environment[object]; actual != nil {
			if seen[object] {
				return nil, nil, nil, false
			}
			seen[object] = true
			base, indexes, offset, resolved := ps6100FlattenIndexedPath(pass, actual, environment, seen)
			delete(seen, object)
			return base, indexes, offset, resolved
		}
	}
	switch value := expression.(type) {
	case *ast.IndexExpr:
		base, indexes, offset, ok := ps6100FlattenIndexedPath(pass, value.X, environment, seen)
		if !ok {
			return nil, nil, nil, false
		}
		resolvedIndex := ps6100ResolveExpression(pass, value.Index, environment, make(map[types.Object]bool))
		indexes = append(indexes, ps6100OffsetIndex(pass, offset, resolvedIndex))
		return base, indexes, nil, true
	case *ast.SliceExpr:
		base, indexes, offset, ok := ps6100FlattenIndexedPath(pass, value.X, environment, seen)
		if !ok {
			return nil, nil, nil, false
		}
		low := ps6100ResolveExpression(pass, value.Low, environment, make(map[types.Object]bool))
		return base, indexes, ps6100OffsetIndex(pass, offset, low), true
	case *ast.CallExpr:
		if len(value.Args) == 1 && pass.TypesInfo.Types[ps2110Unparen(value.Fun)].IsType() && ps6100ReferenceAliasType(pass.TypesInfo.TypeOf(value)) {
			return ps6100FlattenIndexedPath(pass, value.Args[0], environment, seen)
		}
	}
	return expression, nil, nil, true
}

func ps6100OffsetIndex(pass *analysis.Pass, offset, index ast.Expr) ast.Expr {
	if offset == nil {
		return index
	}
	if index == nil {
		return offset
	}
	offsetValue := ps6100Constant(pass, offset)
	indexValue := ps6100Constant(pass, index)
	if offsetValue != nil && offsetValue.Kind() == constant.Int && constant.Sign(offsetValue) == 0 {
		return index
	}
	if indexValue != nil && indexValue.Kind() == constant.Int && constant.Sign(indexValue) == 0 {
		return offset
	}
	if offsetValue != nil && offsetValue.Kind() == constant.Int && indexValue != nil && indexValue.Kind() == constant.Int {
		return &ast.BasicLit{Kind: token.INT, Value: constant.BinaryOp(offsetValue, token.ADD, indexValue).ExactString()}
	}
	return &ast.BinaryExpr{X: offset, Op: token.ADD, Y: index}
}

func ps6100RecordBindingHazard(pass *analysis.Pass, expression ast.Expr, environment map[types.Object]ast.Expr, relevant map[string]ps6100Storage, facts *ps6100MutationFacts) {
	expression = ps2110Unparen(expression)
	if _, indexed := expression.(*ast.IndexExpr); indexed {
		return
	}
	if identifier, ok := expression.(*ast.Ident); ok && environment[pass.TypesInfo.ObjectOf(identifier)] != nil {
		// Assigning a helper parameter only rebinds the helper-local slice header.
		return
	}
	storage, ok := ps6100StorageOf(pass, expression, environment)
	if !ok || !ps6100TouchesRelevant(relevant, storage) {
		return
	}
	facts.hazards = append(facts.hazards, ps6100Hazard{node: expression, reason: "rebinding " + storage.name + " invalidates cached membership"})
}

func ps6100ClosureHazards(pass *analysis.Pass, literal *ast.FuncLit, environment map[types.Object]ast.Expr, relevant map[string]ps6100Storage) []ps6100Hazard {
	if literal == nil || literal.Body == nil {
		return nil
	}
	seen := make(map[string]bool)
	var hazards []ps6100Hazard
	ast.Inspect(literal.Body, func(node ast.Node) bool {
		if nested, ok := node.(*ast.FuncLit); ok && nested != literal {
			return false
		}
		expression, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		storage, ok := ps6100StorageOf(pass, expression, environment)
		if !ok || !ps6100TouchesRelevant(relevant, storage) || seen[storage.key] {
			return true
		}
		seen[storage.key] = true
		hazards = append(hazards, ps6100Hazard{node: expression, reason: "closure may mutate or retain " + storage.name})
		return true
	})
	return hazards
}

func ps6100NestedLoopMutation(root, node ast.Node, parents map[ast.Node]ast.Node) bool {
	for node = parents[node]; node != nil && node != root; node = parents[node] {
		switch node.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			return true
		}
	}
	return false
}

func ps6100IndexedStorage(pass *analysis.Pass, expression ast.Expr, environment map[types.Object]ast.Expr) (ps6100Storage, bool) {
	index, ok := ps6100MutationIndex(pass, expression, environment, make(map[types.Object]bool))
	if !ok {
		return ps6100Storage{}, false
	}
	return ps6100StorageOf(pass, index.X, environment)
}

func ps6100TouchesRelevant(relevant map[string]ps6100Storage, storage ps6100Storage) bool {
	if storage.key == "" {
		return false
	}
	for key := range relevant {
		if key == storage.key || strings.HasPrefix(key, storage.key+"/") || strings.HasPrefix(storage.key, key+"/") {
			return true
		}
	}
	return false
}

func ps6100Addresses(pass *analysis.Pass) []ps6100AddressExposure {
	var addresses []ps6100AddressExposure
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok && function.Body != nil {
				ps6100CollectAddresses(pass, function.Body, ps6100ReachableNodes(pass, function.Body), &addresses)
				continue
			}
			ps6100CollectAddresses(pass, declaration, nil, &addresses)
		}
	}
	return addresses
}

func ps6100CollectAddresses(pass *analysis.Pass, root ast.Node, reachability *ps6100Reachability, addresses *[]ps6100AddressExposure) {
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		if node != root && !reachability.mayExecute(node) {
			return false
		}
		if literal, ok := node.(*ast.FuncLit); ok && literal.Body != nil {
			ps6100CollectAddresses(pass, literal.Body, ps6100ReachableNodes(pass, literal.Body), addresses)
			return false
		}
		address, ok := node.(*ast.UnaryExpr)
		if !ok || address.Op != token.AND {
			return true
		}
		if storage, ok := ps6100ExposedStorage(pass, address.X, nil); ok {
			*addresses = append(*addresses, ps6100AddressExposure{node: address, storage: storage})
		}
		return true
	})
}

func ps6100FunctionValueType(typ types.Type) bool {
	if typ == nil {
		return false
	}
	_, ok := types.Unalias(typ).Underlying().(*types.Signature)
	return ok
}

func ps6100DeferredCall(call *ast.CallExpr, parents map[ast.Node]ast.Node) bool {
	if call == nil {
		return false
	}
	_, deferred := parents[call].(*ast.DeferStmt)
	return deferred
}

func ps6100StableAliases(pass *analysis.Pass, body *ast.BlockStmt, helpers ps6100Helpers, callables ps6100CallableBindings, addresses []ps6100AddressExposure) map[types.Object]ast.Expr {
	writes := make(map[types.Object]int)
	defined := make(map[types.Object]bool)
	candidates := make(map[types.Object]ast.Expr)
	addressed := make(map[types.Object]bool, len(addresses))
	for _, address := range addresses {
		addressed[address.storage.object] = true
	}
	record := func(object types.Object, source ast.Expr, definition bool) {
		if object == nil {
			return
		}
		writes[object]++
		defined[object] = defined[object] || definition
		if source != nil && ps6100ReferenceAliasType(object.Type()) {
			candidates[object] = source
		}
	}
	unknownCallableInvocation := false
	active := make(map[string]bool)
	work := ps6100MaxCallableWork
	var inspect func(ast.Node, map[types.Object]ast.Expr, ps6100CallableState, int, bool)
	inspect = func(root ast.Node, environment map[types.Object]ast.Expr, callableState ps6100CallableState, depth int, outerFrame bool) {
		if root == nil || depth >= ps6100MaxHelperDepth || work == 0 {
			unknownCallableInvocation = true
			return
		}
		parents := ps6087Parents(root)
		block, _ := root.(*ast.BlockStmt)
		live := ps6100ReachableNodes(pass, block)
		var aliasFlow *ps6100AliasFlow
		var callableFlow ps6100CallableBindings
		if !outerFrame && block != nil {
			aliasFlow = ps6100HelperAliasFlow(pass, block, environment)
			callableFlow = ps6100CallableFlowForBody(pass, block, callableState, aliasFlow)
		}
		ast.Inspect(root, func(node ast.Node) bool {
			if node != root && !live.mayExecute(node) {
				return false
			}
			if call, ok := node.(*ast.CallExpr); ok {
				nodeEnvironment := environment
				if aliasFlow != nil {
					nodeEnvironment = aliasFlow.environmentAt(node)
				}
				nodeCallableState := callableState
				if outerFrame {
					if state := callables.stateAt(node); state != nil {
						nodeCallableState = state
					}
				} else if state := callableFlow.stateAt(node); state != nil {
					nodeCallableState = state
				}
				if outerFrame && ps6100DeferredCall(call, parents) {
					return true
				}
				resolution := ps6100ResolveCallables(pass, call.Fun, nodeEnvironment, callables, nodeCallableState)
				if resolution.unknown && ps6100IndirectCallable(pass, call.Fun) {
					unknownCallableInvocation = true
				}
				for _, target := range resolution.targets {
					calleeBody, next, callable := ps6100CallableInvocation(pass, target, call, helpers, nodeEnvironment)
					if !callable {
						if target.literal != nil || (target.function != nil && helpers[target.function.Origin()] != nil) {
							unknownCallableInvocation = true
						}
						continue
					}
					key := ps6100CallableActiveKey(target)
					if active[key] || work == 0 {
						unknownCallableInvocation = true
						continue
					}
					work--
					active[key] = true
					inspect(calleeBody, next, nodeCallableState, depth+1, false)
					delete(active, key)
				}
				for _, argument := range call.Args {
					if ps6100FunctionValueType(pass.TypesInfo.TypeOf(argument)) && ps6100OpaqueCallbackCall(pass, call) {
						unknownCallableInvocation = true
					}
				}
			}
			if _, literal := node.(*ast.FuncLit); literal {
				return false
			}
			switch statement := node.(type) {
			case *ast.AssignStmt:
				for index, left := range statement.Lhs {
					identifier, ok := ps2110Unparen(left).(*ast.Ident)
					if !ok || identifier.Name == "_" {
						continue
					}
					object := ps6100AssignedObject(pass, identifier, statement.Tok)
					var source ast.Expr
					if statement.Tok == token.DEFINE && pass.TypesInfo.Defs[identifier] != nil && len(statement.Lhs) == len(statement.Rhs) {
						source = ps6100AliasSource(pass, statement.Rhs[index])
					}
					record(object, source, pass.TypesInfo.Defs[identifier] != nil)
				}
			case *ast.ValueSpec:
				for index, name := range statement.Names {
					var source ast.Expr
					if len(statement.Names) == len(statement.Values) {
						source = ps6100AliasSource(pass, statement.Values[index])
					}
					record(pass.TypesInfo.Defs[name], source, true)
				}
			case *ast.RangeStmt:
				for _, expression := range []ast.Expr{statement.Key, statement.Value} {
					identifier, ok := ps2110Unparen(expression).(*ast.Ident)
					if ok && identifier.Name != "_" {
						record(ps6100AssignedObject(pass, identifier, statement.Tok), nil, false)
					}
				}
			}
			return true
		})
	}
	inspect(body, nil, nil, 0, true)
	if unknownCallableInvocation {
		// An unresolved function-valued local may select a closure from a
		// source set larger than our bound. Do not promote a reference alias
		// whose captured header could then be rebound invisibly.
		return nil
	}
	aliases := make(map[types.Object]ast.Expr, len(candidates))
	for object, source := range candidates {
		if writes[object] != 1 || !defined[object] || addressed[object] || source == nil {
			continue
		}
		storage, ok := ps6100StorageOf(pass, source, nil)
		if !ok || ps6100PackageVariable(storage.object) || writes[storage.object] > 1 || writes[storage.object] == 1 && !defined[storage.object] {
			continue
		}
		aliases[object] = source
	}
	return aliases
}

func ps6100OpaqueCallbackCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	function, _, resolved := typedCallee(pass, call.Fun)
	return !resolved || function.Pkg() == nil || function.Pkg() != pass.Pkg
}

func ps6100ReferenceAliasType(typ types.Type) bool {
	return ps6100ReferenceAliasConstraint(typ, make(map[types.Type]bool))
}

func ps6100ReferenceAliasConstraint(typ types.Type, seen map[types.Type]bool) bool {
	if typ == nil {
		return false
	}
	typ = types.Unalias(typ)
	if seen[typ] {
		return false
	}
	seen[typ] = true
	defer delete(seen, typ)
	if parameter, ok := typ.(*types.TypeParam); ok {
		return ps6100ReferenceAliasConstraint(parameter.Constraint(), seen)
	}
	if union, ok := typ.(*types.Union); ok {
		if union.Len() == 0 {
			return false
		}
		for index := range union.Len() {
			if !ps6100ReferenceAliasConstraint(union.Term(index).Type(), seen) {
				return false
			}
		}
		return true
	}
	switch underlying := typ.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map:
		return true
	case *types.Interface:
		underlying.Complete()
		for index := range underlying.NumEmbeddeds() {
			if ps6100ReferenceAliasConstraint(underlying.EmbeddedType(index), seen) {
				return true
			}
		}
	}
	return false
}

func ps6100AliasSource(pass *analysis.Pass, expression ast.Expr) ast.Expr {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		return expression
	case *ast.CallExpr:
		if len(value.Args) == 1 && pass.TypesInfo.Types[ps2110Unparen(value.Fun)].IsType() {
			return ps6100AliasSource(pass, value.Args[0])
		}
	}
	return nil
}

func ps6100PackageVariable(object types.Object) bool {
	return object != nil && object.Pkg() != nil && object.Parent() == object.Pkg().Scope()
}

func ps6100AddressHazards(addresses []ps6100AddressExposure, relevant map[string]ps6100Storage) []ps6100Hazard {
	var hazards []ps6100Hazard
	for _, address := range addresses {
		if ps6100TouchesRelevant(relevant, address.storage) {
			hazards = append(hazards, ps6100Hazard{node: address.node, reason: "address exposing " + address.storage.name + " escapes local proof"})
		}
	}
	return hazards
}

func ps6100ExposedStorage(pass *analysis.Pass, expression ast.Expr, environment map[types.Object]ast.Expr) (ps6100Storage, bool) {
	expression = ps6100ResolveExpression(pass, expression, environment, make(map[types.Object]bool))
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.SliceExpr:
		return ps6100StorageOf(pass, expression, environment)
	case *ast.IndexExpr:
		return ps6100StorageOf(pass, value.X, environment)
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			if storage, ok := ps6100IndexedStorage(pass, value.X, environment); ok {
				return storage, true
			}
			return ps6100StorageOf(pass, value.X, environment)
		}
	}
	return ps6100Storage{}, false
}

func ps6100SafeCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	if call == nil {
		return true
	}
	if pass.TypesInfo.Types[ps2110Unparen(call.Fun)].IsType() {
		return true
	}
	for _, name := range []string{"len", "cap", "min", "max"} {
		if typedBuiltinName(pass, call.Fun, name) {
			return true
		}
	}
	return false
}

func ps6100Exits(pass *analysis.Pass, body *ast.BlockStmt, skip map[ast.Node]bool) []ast.Node {
	var exits []ast.Node
	parents := ps6087Parents(body)
	reachability := ps6100ReachableNodes(pass, body)
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		if node != body && !reachability.mayExecute(node) {
			return false
		}
		if skip[node] {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.ReturnStmt:
			exits = append(exits, value)
		case *ast.BranchStmt:
			switch value.Tok {
			case token.GOTO:
				exits = append(exits, value)
			case token.BREAK, token.CONTINUE:
				if ps6100BranchTargetsOuter(value, parents) {
					exits = append(exits, value)
				}
			}
		}
		return true
	})
	return exits
}

func ps6100BranchTargetsOuter(branch *ast.BranchStmt, parents map[ast.Node]ast.Node) bool {
	if branch == nil || branch.Label != nil {
		// Labels can target the enclosing loop from arbitrarily deep nesting.
		return true
	}
	for node := parents[branch]; node != nil; node = parents[node] {
		switch branch.Tok {
		case token.CONTINUE:
			switch node.(type) {
			case *ast.ForStmt, *ast.RangeStmt:
				return false
			}
		case token.BREAK:
			switch node.(type) {
			case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
				return false
			}
		}
	}
	return true
}

func ps6100Report(
	pass *analysis.Pass,
	outer ast.Node,
	scans []ps6100Scan,
	inputs map[string]ps6100Storage,
	invariants map[string]ps6100Storage,
	predicates map[string]bool,
	mutations []ps6100Mutation,
	hazards []ps6100Hazard,
	exits []ast.Node,
) {
	inputNames := make([]string, 0, len(inputs))
	aliasNames := make([]string, 0, len(inputs))
	for _, input := range inputs {
		inputNames = append(inputNames, input.name)
		if _, slice := types.Unalias(input.typ).Underlying().(*types.Slice); slice {
			aliasNames = append(aliasNames, input.name)
		}
	}
	slices.Sort(inputNames)
	inputNames = slices.Compact(inputNames)
	slices.Sort(aliasNames)
	aliasNames = slices.Compact(aliasNames)

	message := fmt.Sprintf("iterative loop repeats %d full scans of %s and derives %d finite-state predicate bit(s) from per-element inputs %s; %d bounded indexed mutation site(s) (%s) make a compact status cache candidate", len(scans), scans[0].loop.bound.name, len(predicates), strings.Join(inputNames, ", "), len(mutations), ps6100MutationSummary(pass, mutations))
	if len(aliasNames) != 0 {
		message += "; prove reference-backed aliases cannot mutate " + strings.Join(aliasNames, ", ") + " outside the listed refresh sites"
	}
	if names := ps6100StorageNames(invariants); len(names) != 0 {
		message += "; prove predicate/config dependencies remain invariant or rebuild all status entries when they change: " + strings.Join(names, ", ")
	}
	if summary := ps6100HazardSummary(pass, hazards); summary != "" {
		message += "; incremental refresh is unsafe until these bulk/escape hazards are resolved: " + summary
	}
	if summary := ps6100ExitSummary(pass, exits); summary != "" {
		message += "; refresh every changed index before these exit/continue paths: " + summary
	}
	message += "; initialize once, refresh only proven changed indexes, and preserve traversal order, ties, arithmetic, and termination (advisory, no automatic fix)"

	related := make([]analysis.RelatedInformation, 0, len(scans)+len(mutations)+len(hazards)+len(exits))
	for index, scan := range scans {
		related = append(related, analysis.RelatedInformation{
			Pos: scan.loop.node.Pos(), End: scan.loop.node.End(),
			Message: "full predicate scan " + strconv.Itoa(index+1) + "/" + strconv.Itoa(len(scans)),
		})
	}
	for _, mutation := range mutations {
		related = append(related, analysis.RelatedInformation{Pos: mutation.node.Pos(), End: mutation.node.End(), Message: "bounded mutation of " + mutation.storage.name + "[" + mutation.index + "]"})
	}
	for _, hazard := range ps6100UniqueHazards(hazards) {
		related = append(related, analysis.RelatedInformation{Pos: hazard.node.Pos(), End: hazard.node.End(), Message: hazard.reason})
	}
	for _, exit := range exits {
		related = append(related, analysis.RelatedInformation{Pos: exit.Pos(), End: exit.End(), Message: "status refresh path to audit"})
	}
	pass.Report(analysis.Diagnostic{Pos: outer.Pos(), End: outer.End(), Message: message, Related: related})
}

func ps6100StorageNames(storages map[string]ps6100Storage) []string {
	names := make([]string, 0, len(storages))
	for _, storage := range storages {
		names = append(names, storage.name)
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func ps6100MutationSummary(pass *analysis.Pass, mutations []ps6100Mutation) string {
	values := make([]string, 0, len(mutations))
	for _, mutation := range mutations {
		position := pass.Fset.Position(mutation.node.Pos())
		values = append(values, mutation.storage.name+"["+mutation.index+"]@"+strconv.Itoa(position.Line))
	}
	slices.Sort(values)
	return strings.Join(slices.Compact(values), ", ")
}

func ps6100UniqueHazards(hazards []ps6100Hazard) []ps6100Hazard {
	seen := make(map[string]bool)
	result := make([]ps6100Hazard, 0, len(hazards))
	for _, hazard := range hazards {
		key := strconv.Itoa(int(hazard.node.Pos())) + "|" + hazard.reason
		if !seen[key] {
			seen[key] = true
			result = append(result, hazard)
		}
	}
	return result
}

func ps6100HazardSummary(pass *analysis.Pass, hazards []ps6100Hazard) string {
	values := make([]string, 0, len(hazards))
	for _, hazard := range ps6100UniqueHazards(hazards) {
		values = append(values, hazard.reason+"@"+strconv.Itoa(pass.Fset.Position(hazard.node.Pos()).Line))
	}
	slices.Sort(values)
	return strings.Join(values, ", ")
}

func ps6100ExitSummary(pass *analysis.Pass, exits []ast.Node) string {
	values := make([]string, 0, len(exits))
	for _, exit := range exits {
		kind := "return"
		if branch, ok := exit.(*ast.BranchStmt); ok {
			kind = branch.Tok.String()
		}
		values = append(values, kind+"@"+strconv.Itoa(pass.Fset.Position(exit.Pos()).Line))
	}
	slices.Sort(values)
	return strings.Join(slices.Compact(values), ", ")
}

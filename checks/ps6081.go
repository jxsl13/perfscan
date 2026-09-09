package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6081 implements owner issue #830 as a fail-closed contract advisory.
var PS6081 = register(&lint.Check{
	ID: "PS6081", Category: "verify", Slug: "sibling-kernels-repeat-parallel-fanout",
	Level: lint.LevelStructured, AutoFix: false, NeedsConfig: true,
	Vocab: []string{"sharedFanOutContracts"},
	Doc: lint.Documentation{
		Title: "configured sibling kernels repeat one exact parallel fan-out",
		Text: `PS6081 reports only a project-configured owner whose two exact typed
producer roles share the configured input, metadata, mapped fan-out domain,
helper route, and synchronous completion semantics before an exact configured
transform/composite/final flow. Argument roles are explicit; the check never
guesses that the first reference-bearing argument is the input.

The contract is fail closed. It names receiver paths, argument and result
positions and modes, receiver-held weights or explicit weight arguments,
operation constants, the exact terminal callback position, mapped extent and
work arguments, collection indexes, data-or-collection/error transforms and
composite, a direct-return final, and bool-condition alternate variants with
exactly one matched owner call.
Duplicate names or semantic claims, incomplete promises, short-name collisions, unlisted observations,
unsafe escapes, asynchronous work, ambiguous local routes, and source-visible
contradictions all stay silent. Each source-visible route step must dominate
every return from its local producer or wrapper. Imported routes are trusted only through the
complete project-owned contract, including shape, dtype, backend, domain,
ownership, immutability, side-effect, error, panic, and fallback assertions.

A declaration-doc line beginning exactly with
//perfscan:shared-fanout-validated suppresses the configured owner. Ordinary
//perfscan:ignore PS6081 handling is unchanged.

There is NO automatic fix and no universal performance claim. Evaluate a
separately selectable paired primitive at the complete owner boundary, with
exact-output and odd-tail gates plus paired, alternating-order benchmarks.
Allocation or scheduler changes alone are not evidence of a win.`,
		Before: `gate, err := s.Gate.Forward(ctx, x)
up, err := s.Up.Forward(ctx, x)
gate, err = SiLU(ctx, gate)
out, err := Mul(ctx, OpMul, gate, up)
return s.Down.Forward(ctx, out)`,
		After: `// Candidate only: preserve the complete configured behavior.
gate, up, err := forwardPaired(ctx, x, s.Gate, s.Up)
gate, err = SiLU(ctx, gate)
out, err := Mul(ctx, OpMul, gate, up)
return s.Down.Forward(ctx, out)`,
		MeasuredWin: `Owner issue #830 identified the pre-GoAI-d5548e24 eager
QuantSwiGLU.Forward shape: Gate.Forward and Up.Forward each reached
gguf.QMatMul and qmatmulParallelChunks before SiLU, Mul, and Down.Forward.
The advisory requires exact-output and odd-tail validation and paired
complete-operation benchmarks; it does not claim that every backend wins.`,
	},
	Analyzer: &analysis.Analyzer{Name: "PS6081", Doc: "configured sibling producers repeat one fan-out before joint consumption", Run: runPS6081},
})

type ps6081Binding struct {
	call     *ast.CallExpr
	data     types.Object
	err      types.Object
	block    *ast.BlockStmt
	index    int
	receiver ast.Expr
}

type ps6081State struct {
	pass         *analysis.Pass
	declarations map[*types.Func]*ast.FuncDecl
	reachable    map[*ast.CallExpr]bool
	flow         *ps6089LifecycleFlow
}

func runPS6081(pass *analysis.Pass) (any, error) {
	sets := config.Current()
	return runPS6081WithContracts(pass, sets.SharedFanOutContracts)
}

func runPS6081WithContracts(pass *analysis.Pass, configured []config.SharedFanOutContract) (any, error) {
	contracts := ps6081Contracts(configured)
	if len(contracts) == 0 {
		return nil, nil
	}
	state := &ps6081State{pass: pass, declarations: make(map[*types.Func]*ast.FuncDecl)}
	owners := make(map[string]*ast.FuncDecl)
	for _, file := range pass.Files {
		for _, node := range file.Decls {
			declaration, ok := node.(*ast.FuncDecl)
			if !ok || declaration.Body == nil {
				continue
			}
			object, _ := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
			if object != nil {
				state.declarations[object] = declaration
				owners[ps6081ObjectID(object)] = declaration
			}
		}
	}
	for _, contract := range contracts {
		owner := owners[contract.OwnerSite]
		if owner == nil || !ps6081DeclarationKindMatches(state.pass, owner, contract.OwnerKind) || ps6081Validated(owner) || !state.matchOwner(owner, contract) {
			continue
		}
		pass.Reportf(owner.Name.Pos(), "%s: exact configured sibling producers repeat synchronous fan-out %s before %s and %s; evaluate a separately selectable paired primitive with exact-output and odd-tail gates plus alternating-order complete-operation benchmarks (PS6081 advisory, no automatic fix; no universal win)", contract.Name, contract.FanOutHelper, contract.CompositeConsumer.Callable, contract.FinalConsumer.Callable)
	}
	return nil, nil
}

func ps6081Contracts(configured []config.SharedFanOutContract) []*config.SharedFanOutContract {
	names, claims := make(map[string]int), make(map[string]int)
	for index := range configured {
		if configured[index].Valid() {
			names[configured[index].Name]++
			claims[ps6081ContractKey(&configured[index])]++
		}
	}
	var result []*config.SharedFanOutContract
	for index := range configured {
		contract := &configured[index]
		if contract.Valid() && names[contract.Name] == 1 && claims[ps6081ContractKey(contract)] == 1 {
			result = append(result, contract)
		}
	}
	slices.SortFunc(result, func(left, right *config.SharedFanOutContract) int { return strings.Compare(left.Name, right.Name) })
	return result
}

func ps6081ContractKey(contract *config.SharedFanOutContract) string {
	return contract.OwnerSite + "\x00" + string(contract.OwnerKind) + "\x00" + contract.Producers[0].Callable + "\x00" + string(contract.Producers[0].Kind) + "\x00" + contract.Producers[0].ReceiverPath +
		"\x00" + contract.Producers[1].Callable + "\x00" + string(contract.Producers[1].Kind) + "\x00" + contract.Producers[1].ReceiverPath
}

func (state *ps6081State) matchOwner(owner *ast.FuncDecl, contract *config.SharedFanOutContract) bool {
	parents := ps6087Parents(owner.Body)
	state.reachable = ps6099ReachableCalls(state.pass, owner, parents)
	state.flow = ps6089NewLifecycleFlow(state.pass, owner.Body)
	defer func() { state.reachable, state.flow = nil, nil }()
	receiver := ps6081ReceiverObject(state.pass, owner)
	var producers [2][]ps6081Binding
	for role := range contract.Producers {
		producer := &contract.Producers[role]
		producers[role] = state.bindings(owner.Body, producer.Callable, producer.ReceiverPath, receiver, producer.DataResult, producer.ErrorResult)
		if len(producers[role]) == 0 || len(producers[role]) > 2 || !ps6081AlternativeBindings(producers[role]) ||
			!state.producerSourceSafe(contract, producer, producers[role]) {
			return false
		}
		for _, binding := range producers[role] {
			function, signature, ok := typedCallee(state.pass, binding.call.Fun)
			if !ok || !ps6081CallKindMatches(state.pass, binding.call, producer.Callable, producer.Kind) ||
				!ps6081ProducerRolesTyped(signature, producer) || state.declarations[function] != nil && !state.exactErrorGuard(binding) ||
				state.declarations[function] == nil && !state.exactErrorGuard(binding) {
				return false
			}
		}
	}
	for _, left := range producers[0] {
		for _, right := range producers[1] {
			if !state.compatibleRoles(contract, left, right) {
				return false
			}
		}
	}
	return state.exactConsumerFlow(owner, receiver, contract, producers)
}

func (state *ps6081State) bindings(body *ast.BlockStmt, callable, path string, root types.Object, dataResult, errorResult int) []ps6081Binding {
	var result []ps6081Binding
	ps6032Blocks(body, func(block *ast.BlockStmt) {
		for index, statement := range block.List {
			assignment, ok := statement.(*ast.AssignStmt)
			if !ok || len(assignment.Rhs) != 1 {
				continue
			}
			call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
			if !ok || ps6087FunctionID(state.pass, call) != callable || state.reachable != nil && !state.reachable[call] {
				continue
			}
			var receiver ast.Expr
			if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
				receiver = selector.X
			}
			if !ps6081ReceiverMatches(state.pass, receiver, path, root) {
				continue
			}
			result = append(result, ps6081Binding{call: call, data: ps6081LHSObject(state.pass, assignment, dataResult), err: ps6081LHSObject(state.pass, assignment, errorResult), block: block, index: index, receiver: receiver})
		}
	})
	return result
}

func ps6081LHSObject(pass *analysis.Pass, assignment *ast.AssignStmt, position int) types.Object {
	if position <= 0 || position > len(assignment.Lhs) {
		return nil
	}
	id, _ := ps2110Unparen(assignment.Lhs[position-1]).(*ast.Ident)
	if id == nil || id.Name == "_" {
		return nil
	}
	return pass.TypesInfo.ObjectOf(id)
}

func (state *ps6081State) compatibleRoles(contract *config.SharedFanOutContract, left, right ps6081Binding) bool {
	roles := contract.Producers
	if left.data == nil || right.data == nil || left.err == nil || right.err == nil || left.data == right.data ||
		!ps6081SameArgument(state.pass, left.call, roles[0].InputArgument, right.call, roles[1].InputArgument) ||
		!ps6081SameRoles(state.pass, left, right, roles[0].MetadataArguments, roles[1].MetadataArguments, roles[0].MetadataReceiverFields, roles[1].MetadataReceiverFields) ||
		!ps6081SameRoles(state.pass, left, right, roles[0].DomainArguments, roles[1].DomainArguments, roles[0].DomainReceiverFields, roles[1].DomainReceiverFields) {
		return false
	}
	leftWeight, leftOK := state.weightStorage(left, &roles[0])
	rightWeight, rightOK := state.weightStorage(right, &roles[1])
	return roles[0].WeightReceiverField == roles[1].WeightReceiverField && leftOK && rightOK && leftWeight != rightWeight
}

func ps6081SameRoles(pass *analysis.Pass, left, right ps6081Binding, leftArgs, rightArgs []int, leftFields, rightFields []string) bool {
	if len(leftArgs) != len(rightArgs) || len(leftFields) != len(rightFields) {
		return false
	}
	for index := range leftArgs {
		if !ps6081SameArgument(pass, left.call, leftArgs[index], right.call, rightArgs[index]) {
			return false
		}
	}
	for index := range leftFields {
		if leftFields[index] != rightFields[index] {
			return false
		}
	}
	return true
}

type ps6081StorageIdentity struct {
	root      types.Object
	container types.Object
	field     *types.Var
}

func (state *ps6081State) weightStorage(binding ps6081Binding, role *config.SharedFanOutProducer) (ps6081StorageIdentity, bool) {
	if role.WeightArgument > 0 && role.WeightArgument <= len(binding.call.Args) {
		expression := binding.call.Args[role.WeightArgument-1]
		object := ps6081ObjectExpr(state.pass, expression)
		return ps6081StorageIdentity{root: ps6081RootObject(state.pass, expression), container: object}, object != nil
	}
	_, signature, ok := typedCallee(state.pass, binding.call.Fun)
	if !ok || signature.Recv() == nil {
		return ps6081StorageIdentity{}, false
	}
	field, ok := ps6081ResolveField(signature.Recv().Type(), role.WeightReceiverField)
	if !ok {
		return ps6081StorageIdentity{}, false
	}
	identity := ps6081StorageIdentity{
		root:      ps6081RootObject(state.pass, binding.receiver),
		container: ps6081ObjectExpr(state.pass, binding.receiver),
		field:     field,
	}
	return identity, identity.root != nil && identity.container != nil
}

func (state *ps6081State) exactErrorGuard(binding ps6081Binding) bool {
	if binding.err == nil || binding.index+1 >= len(binding.block.List) {
		return false
	}
	guard, ok := binding.block.List[binding.index+1].(*ast.IfStmt)
	if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 || !ps6081ErrorCondition(state.pass, guard.Cond, binding.err) {
		return false
	}
	returned, ok := guard.Body.List[0].(*ast.ReturnStmt)
	if !ok {
		return false
	}
	for _, result := range returned.Results {
		if ps6081ObjectExpr(state.pass, result) == binding.err {
			return true
		}
	}
	return false
}

func ps6081ErrorCondition(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	return ok && binary.Op == token.NEQ &&
		(ps6081ObjectExpr(pass, binary.X) == object && ps6081Nil(binary.Y) || ps6081ObjectExpr(pass, binary.Y) == object && ps6081Nil(binary.X))
}

func (state *ps6081State) exactConsumerFlow(owner *ast.FuncDecl, root types.Object, contract *config.SharedFanOutContract, producers [2][]ps6081Binding) bool {
	if len(producers[0]) != 1 {
		return false
	}
	left, right := ps6081DirectValues(producers[0]), ps6081DirectValues(producers[1])
	gatePosition := producers[0][0].call.Pos()
	var transform ps6081Binding
	transformCount := 0
	for index := range contract.TransformConsumers {
		consumer := &contract.TransformConsumers[index]
		if consumer.ResultMode != config.SharedFanOutCollectionError && consumer.ResultMode != config.SharedFanOutDataError {
			return false
		}
		for _, binding := range state.bindings(owner.Body, consumer.Callable, consumer.ReceiverPath, root, consumer.DataResult, consumer.ErrorResult) {
			if !ps6081CallKindMatches(state.pass, binding.call, consumer.Callable, consumer.Kind) || !ps6081OperationMatches(state.pass, binding.call, consumer) {
				continue
			}
			if !state.consumerMatches(binding.call, consumer, []ps6081ValueSet{left}) || !state.exactErrorGuard(binding) || binding.call.Pos() <= gatePosition || !state.canReach(producers[0][0].call, binding.call) {
				return false
			}
			transformCount++
			transform = binding
		}
	}
	if transformCount != 1 {
		return false
	}
	transformConsumer := ps6081ConsumerForCall(state.pass, transform.call, contract.TransformConsumers)
	transformValues := ps6081ResultValues(transform, transformConsumer)
	if len(transformValues) == 0 {
		return false
	}
	parents := ps6087Parents(owner.Body)
	alternateCalls := 0
	var alternatePosition token.Pos
	type matchedAlternate struct {
		call     *ast.CallExpr
		consumer *config.SharedFanOutConsumer
	}
	var alternates []matchedAlternate
	for index := range contract.AlternateConsumers {
		consumer := &contract.AlternateConsumers[index]
		if consumer.ResultMode != config.SharedFanOutBoolCondition {
			return false
		}
		for _, call := range state.calls(owner.Body, consumer.Callable, consumer.Kind, consumer.ReceiverPath, root) {
			if _, ok := parents[call].(*ast.IfStmt); !ok || !state.consumerMatches(call, consumer, []ps6081ValueSet{left, right}) || !state.canReach(producers[0][0].call, call) {
				return false
			}
			alternateCalls++
			alternatePosition = call.Pos()
			alternates = append(alternates, matchedAlternate{call: call, consumer: consumer})
		}
	}
	if alternateCalls != 1 || alternatePosition <= gatePosition || alternatePosition >= transform.call.Pos() {
		return false
	}
	compositeConsumer := &contract.CompositeConsumer
	var composites []ps6081Binding
	for _, binding := range state.bindings(owner.Body, compositeConsumer.Callable, compositeConsumer.ReceiverPath, root, compositeConsumer.DataResult, compositeConsumer.ErrorResult) {
		if ps6081CallKindMatches(state.pass, binding.call, compositeConsumer.Callable, compositeConsumer.Kind) && ps6081OperationMatches(state.pass, binding.call, compositeConsumer) {
			composites = append(composites, binding)
		}
	}
	if len(composites) != 1 || !ps6081CallKindMatches(state.pass, composites[0].call, compositeConsumer.Callable, compositeConsumer.Kind) ||
		!state.consumerMatches(composites[0].call, compositeConsumer, []ps6081ValueSet{transformValues, right}) ||
		!state.exactErrorGuard(composites[0]) || composites[0].call.Pos() <= transform.call.Pos() ||
		!state.canReach(transform.call, composites[0].call) || !state.dominates(transform.call, composites[0].call) {
		return false
	}
	for _, binding := range producers[1] {
		if binding.call.Pos() >= composites[0].call.Pos() || !state.canReach(binding.call, composites[0].call) {
			return false
		}
	}
	if len(producers[1]) == 2 {
		if !(producers[1][0].call.Pos() < alternatePosition && producers[1][1].call.Pos() > transform.call.Pos()) {
			return false
		}
	}
	gateCall := producers[0][0].call
	gateConsumers := []*ast.CallExpr{transform.call, composites[0].call}
	for _, alternate := range alternates {
		gateConsumers = append(gateConsumers, alternate.call)
	}
	for _, binding := range producers[1] {
		gateConsumers = append(gateConsumers, binding.call)
	}
	for _, call := range gateConsumers {
		if !state.dominates(gateCall, call) {
			return false
		}
	}
	upCalls := make([]*ast.CallExpr, 0, len(producers[1]))
	for _, binding := range producers[1] {
		upCalls = append(upCalls, binding.call)
	}
	if !state.setDominates(upCalls, composites[0].call) && !state.fallbackCompletesUp(owner, producers[1], composites[0].call) {
		return false
	}
	if len(alternates) != 1 || !state.dominates(upCalls[0], alternates[0].call) {
		return false
	}
	compositeValues := ps6081ResultValues(composites[0], compositeConsumer)
	if len(compositeValues) == 0 {
		return false
	}
	finalConsumer := &contract.FinalConsumer
	if finalConsumer.ResultMode != config.SharedFanOutDirectReturn {
		return false
	}
	finalCalls := state.calls(owner.Body, finalConsumer.Callable, finalConsumer.Kind, finalConsumer.ReceiverPath, root)
	if len(finalCalls) != 2 {
		return false
	}
	earlyFinal, normalFinal := false, false
	for _, call := range finalCalls {
		if _, ok := parents[call].(*ast.ReturnStmt); !ok {
			return false
		}
		if state.consumerMatches(call, finalConsumer, []ps6081ValueSet{left}) && call.Pos() > alternatePosition && call.Pos() < transform.call.Pos() && state.canReach(alternates[0].call, call) {
			earlyFinal = true
		} else if state.consumerMatches(call, finalConsumer, []ps6081ValueSet{compositeValues}) && call.Pos() > composites[0].call.Pos() && state.canReach(composites[0].call, call) {
			normalFinal = true
		} else {
			return false
		}
	}
	for _, call := range finalCalls {
		if !state.dominates(gateCall, call) {
			return false
		}
	}
	permittedOwnerCalls := make(map[*ast.CallExpr]bool)
	for _, bindings := range producers {
		for _, binding := range bindings {
			permittedOwnerCalls[binding.call] = true
		}
	}
	for _, call := range finalCalls {
		permittedOwnerCalls[call] = true
	}
	for _, guard := range contract.AllowedEligibilityGuards {
		ast.Inspect(owner.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok && len(call.Args) == 0 && state.reachable[call] && ps6081CallKindMatches(state.pass, call, guard.Callable, guard.Kind) {
				permittedOwnerCalls[call] = true
			}
			return true
		})
	}
	if !earlyFinal || !normalFinal || !state.ownerStableAfterProducer(owner, root, producers[0][0], contract.Producers[0].InputArgument, permittedOwnerCalls) {
		return false
	}
	allowedCallables := map[string]bool{
		ps6081CallableKey(contract.Producers[0].Callable, contract.Producers[0].Kind):           true,
		ps6081CallableKey(contract.Producers[1].Callable, contract.Producers[1].Kind):           true,
		ps6081CallableKey(contract.CompositeConsumer.Callable, contract.CompositeConsumer.Kind): true,
		ps6081CallableKey(contract.FinalConsumer.Callable, contract.FinalConsumer.Kind):         true,
	}
	for index := range contract.TransformConsumers {
		consumer := &contract.TransformConsumers[index]
		allowedCallables[ps6081CallableKey(consumer.Callable, consumer.Kind)] = true
	}
	for index := range contract.AlternateConsumers {
		consumer := &contract.AlternateConsumers[index]
		allowedCallables[ps6081CallableKey(consumer.Callable, consumer.Kind)] = true
	}
	for _, guard := range contract.AllowedEligibilityGuards {
		allowedCallables[ps6081CallableKey(guard.Callable, guard.Kind)] = true
	}
	allCallsConfigured := true
	ast.Inspect(owner.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		function, signature, typed := typedCallee(state.pass, call.Fun)
		if !typed {
			allCallsConfigured = false
			return false
		}
		kind := config.SharedFanOutFunction
		if signature.Recv() != nil {
			kind = config.SharedFanOutMethod
		}
		if !allowedCallables[ps6081CallableKey(ps6081ObjectID(function), kind)] {
			allCallsConfigured = false
			return false
		}
		return true
	})
	if !allCallsConfigured {
		return false
	}
	participants := make(map[types.Object]bool)
	allowedUses := make(map[*ast.Ident]bool)
	for _, bindings := range producers {
		for _, binding := range bindings {
			participants[binding.data] = true
			ps6081AllowBindingDefinition(state.pass, binding, allowedUses)
		}
	}
	participants[transform.data] = true
	participants[composites[0].data] = true
	ps6081AllowBindingDefinition(state.pass, transform, allowedUses)
	ps6081AllowBindingDefinition(state.pass, composites[0], allowedUses)
	ps6081AllowConsumerOperands(state.pass, transform.call, transformConsumer, allowedUses)
	for _, alternate := range alternates {
		ps6081AllowConsumerOperands(state.pass, alternate.call, alternate.consumer, allowedUses)
	}
	ps6081AllowConsumerOperands(state.pass, composites[0].call, compositeConsumer, allowedUses)
	for _, call := range finalCalls {
		ps6081AllowConsumerOperands(state.pass, call, finalConsumer, allowedUses)
	}
	return ps6081OnlyExactValueUses(state.pass, owner.Body, participants, allowedUses, parents)
}

func ps6081CallableKey(callable string, kind config.SharedFanOutCallableKind) string {
	return callable + "\x00" + string(kind)
}

func ps6081AllowBindingDefinition(pass *analysis.Pass, binding ps6081Binding, allowed map[*ast.Ident]bool) {
	assignment, _ := binding.block.List[binding.index].(*ast.AssignStmt)
	if assignment == nil {
		return
	}
	for _, expression := range assignment.Lhs {
		identifier, _ := ps2110Unparen(expression).(*ast.Ident)
		if identifier != nil && pass.TypesInfo.ObjectOf(identifier) == binding.data {
			allowed[identifier] = true
		}
	}
}

func ps6081AllowConsumerOperands(pass *analysis.Pass, call *ast.CallExpr, consumer *config.SharedFanOutConsumer, allowed map[*ast.Ident]bool) {
	if consumer == nil {
		return
	}
	for _, position := range consumer.OperandArguments {
		if position > 0 && position <= len(call.Args) {
			ps6081AllowIdentifiers(call.Args[position-1], allowed)
		}
	}
	if consumer.CollectionArgument <= 0 || consumer.CollectionArgument > len(call.Args) {
		return
	}
	collection, _ := ps2110Unparen(call.Args[consumer.CollectionArgument-1]).(*ast.CompositeLit)
	if collection == nil {
		return
	}
	for _, position := range consumer.CollectionIndexes {
		if position > 0 && position <= len(collection.Elts) {
			ps6081AllowIdentifiers(collection.Elts[position-1], allowed)
		}
	}
}

func ps6081AllowIdentifiers(node ast.Node, allowed map[*ast.Ident]bool) {
	ast.Inspect(node, func(node ast.Node) bool {
		if identifier, ok := node.(*ast.Ident); ok {
			allowed[identifier] = true
		}
		return true
	})
}

func ps6081OnlyExactValueUses(pass *analysis.Pass, body *ast.BlockStmt, participants map[types.Object]bool, allowed map[*ast.Ident]bool, parents map[ast.Node]ast.Node) bool {
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || allowed[identifier] {
			return valid
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		if !participants[object] {
			return valid
		}
		if spec, ok := parents[identifier].(*ast.ValueSpec); ok && pass.TypesInfo.Defs[identifier] == object && len(spec.Values) == 0 {
			return true
		}
		if binary, ok := parents[identifier].(*ast.BinaryExpr); ok &&
			(binary.Op == token.EQL || binary.Op == token.NEQ) && (ps6081Nil(binary.X) || ps6081Nil(binary.Y)) {
			return true
		}
		valid = false
		return false
	})
	return valid
}

type ps6081ValueSet map[types.Object]map[int]bool

func ps6081DirectValues(bindings []ps6081Binding) ps6081ValueSet {
	values := make(ps6081ValueSet)
	for _, binding := range bindings {
		values.add(binding.data, 0)
	}
	return values
}

func ps6081ResultValues(binding ps6081Binding, consumer *config.SharedFanOutConsumer) ps6081ValueSet {
	values := make(ps6081ValueSet)
	if binding.data == nil || consumer == nil {
		return values
	}
	if consumer.ResultMode == config.SharedFanOutCollectionError {
		for _, index := range consumer.ResultCollectionIndexes {
			values.add(binding.data, index)
		}
	} else {
		values.add(binding.data, 0)
	}
	return values
}

func (values ps6081ValueSet) add(object types.Object, index int) {
	if object == nil {
		return
	}
	if values[object] == nil {
		values[object] = make(map[int]bool)
	}
	values[object][index] = true
}

func ps6081ValueMatches(pass *analysis.Pass, expression ast.Expr, values ps6081ValueSet) bool {
	if index, ok := ps2110Unparen(expression).(*ast.IndexExpr); ok {
		object := ps6081ObjectExpr(pass, index.X)
		basic, ok := ps2110Unparen(index.Index).(*ast.BasicLit)
		if !ok || basic.Kind != token.INT {
			return false
		}
		value, err := strconv.Atoi(basic.Value)
		return err == nil && values[object][value+1]
	}
	return values[ps6081ObjectExpr(pass, expression)][0]
}

func ps6081ConsumerForCall(pass *analysis.Pass, call *ast.CallExpr, consumers []config.SharedFanOutConsumer) *config.SharedFanOutConsumer {
	for index := range consumers {
		if ps6081CallKindMatches(pass, call, consumers[index].Callable, consumers[index].Kind) {
			return &consumers[index]
		}
	}
	return nil
}

func (state *ps6081State) calls(body *ast.BlockStmt, callable string, kind config.SharedFanOutCallableKind, path string, root types.Object) []*ast.CallExpr {
	var result []*ast.CallExpr
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !ps6081CallKindMatches(state.pass, call, callable, kind) || state.reachable != nil && !state.reachable[call] {
			return true
		}
		var receiver ast.Expr
		if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
			receiver = selector.X
		}
		if ps6081ReceiverMatches(state.pass, receiver, path, root) {
			result = append(result, call)
		}
		return true
	})
	return result
}

func (state *ps6081State) canReach(before, after *ast.CallExpr) bool {
	if state.flow == nil || before == nil || after == nil {
		return false
	}
	budget := &ps6089LifecycleBudget{limit: 4096, complete: true}
	return ps6089PositionCanReachBudget(state.pass, state.flow, before.Pos(), after.Pos(), budget) && budget.complete
}

func (state *ps6081State) dominates(barrier, target *ast.CallExpr) bool {
	return state.setDominates([]*ast.CallExpr{barrier}, target)
}

func (state *ps6081State) setDominates(barriers []*ast.CallExpr, target *ast.CallExpr) bool {
	if target == nil || len(barriers) == 0 {
		return false
	}
	positions := make([]token.Pos, len(barriers))
	for index, barrier := range barriers {
		if barrier == nil {
			return false
		}
		positions[index] = barrier.Pos()
	}
	return state.positionsDominate(positions, target.Pos())
}

func (state *ps6081State) positionsDominate(barriers []token.Pos, target token.Pos) bool {
	if state.flow == nil || state.flow.graph == nil || target == token.NoPos || len(barriers) == 0 {
		return false
	}
	targetBlock := state.flow.blocks[target]
	if targetBlock == nil || !targetBlock.Live || len(state.flow.graph.Blocks) == 0 {
		return false
	}
	blocked := make(map[*cfg.Block][]token.Pos, len(barriers))
	for _, barrier := range barriers {
		block := ps6081FlowBlockAt(state.flow, barrier)
		if block == nil || !block.Live {
			return false
		}
		blocked[block] = append(blocked[block], barrier)
	}
	queue := []*cfg.Block{state.flow.graph.Blocks[0]}
	seen := make(map[*cfg.Block]bool)
	budget := &ps6089LifecycleBudget{limit: 4096, complete: true}
	for len(queue) > 0 {
		if !budget.take(1) {
			return false
		}
		block := queue[0]
		queue = queue[1:]
		if block == nil || seen[block] || !block.Live {
			continue
		}
		seen[block] = true
		if block == targetBlock {
			unblocked := true
			for _, position := range blocked[block] {
				if position < target {
					unblocked = false
					break
				}
			}
			if unblocked {
				return false
			}
			continue
		}
		if len(blocked[block]) != 0 {
			continue
		}
		queue = append(queue, ps6089LifecycleSuccessors(state.pass, block, budget)...)
	}
	return budget.complete
}

func ps6081FlowBlockAt(flow *ps6089LifecycleFlow, position token.Pos) *cfg.Block {
	return flow.blocks[position]
}

func (state *ps6081State) fallbackCompletesUp(owner *ast.FuncDecl, bindings []ps6081Binding, target *ast.CallExpr) bool {
	if owner == nil || len(bindings) != 2 || bindings[0].data == nil || bindings[0].data != bindings[1].data ||
		!state.canReach(bindings[0].call, target) || !state.canReach(bindings[1].call, target) {
		return false
	}
	parents := ps6087Parents(owner.Body)
	conditional, ok := parents[bindings[1].block].(*ast.IfStmt)
	if !ok || conditional.Init != nil || conditional.Else != nil || bindings[1].block != conditional.Body ||
		bindings[1].index != 0 || len(conditional.Body.List) != 2 ||
		!ps6081NilComparison(state.pass, conditional.Cond, bindings[1].data, token.EQL) ||
		!state.positionsDominate([]token.Pos{conditional.Cond.Pos()}, target.Pos()) {
		return false
	}
	declarationPosition := token.NoPos
	ast.Inspect(owner.Body, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok || len(spec.Values) != 0 {
			return true
		}
		for _, name := range spec.Names {
			if state.pass.TypesInfo.ObjectOf(name) == bindings[1].data {
				declarationPosition = spec.Pos()
				return false
			}
		}
		return true
	})
	return declarationPosition != token.NoPos && declarationPosition < bindings[0].call.Pos() && declarationPosition < bindings[1].call.Pos()
}

func ps6081NilComparison(pass *analysis.Pass, expression ast.Expr, object types.Object, operator token.Token) bool {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	return ok && binary.Op == operator &&
		(ps6081ObjectExpr(pass, binary.X) == object && ps6081Nil(binary.Y) || ps6081ObjectExpr(pass, binary.Y) == object && ps6081Nil(binary.X))
}

func ps6081CallKindMatches(pass *analysis.Pass, call *ast.CallExpr, callable string, kind config.SharedFanOutCallableKind) bool {
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || ps6081ObjectID(function) != callable {
		return false
	}
	return kind == config.SharedFanOutMethod && signature.Recv() != nil || kind == config.SharedFanOutFunction && signature.Recv() == nil
}

func ps6081DeclarationKindMatches(pass *analysis.Pass, declaration *ast.FuncDecl, kind config.SharedFanOutCallableKind) bool {
	function, _ := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	signature, _ := function.Type().(*types.Signature)
	return signature != nil && (kind == config.SharedFanOutMethod && signature.Recv() != nil || kind == config.SharedFanOutFunction && signature.Recv() == nil)
}

func ps6081ProducerRolesTyped(signature *types.Signature, producer *config.SharedFanOutProducer) bool {
	if signature == nil {
		return false
	}
	if producer.InputArgument <= 0 || producer.InputArgument > signature.Params().Len() || !ps6081ReferenceType(signature.Params().At(producer.InputArgument-1).Type()) {
		return false
	}
	if signature.Recv() == nil {
		return producer.ReceiverPath == "" && producer.WeightReceiverField == "" && len(producer.MetadataReceiverFields) == 0 && len(producer.DomainReceiverFields) == 0 && producer.WorkReceiverField == "" &&
			producer.WeightArgument > 0 && producer.WeightArgument <= signature.Params().Len() && ps6081ReferenceType(signature.Params().At(producer.WeightArgument-1).Type()) &&
			ps6081IntegerParameters(signature, producer.DomainArguments) && producer.WorkArgument > 0 && producer.WorkArgument <= signature.Params().Len() && ps6081IntegerType(signature.Params().At(producer.WorkArgument-1).Type())
	}
	weight, ok := ps6081ResolveField(signature.Recv().Type(), producer.WeightReceiverField)
	if !ok || !ps6081OwnedStorageType(weight.Type()) {
		return false
	}
	work, ok := ps6081ResolveField(signature.Recv().Type(), producer.WorkReceiverField)
	if !ok || !ps6081IntegerType(work.Type()) {
		return false
	}
	for _, path := range producer.MetadataReceiverFields {
		if _, ok := ps6081ResolveField(signature.Recv().Type(), path); !ok {
			return false
		}
	}
	for _, path := range producer.DomainReceiverFields {
		field, ok := ps6081ResolveField(signature.Recv().Type(), path)
		if !ok || !ps6081IntegerType(field.Type()) {
			return false
		}
	}
	return true
}

func ps6081IntegerParameters(signature *types.Signature, positions []int) bool {
	for _, position := range positions {
		if position <= 0 || position > signature.Params().Len() || !ps6081IntegerType(signature.Params().At(position-1).Type()) {
			return false
		}
	}
	return true
}

func ps6081IntegerType(value types.Type) bool {
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsInteger != 0
}

func ps6081ReferenceType(value types.Type) bool {
	switch types.Unalias(value).Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Interface:
		return true
	}
	return false
}

func ps6081OwnedStorageType(value types.Type) bool {
	switch types.Unalias(value).Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map:
		return true
	}
	return false
}

func ps6081ResolveField(value types.Type, path string) (*types.Var, bool) {
	parts := strings.Split(path, ".")
	for pathIndex, part := range parts {
		value = types.Unalias(value)
		if pointer, ok := value.(*types.Pointer); ok {
			value = types.Unalias(pointer.Elem())
		}
		named, ok := value.(*types.Named)
		if ok {
			value = named.Underlying()
		}
		structure, ok := value.(*types.Struct)
		if !ok {
			return nil, false
		}
		var field *types.Var
		for index := 0; index < structure.NumFields(); index++ {
			if structure.Field(index).Name() == part {
				field = structure.Field(index)
				break
			}
		}
		if field == nil {
			return nil, false
		}
		value = field.Type()
		if pathIndex == len(parts)-1 {
			return field, true
		}
	}
	return nil, false
}

func (state *ps6081State) ownerStableAfterProducer(owner *ast.FuncDecl, root types.Object, first ps6081Binding, inputPosition int, permittedOwnerCalls map[*ast.CallExpr]bool) bool {
	if inputPosition <= 0 || inputPosition > len(first.call.Args) {
		return false
	}
	input := ps6081ObjectExpr(state.pass, first.call.Args[inputPosition-1])
	if input == nil || root == nil {
		return false
	}
	type aliasEdge struct{ left, right types.Object }
	protected := map[types.Object]bool{root: true, input: true}
	var edges []aliasEdge
	ast.Inspect(owner.Body, func(node ast.Node) bool {
		if node == nil || node.Pos() >= first.call.Pos() {
			return false
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.AssignStmt:
			if len(value.Lhs) == len(value.Rhs) || len(value.Rhs) == 1 {
				for index := range value.Lhs {
					left := ps6081AliasDefinitionObject(state.pass, value.Lhs[index])
					if left == nil {
						left = ps6081RootObject(state.pass, value.Lhs[index])
					}
					rightIndex := index
					if len(value.Rhs) == 1 {
						rightIndex = 0
					}
					if call, isCall := ps2110Unparen(value.Rhs[rightIndex]).(*ast.CallExpr); isCall && call == first.call {
						continue
					}
					for right := range ps6081ReferencedObjects(state.pass, value.Rhs[rightIndex]) {
						if left != nil {
							edges = append(edges, aliasEdge{left: left, right: right})
						}
					}
				}
			}
		case *ast.RangeStmt:
			for _, expression := range []ast.Expr{value.Key, value.Value} {
				left := ps6081AliasDefinitionObject(state.pass, expression)
				for right := range ps6081ReferencedObjects(state.pass, value.X) {
					if left != nil {
						edges = append(edges, aliasEdge{left: left, right: right})
					}
				}
			}
		case *ast.ValueSpec:
			if len(value.Names) == len(value.Values) || len(value.Values) == 1 {
				for index, name := range value.Names {
					left := state.pass.TypesInfo.ObjectOf(name)
					rightIndex := index
					if len(value.Values) == 1 {
						rightIndex = 0
					}
					if call, isCall := ps2110Unparen(value.Values[rightIndex]).(*ast.CallExpr); isCall && call == first.call {
						continue
					}
					for right := range ps6081ReferencedObjects(state.pass, value.Values[rightIndex]) {
						if left != nil {
							edges = append(edges, aliasEdge{left: left, right: right})
						}
					}
				}
			}
		}
		return true
	})
	changed := true
	for changed {
		changed = false
		for _, edge := range edges {
			if protected[edge.left] || protected[edge.right] {
				if !protected[edge.left] || !protected[edge.right] {
					protected[edge.left], protected[edge.right], changed = true, true, true
				}
			}
		}
	}
	stable := true
	ast.Inspect(owner.Body, func(node ast.Node) bool {
		if !stable || node == nil || node.Pos() <= first.call.Pos() {
			return stable
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			if ps6081ReferencesProtected(state.pass, value, protected) {
				stable = false
			}
			return false
		case *ast.GoStmt, *ast.DeferStmt:
			stable = false
			return false
		case *ast.SendStmt:
			if ps6081ReferencesProtected(state.pass, value.Value, protected) {
				stable = false
				return false
			}
		case *ast.ReturnStmt:
			for _, expression := range value.Results {
				if call, ok := ps2110Unparen(expression).(*ast.CallExpr); ok && permittedOwnerCalls[call] {
					continue
				}
				if ps6081ReferencesProtected(state.pass, expression, protected) {
					stable = false
					return false
				}
			}
		case *ast.IncDecStmt:
			if protected[ps6081RootObject(state.pass, value.X)] {
				stable = false
				return false
			}
		case *ast.AssignStmt:
			for _, expression := range value.Lhs {
				if ps6081ReferencesProtected(state.pass, expression, protected) {
					stable = false
					return false
				}
			}
			for _, expression := range value.Rhs {
				if call, ok := ps2110Unparen(expression).(*ast.CallExpr); ok {
					if permittedOwnerCalls[call] {
						continue
					}
				}
				if ps6081ReferencesProtected(state.pass, expression, protected) {
					stable = false
					return false
				}
			}
		case *ast.ValueSpec:
			for _, expression := range value.Values {
				if ps6081ReferencesProtected(state.pass, expression, protected) {
					stable = false
					return false
				}
			}
		case *ast.RangeStmt:
			if ps6081ReferencesProtected(state.pass, value.X, protected) {
				stable = false
				return false
			}
		case *ast.CallExpr:
			protectedArgument := ps6081ReferencesProtected(state.pass, value.Fun, protected)
			for _, argument := range value.Args {
				protectedArgument = protectedArgument || ps6081ReferencesProtected(state.pass, argument, protected)
			}
			if protectedArgument && !permittedOwnerCalls[value] {
				stable = false
				return false
			}
		}
		return true
	})
	return stable
}

func ps6081AliasDefinitionObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	identifier, _ := ps2110Unparen(expression).(*ast.Ident)
	if identifier == nil || identifier.Name == "_" {
		return nil
	}
	return pass.TypesInfo.ObjectOf(identifier)
}

func ps6081ReferencedObjects(pass *analysis.Pass, node ast.Node) map[types.Object]bool {
	result := make(map[types.Object]bool)
	ast.Inspect(node, func(node ast.Node) bool {
		if identifier, ok := node.(*ast.Ident); ok {
			if object := pass.TypesInfo.ObjectOf(identifier); object != nil {
				result[object] = true
			}
		}
		return true
	})
	return result
}

func ps6081ReferencesProtected(pass *analysis.Pass, node ast.Node, protected map[types.Object]bool) bool {
	for object := range ps6081ReferencedObjects(pass, node) {
		if protected[object] {
			return true
		}
	}
	return false
}

func ps6081RootObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return pass.TypesInfo.ObjectOf(value)
	case *ast.SelectorExpr:
		return ps6081RootObject(pass, value.X)
	case *ast.IndexExpr:
		return ps6081RootObject(pass, value.X)
	case *ast.StarExpr:
		return ps6081RootObject(pass, value.X)
	case *ast.SliceExpr:
		return ps6081RootObject(pass, value.X)
	case *ast.UnaryExpr:
		return ps6081RootObject(pass, value.X)
	}
	return nil
}

func (state *ps6081State) consumerMatches(call *ast.CallExpr, consumer *config.SharedFanOutConsumer, operands []ps6081ValueSet) bool {
	if !ps6081OperationMatches(state.pass, call, consumer) {
		return false
	}
	if len(consumer.OperandArguments) != 0 {
		if len(consumer.OperandArguments) != len(operands) {
			return false
		}
		for index, position := range consumer.OperandArguments {
			if position <= 0 || position > len(call.Args) || !ps6081ValueMatches(state.pass, call.Args[position-1], operands[index]) {
				return false
			}
		}
		return true
	}
	if consumer.CollectionArgument <= 0 || consumer.CollectionArgument > len(call.Args) || len(consumer.CollectionIndexes) != len(operands) {
		return false
	}
	collection, ok := ps2110Unparen(call.Args[consumer.CollectionArgument-1]).(*ast.CompositeLit)
	if !ok {
		return false
	}
	for index, position := range consumer.CollectionIndexes {
		if position <= 0 || position > len(collection.Elts) {
			return false
		}
		expression := collection.Elts[position-1]
		if !ps6081ValueMatches(state.pass, expression, operands[index]) {
			return false
		}
	}
	return true
}

func ps6081OperationMatches(pass *analysis.Pass, call *ast.CallExpr, consumer *config.SharedFanOutConsumer) bool {
	if consumer.OperationArgument == 0 {
		return true
	}
	if consumer.OperationArgument > len(call.Args) {
		return false
	}
	value, ok := ps6081ObjectExpr(pass, call.Args[consumer.OperationArgument-1]).(*types.Const)
	return ok && ps6081ObjectID(value) == consumer.OperationConstant && value.Val().ExactString() == consumer.OperationConstantValue
}

func (state *ps6081State) producerSourceSafe(contract *config.SharedFanOutContract, producer *config.SharedFanOutProducer, bindings []ps6081Binding) bool {
	for _, binding := range bindings {
		function, signature, ok := typedCallee(state.pass, binding.call.Fun)
		if !ok {
			return false
		}
		declaration := state.declarations[function]
		if declaration == nil {
			continue
		}
		domains := ps6081ParameterObjects(signature, producer.DomainArguments)
		work := ps6081ParameterObject(signature, producer.WorkArgument)
		if domains == nil || work == nil || state.routeCount(declaration, domains, work, producer.Route, make(map[*types.Func]bool)) != 1 ||
			!state.routeCoversSuccessfulReturns(declaration, domains, work, producer.Route, producer.DataResult, producer.ErrorResult, make(map[*types.Func]bool)) ||
			!state.localSafe(declaration, producer, contract) {
			return false
		}
	}
	return true
}

func (state *ps6081State) routeCount(declaration *ast.FuncDecl, domains []types.Object, work types.Object, route []config.SharedFanOutRouteStep, active map[*types.Func]bool) int {
	object, _ := state.pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	if object == nil || active[object] || len(route) == 0 {
		return 0
	}
	active[object] = true
	defer delete(active, object)
	count := 0
	ps6081ReachableCalls(state.pass, declaration, func(call *ast.CallExpr) {
		if !ps6081CallKindMatches(state.pass, call, route[0].Callable, route[0].Kind) ||
			!ps6081ArgumentsAreObjects(state.pass, call, route[0].DomainArguments, domains) ||
			route[0].WorkArgument > len(call.Args) || ps6081ObjectExpr(state.pass, call.Args[route[0].WorkArgument-1]) != work {
			return
		}
		if len(route) == 1 {
			if route[0].CallbackArgument <= len(call.Args) {
				if _, ok := ps2110Unparen(call.Args[route[0].CallbackArgument-1]).(*ast.FuncLit); ok {
					count++
				}
			}
			return
		}
		callee, signature, ok := typedCallee(state.pass, call.Fun)
		if !ok {
			return
		}
		if next := state.declarations[callee]; next != nil {
			count += state.routeCount(next, ps6081ParameterObjects(signature, route[0].DomainArguments), ps6081ParameterObject(signature, route[0].WorkArgument), route[1:], active)
		} else {
			count++ // remainder is an explicitly asserted imported/opaque route
		}
	})
	return count
}

func (state *ps6081State) routeCoversSuccessfulReturns(declaration *ast.FuncDecl, domains []types.Object, work types.Object, route []config.SharedFanOutRouteStep, dataResult, errorResult int, active map[*types.Func]bool) bool {
	object, _ := state.pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	if object == nil || active[object] || len(route) == 0 {
		return false
	}
	active[object] = true
	defer delete(active, object)
	var matches []*ast.CallExpr
	ps6081ReachableCalls(state.pass, declaration, func(call *ast.CallExpr) {
		if ps6081CallKindMatches(state.pass, call, route[0].Callable, route[0].Kind) &&
			ps6081ArgumentsAreObjects(state.pass, call, route[0].DomainArguments, domains) &&
			route[0].WorkArgument <= len(call.Args) && ps6081ObjectExpr(state.pass, call.Args[route[0].WorkArgument-1]) == work {
			matches = append(matches, call)
		}
	})
	if len(matches) != 1 {
		return false
	}
	parents := ps6087Parents(declaration.Body)
	switch parents[matches[0]].(type) {
	case *ast.GoStmt, *ast.DeferStmt:
		return false
	}
	if !state.routeCallDominatesSuccessfulReturns(declaration, matches[0], dataResult, errorResult) {
		return false
	}
	if len(route) == 1 {
		if route[0].CallbackArgument > len(matches[0].Args) {
			return false
		}
		_, literal := ps2110Unparen(matches[0].Args[route[0].CallbackArgument-1]).(*ast.FuncLit)
		return literal
	}
	callee, signature, ok := typedCallee(state.pass, matches[0].Fun)
	if !ok {
		return false
	}
	next := state.declarations[callee]
	if next == nil {
		return true // the remaining imported route is covered by the explicit contract assertions
	}
	nextDomains := ps6081ParameterObjects(signature, route[0].DomainArguments)
	nextWork := ps6081ParameterObject(signature, route[0].WorkArgument)
	return nextDomains != nil && nextWork != nil && state.routeCoversSuccessfulReturns(next, nextDomains, nextWork, route[1:], dataResult, errorResult, active)
}

func (state *ps6081State) routeCallDominatesSuccessfulReturns(declaration *ast.FuncDecl, routeCall *ast.CallExpr, dataResult, errorResult int) bool {
	object, _ := state.pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	signature, _ := object.Type().(*types.Signature)
	if signature == nil || dataResult <= 0 || errorResult <= 0 || dataResult > signature.Results().Len() || errorResult > signature.Results().Len() {
		return false
	}
	flow := ps6089NewLifecycleFlow(state.pass, declaration.Body)
	flowState := &ps6081State{pass: state.pass, flow: flow}
	covered := true
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if !covered {
			return false
		}
		if literal, ok := node.(*ast.FuncLit); ok && literal.Body != declaration.Body {
			return false
		}
		returned, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		if !flowState.positionsDominate([]token.Pos{routeCall.Pos()}, returned.End()) {
			covered = false
			return false
		}
		return true
	})
	return covered
}

func (state *ps6081State) localSafe(declaration *ast.FuncDecl, producer *config.SharedFanOutProducer, contract *config.SharedFanOutContract) bool {
	object, _ := state.pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	signature, _ := object.Type().(*types.Signature)
	if signature == nil || producer.InputArgument > signature.Params().Len() {
		return false
	}
	protected := map[types.Object]bool{signature.Params().At(producer.InputArgument - 1): true}
	if producer.WeightArgument > 0 && producer.WeightArgument <= signature.Params().Len() {
		protected[signature.Params().At(producer.WeightArgument-1)] = true
	}
	valid := true
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		switch value := node.(type) {
		case *ast.GoStmt, *ast.DeferStmt, *ast.SendStmt:
			valid = false
		case *ast.AssignStmt:
			for index, lhs := range value.Lhs {
				if ps6081MutatesObjects(state.pass, lhs, protected) || ps6081ExternalStore(state.pass, lhs) {
					valid = false
				}
				if index < len(value.Rhs) && protected[ps6081ObjectExpr(state.pass, value.Rhs[index])] {
					if identifier, ok := ps2110Unparen(lhs).(*ast.Ident); ok {
						protected[state.pass.TypesInfo.ObjectOf(identifier)] = true
					}
				}
			}
		case *ast.ReturnStmt:
			for _, result := range value.Results {
				if protected[ps6081ObjectExpr(state.pass, result)] {
					valid = false
				}
			}
		case *ast.CallExpr:
			id := ps6087FunctionID(state.pass, value)
			if ps6081TransferName(id) || (!ps6081SafeBuiltin(state.pass, value) && id != contract.FanOutHelper && !ps6081RouteContains(producer.Route, id)) {
				valid = false
			}
		}
		return valid
	})
	return valid
}

func ps6081ReachableCalls(pass *analysis.Pass, declaration *ast.FuncDecl, visit func(*ast.CallExpr)) {
	reachable := ps6099ReachableCalls(pass, declaration, ps6087Parents(declaration.Body))
	calls := make([]*ast.CallExpr, 0, len(reachable))
	for call := range reachable {
		calls = append(calls, call)
	}
	slices.SortFunc(calls, func(left, right *ast.CallExpr) int { return int(left.Pos() - right.Pos()) })
	for _, call := range calls {
		visit(call)
	}
}

func ps6081AlternativeBindings(bindings []ps6081Binding) bool {
	return len(bindings) <= 1 || bindings[0].block != bindings[1].block
}

func ps6081ReceiverObject(pass *analysis.Pass, declaration *ast.FuncDecl) types.Object {
	if declaration.Recv == nil || len(declaration.Recv.List) != 1 || len(declaration.Recv.List[0].Names) != 1 {
		return nil
	}
	return pass.TypesInfo.Defs[declaration.Recv.List[0].Names[0]]
}

func ps6081ReceiverMatches(pass *analysis.Pass, expression ast.Expr, path string, root types.Object) bool {
	if path == "" {
		return true
	}
	parts := strings.Split(path, ".")
	for index := len(parts) - 1; index >= 0; index-- {
		selector, ok := ps2110Unparen(expression).(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != parts[index] {
			return false
		}
		expression = selector.X
	}
	return ps6081ObjectExpr(pass, expression) == root
}

func ps6081SameArgument(pass *analysis.Pass, left *ast.CallExpr, leftPosition int, right *ast.CallExpr, rightPosition int) bool {
	return leftPosition > 0 && leftPosition <= len(left.Args) && rightPosition > 0 && rightPosition <= len(right.Args) && ps6081SameExpr(pass, left.Args[leftPosition-1], right.Args[rightPosition-1])
}

func ps6081SameExpr(pass *analysis.Pass, left, right ast.Expr) bool {
	if left == nil || right == nil {
		return left == right
	}
	leftObject, rightObject := ps6081ObjectExpr(pass, left), ps6081ObjectExpr(pass, right)
	if leftObject != nil || rightObject != nil {
		return leftObject != nil && leftObject == rightObject
	}
	leftValue, rightValue := pass.TypesInfo.Types[left].Value, pass.TypesInfo.Types[right].Value
	return leftValue != nil && rightValue != nil && leftValue.ExactString() == rightValue.ExactString() && types.Identical(pass.TypesInfo.TypeOf(left), pass.TypesInfo.TypeOf(right))
}

func ps6081ObjectExpr(pass *analysis.Pass, expression ast.Expr) types.Object {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return pass.TypesInfo.ObjectOf(value)
	case *ast.SelectorExpr:
		return pass.TypesInfo.ObjectOf(value.Sel)
	}
	return nil
}

func ps6081Nil(expression ast.Expr) bool {
	id, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && id.Name == "nil"
}

func ps6081ObjectID(object types.Object) string {
	if object == nil || object.Pkg() == nil {
		return ""
	}
	if function, ok := object.(*types.Func); ok {
		signature, _ := function.Type().(*types.Signature)
		if signature != nil && signature.Recv() != nil {
			if named := ps6087Named(signature.Recv().Type()); named != nil {
				return object.Pkg().Path() + "." + named.Obj().Name() + "." + object.Name()
			}
		}
	}
	return object.Pkg().Path() + "." + object.Name()
}

func ps6081ParameterObjects(signature *types.Signature, positions []int) []types.Object {
	objects := make([]types.Object, len(positions))
	for index, position := range positions {
		if position <= 0 || position > signature.Params().Len() {
			return nil
		}
		objects[index] = signature.Params().At(position - 1)
	}
	return objects
}

func ps6081ParameterObject(signature *types.Signature, position int) types.Object {
	if signature == nil || position <= 0 || position > signature.Params().Len() {
		return nil
	}
	return signature.Params().At(position - 1)
}

func ps6081ArgumentsAreObjects(pass *analysis.Pass, call *ast.CallExpr, positions []int, objects []types.Object) bool {
	if len(positions) != len(objects) {
		return false
	}
	for index, position := range positions {
		if position <= 0 || position > len(call.Args) || ps6081ObjectExpr(pass, call.Args[position-1]) != objects[index] {
			return false
		}
	}
	return true
}

func ps6081MutatesObjects(pass *analysis.Pass, expression ast.Expr, objects map[types.Object]bool) bool {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return objects[pass.TypesInfo.ObjectOf(value)]
	case *ast.IndexExpr:
		return objects[ps6081ObjectExpr(pass, value.X)]
	case *ast.SelectorExpr:
		return objects[ps6081ObjectExpr(pass, value.X)]
	case *ast.StarExpr:
		return objects[ps6081ObjectExpr(pass, value.X)]
	}
	return false
}

func ps6081ExternalStore(pass *analysis.Pass, expression ast.Expr) bool {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		return object != nil && object.Pkg() == pass.Pkg && object.Parent() == pass.Pkg.Scope()
	case *ast.SelectorExpr:
		return true
	case *ast.IndexExpr:
		if _, ok := types.Unalias(pass.TypesInfo.TypeOf(value.X)).Underlying().(*types.Map); ok {
			return true
		}
		switch base := ps2110Unparen(value.X).(type) {
		case *ast.SelectorExpr:
			return true
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(base)
			return object != nil && object.Pkg() == pass.Pkg && object.Parent() == pass.Pkg.Scope()
		}
	}
	return false
}

func ps6081SafeBuiltin(pass *analysis.Pass, call *ast.CallExpr) bool {
	id, _ := ps2110Unparen(call.Fun).(*ast.Ident)
	builtin, _ := pass.TypesInfo.Uses[id].(*types.Builtin)
	if builtin == nil {
		return false
	}
	switch builtin.Name() {
	case "make", "new", "len", "cap", "min", "max":
		return true
	}
	return false
}

func ps6081RouteContains(route []config.SharedFanOutRouteStep, id string) bool {
	for _, step := range route {
		if step.Callable == id {
			return true
		}
	}
	return false
}

var ps6081TransferNameReplacer = strings.NewReplacer("_", "", "-", "")

func ps6081TransferName(name string) bool {
	normalized := strings.ToLower(ps6081TransferNameReplacer.Replace(name))
	for _, part := range []string{"transfer", "upload", "download", "todevice", "tohost", "copydevice", "copyhost", "backendcopy"} {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
}

func ps6081Validated(function *ast.FuncDecl) bool {
	if function.Doc == nil {
		return false
	}
	const marker = "//perfscan:shared-fanout-validated"
	for _, comment := range function.Doc.List {
		if comment.Text == marker {
			return true
		}
		if strings.HasPrefix(comment.Text, marker) {
			rest := strings.TrimPrefix(comment.Text, marker)
			if rest != "" && (rest[0] == ' ' || rest[0] == '\t') {
				return true
			}
		}
	}
	return false
}

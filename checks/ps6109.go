package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6109 implements owner issue #892 through an exact project lifecycle
// contract. Native handle semantics are never inferred from API names.
var PS6109 = register(&lint.Check{
	ID:          "PS6109",
	Category:    "alloc",
	Slug:        "repeated-fresh-wrapper-around-one-shot-native-handle",
	Level:       lint.LevelAggressive,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"reusableOneShotWrapperContracts"},
	Doc: lint.Documentation{
		Title: "a fixed repeated path creates fresh Go shells around one-shot native handles",
		Text: `A fixed repeated path may allocate a fresh Go wrapper for each native
generation even when the wrapper is terminally owned and could be retained as
one or two owner-local shells. This does not make the native handle reusable:
each reset must install an always-fresh one-shot handle.

PS6109 is deliberately contract-gated. It requires exact typed wrapper,
provider, acquisition, constructor, terminal, reset, native-factory, field,
and synchronous-use identities plus explicit promises for failure state,
terminal clearing/idempotence, fresh generations, stale-reference exclusion,
non-retention, synchronization, concurrency, fallback, and panic behavior.
Names such as New, Reset, Recorder, and Free establish none of those facts.

The initial source proof accepts only an exact positive fixed loop, at most
three package-local straight-line forwarding edges, a monomorphic provider
whose allocation dominates the loop, and one or two fully classified wrapper
generations. The loop body, induction object, and every forwarded repetition
root are closed so a branch, early exit, or index mutation cannot overstate the
static count. A fallible one-generation acquisition must use the canonical
nil-error guard. Two overlapping generations require an infallible two-slot
contract. A proved concrete acquisition may cross one direct package boundary
through an exact typed object fact. That fact carries normalized producer-local
concrete identities and body proof; the consumer independently validates its
static interface against the exact dynamic provider. Missing or mismatched
facts and imported intermediate bodies remain silent. Calls through unknown interfaces,
async/deferred work, wrapper aliases or aggregates, post-terminal uses,
unclassified exits, and unstable provider identity suppress the finding.

The initial owner-field form is assignment-only: one fresh unexported owner is
returned bare from its local constructor after one exact provider-field
assignment, then calls the candidate pointer method directly on that constructor
result. Every runtime owner leaf and provider-field use is closed; keyed field
initializers, stored owners, and calls through another same-typed owner remain
outside the proof. The function-local form likewise closes both the interface
binding and its concrete fresh provider initializer.

There is NO automatic fix. Retain only the Go shell; reset it with a fresh
one-shot native handle. Preserve the allocating fallback on reset failure and
audit stale references, generation identity, terminal order, error/panic
behavior, races, outputs, and application crossover. A wrapper-allocation
reduction is not an unconditional wall-time win.`,
		Before: `for step := 0; step < 12; step++ {
	recorder := provider.NewRecorder()
	recorder.Encode()
	recorder.Free()
}`,
		After: `recorder := provider.NewRecorder()
	defer recorder.Free()
	for step := 0; step < 12; step++ {
		if step != 0 {
			recorder.Reset() // installs a fresh one-shot native handle
		}
		recorder.Encode()
		recorder.Free()
	}`,
		MeasuredWin: `An isolated Go-shell microbenchmark ran six alternating
fresh-process pairs for two seconds per arm. The source-applicable retained-work
mechanism reduced the median from 11.570 ns/op to 5.4055 ns/op; the median
paired delta was -53.306% (2.1416x), and all six pairs favored the retained
shell. It also reduced 24 B/op and 1 alloc/op to 0 B/op and 0 alloc/op while
preserving the benchmark's generation, terminal-state, idempotence, and digest
checks. The timed binary itself was not disassembled; its unchanged benchmark
functions and code shape inherited earlier native inspection. After the
campaign, the benchmark file changed only in the untimed worker test: a comment
now records why testing.AllocsPerRun requires that worker to remain serial.

This microbenchmark does not execute or measure native handle creation,
configured factory/interface dispatch, a decoder, or application behavior.
Owner issue #892 attributed the production Go-wrapper change as 1 to 0
allocations/op. Earlier 7 to 0 evidence included unrelated Into and staging
work, and its 1.029x, 0.996x, 1.000x, and 1.027x timings remain historical
production boundaries, not isolated wrapper-only speedups. Revalidate the
exact backend and workload.`,
	},
	Analyzer: &analysis.Analyzer{
		Name:      "PS6109",
		Doc:       "fixed repeated fresh Go wrapper around an always-fresh one-shot native handle",
		Run:       runPS6109,
		FactTypes: []analysis.Fact{new(ps6109FactoryFact)},
	},
})

type ps6109FactoryFact struct {
	Proofs []string
}

func (*ps6109FactoryFact) AFact() {}
func (fact *ps6109FactoryFact) String() string {
	return "proved reusable one-shot wrapper factory (" + strconv.Itoa(len(fact.Proofs)) + " compatible contract(s))"
}

type ps6109Generation struct {
	assignment *ast.AssignStmt
	call       *ast.CallExpr
	wrapper    *types.Var
	terminal   *ast.CallExpr
	useCount   int
}

type ps6109Context struct {
	pass      *analysis.Pass
	locals    map[*types.Func]*ast.FuncDecl
	byID      map[string]*types.Func
	packages  map[string]*types.Package
	contracts []config.ReusableOneShotWrapperContract
	reported  map[*ast.CallExpr]bool
}

func runPS6109(pass *analysis.Pass) (any, error) {
	return runPS6109WithContracts(pass, config.Current().ReusableOneShotWrapperContracts)
}

func runPS6109WithContracts(pass *analysis.Pass, configured []config.ReusableOneShotWrapperContract) (any, error) {
	if len(configured) == 0 {
		return nil, nil
	}
	context := &ps6109Context{
		pass:     pass,
		locals:   make(map[*types.Func]*ast.FuncDecl),
		byID:     make(map[string]*types.Func),
		packages: make(map[string]*types.Package),
		reported: make(map[*ast.CallExpr]bool),
	}
	context.packages[pass.Pkg.Path()] = pass.Pkg
	for _, imported := range pass.Pkg.Imports() {
		context.packages[imported.Path()] = imported
	}
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
			object = object.Origin()
			context.locals[object] = function
			context.byID[ps6090FunctionID(object)] = object
		}
	}
	context.exportFactoryFacts(configured)
	for index := range configured {
		if configured[index].Valid() && !ps6109DuplicateName(configured, index) &&
			!ps6109DuplicateAcquisition(configured, index) && context.contractTyped(&configured[index]) {
			context.contracts = append(context.contracts, configured[index])
		}
	}
	if len(context.contracts) == 0 {
		return nil, nil
	}
	for _, function := range context.locals {
		context.scanFunction(function)
	}
	return nil, nil
}

func (context *ps6109Context) exportFactoryFacts(configured []config.ReusableOneShotWrapperContract) {
	proofs := make(map[*types.Func]map[string]bool)
	for index := range configured {
		contract := &configured[index]
		if !contract.Valid() || !context.producerContractTyped(contract) {
			continue
		}
		function := context.lookupMethod(contract.ConcreteAcquisition)
		if function == nil || function.Pkg() != context.pass.Pkg || !context.factoryProvenLocal(function, contract) {
			continue
		}
		if proofs[function.Origin()] == nil {
			proofs[function.Origin()] = make(map[string]bool)
		}
		proofs[function.Origin()][ps6109ProducerFingerprint(contract)] = true
	}
	for function, entries := range proofs {
		fact := &ps6109FactoryFact{Proofs: make([]string, 0, len(entries))}
		for proof := range entries {
			fact.Proofs = append(fact.Proofs, proof)
		}
		slices.Sort(fact.Proofs)
		context.pass.ExportObjectFact(function, fact)
	}
}

func (context *ps6109Context) producerContractTyped(contract *config.ReusableOneShotWrapperContract) bool {
	wrapper := context.named(contract.WrapperType)
	provider := context.named(contract.ProviderType)
	if wrapper == nil || provider == nil || wrapper.Obj().Pkg() != context.pass.Pkg || provider.Obj().Pkg() != context.pass.Pkg {
		return false
	}
	structure, structureOK := wrapper.Underlying().(*types.Struct)
	if !structureOK {
		return false
	}
	if _, dynamic := provider.Underlying().(*types.Interface); dynamic {
		return false
	}
	acquisition := context.lookupMethod(contract.ConcreteAcquisition)
	constructor := context.lookupFunction(contract.WrapperConstructor)
	if acquisition == nil || acquisition.Pkg() != context.pass.Pkg || constructor == nil || constructor.Pkg() != context.pass.Pkg ||
		!ps6109ConcreteReceiver(acquisition, provider) ||
		!ps6109WrapperResultRole(acquisition, contract.AcquisitionWrapperResult, wrapper) ||
		!ps6109StatusRole(acquisition, contract.AcquisitionStatusResult, contract.AcquisitionFailureMode) ||
		!ps6109ResultRole(constructor, contract.ConstructorWrapperResult, types.NewPointer(wrapper)) {
		return false
	}
	if !context.concreteMethodTyped(contract.Terminal.Concrete, wrapper) || !context.concreteMethodTyped(contract.Reset.Concrete, wrapper) ||
		!ps6109StatusRole(context.lookupMethod(contract.Reset.Concrete), contract.ResetStatusResult, ps6109ResetAsAcquisitionMode(contract.ResetFailureState)) {
		return false
	}
	for _, method := range contract.AllowedSynchronousUses {
		if !context.concreteMethodTyped(method.Concrete, wrapper) {
			return false
		}
	}
	handle := ps6109ConfiguredField(structure, contract.NativeHandleField, wrapper)
	if handle == nil {
		return false
	}
	for _, fieldID := range contract.MutableStateFields {
		if ps6109ConfiguredField(structure, fieldID, wrapper) == nil {
			return false
		}
	}
	factory := context.lookupCallable(contract.FreshNativeHandleFactory)
	if factory == nil || factory.Pkg() != context.pass.Pkg {
		return false
	}
	signature, _ := factory.Type().(*types.Signature)
	return signature != nil && !signature.Variadic() && signature.TypeParams().Len() == 0 && signature.RecvTypeParams().Len() == 0 &&
		signature.Results().Len() == 1 && types.AssignableTo(signature.Results().At(0).Type(), handle.Type())
}

func ps6109ConcreteReceiver(function *types.Func, owner *types.Named) bool {
	signature, _ := function.Type().(*types.Signature)
	return signature != nil && signature.Recv() != nil && !signature.Variadic() && signature.TypeParams().Len() == 0 &&
		signature.RecvTypeParams().Len() == 0 && ps6109TypeID(signature.Recv().Type()) == ps6109TypeID(owner)
}

func (context *ps6109Context) concreteMethodTyped(id string, wrapper *types.Named) bool {
	method := context.lookupMethod(id)
	return method != nil && method.Pkg() == context.pass.Pkg && ps6109ConcreteReceiver(method, wrapper)
}

func (context *ps6109Context) contractTyped(contract *config.ReusableOneShotWrapperContract) bool {
	wrapper := context.named(contract.WrapperType)
	provider := context.named(contract.ProviderType)
	if wrapper == nil || provider == nil {
		return false
	}
	if _, ok := wrapper.Underlying().(*types.Struct); !ok {
		return false
	}
	if _, dynamic := provider.Underlying().(*types.Interface); dynamic {
		return false
	}
	acquisition := context.lookupMethod(contract.Acquisition)
	concreteAcquisition := context.lookupMethod(contract.ConcreteAcquisition)
	constructor := context.lookupFunction(contract.WrapperConstructor)
	if acquisition == nil || concreteAcquisition == nil || constructor == nil ||
		!context.acquisitionPairTyped(acquisition, concreteAcquisition, provider) ||
		!ps6109WrapperResultRole(concreteAcquisition, contract.AcquisitionWrapperResult, wrapper) ||
		!ps6109StatusRole(concreteAcquisition, contract.AcquisitionStatusResult, contract.AcquisitionFailureMode) ||
		!ps6109ResultRole(constructor, contract.ConstructorWrapperResult, types.NewPointer(wrapper)) {
		return false
	}
	if !context.methodPairTyped(contract.Terminal, wrapper) || !context.methodPairTyped(contract.Reset, wrapper) ||
		!ps6109StatusRole(context.lookupMethod(contract.Reset.Concrete), contract.ResetStatusResult, ps6109ResetAsAcquisitionMode(contract.ResetFailureState)) {
		return false
	}
	for _, method := range contract.AllowedSynchronousUses {
		if !context.methodPairTyped(method, wrapper) {
			return false
		}
	}
	structure := wrapper.Underlying().(*types.Struct)
	handle := ps6109ConfiguredField(structure, contract.NativeHandleField, wrapper)
	if handle == nil {
		return false
	}
	for _, fieldID := range contract.MutableStateFields {
		if ps6109ConfiguredField(structure, fieldID, wrapper) == nil {
			return false
		}
	}
	factory := context.lookupCallable(contract.FreshNativeHandleFactory)
	if factory == nil {
		return false
	}
	factorySignature, _ := factory.Type().(*types.Signature)
	return factorySignature != nil && factorySignature.Results().Len() == 1 &&
		types.AssignableTo(factorySignature.Results().At(0).Type(), handle.Type())
}

func (context *ps6109Context) named(id string) *types.Named {
	separator := strings.LastIndexByte(id, '.')
	if separator <= 0 {
		return nil
	}
	pkg := context.packages[id[:separator]]
	if pkg == nil {
		return nil
	}
	object, _ := pkg.Scope().Lookup(id[separator+1:]).(*types.TypeName)
	if object == nil {
		return nil
	}
	named, _ := types.Unalias(object.Type()).(*types.Named)
	if named == nil || named.TypeParams().Len() != 0 {
		return nil
	}
	return named
}

func (context *ps6109Context) lookupCallable(id string) *types.Func {
	if function := context.byID[id]; function != nil {
		return function
	}
	if function := context.lookupFunction(id); function != nil {
		return function
	}
	return context.lookupMethod(id)
}

func (context *ps6109Context) lookupFunction(id string) *types.Func {
	separator := strings.LastIndexByte(id, '.')
	if separator <= 0 {
		return nil
	}
	pkg := context.packages[id[:separator]]
	if pkg == nil {
		return nil
	}
	function, _ := pkg.Scope().Lookup(id[separator+1:]).(*types.Func)
	return function
}

func (context *ps6109Context) lookupMethod(id string) *types.Func {
	methodSeparator := strings.LastIndexByte(id, '.')
	if methodSeparator <= 0 {
		return nil
	}
	receiverSeparator := strings.LastIndexByte(id[:methodSeparator], '.')
	if receiverSeparator <= 0 {
		return nil
	}
	pkg := context.packages[id[:receiverSeparator]]
	if pkg == nil {
		return nil
	}
	typeName := id[receiverSeparator+1 : methodSeparator]
	named := context.named(id[:receiverSeparator] + "." + typeName)
	if named == nil {
		return nil
	}
	object, _, _ := types.LookupFieldOrMethod(named, true, pkg, id[methodSeparator+1:])
	if object == nil {
		object, _, _ = types.LookupFieldOrMethod(types.NewPointer(named), true, pkg, id[methodSeparator+1:])
	}
	function, _ := object.(*types.Func)
	return function
}

func (context *ps6109Context) methodPairTyped(method config.ReusableOneShotMethod, wrapper *types.Named) bool {
	static := context.lookupMethod(method.Static)
	concrete := context.lookupMethod(method.Concrete)
	if static == nil || concrete == nil || ps6109TypeID(concrete.Type().(*types.Signature).Recv().Type()) != ps6109TypeID(wrapper) ||
		!ps6109SignaturesCompatible(static, concrete) {
		return false
	}
	selected, _, _ := types.LookupFieldOrMethod(types.NewPointer(wrapper), true, wrapper.Obj().Pkg(), static.Name())
	selectedFunction, _ := selected.(*types.Func)
	return selectedFunction != nil && selectedFunction.Origin() == concrete.Origin()
}

func (context *ps6109Context) acquisitionPairTyped(static, concrete *types.Func, provider *types.Named) bool {
	if static == nil || concrete == nil || provider == nil || !ps6109SignaturesCompatible(static, concrete) ||
		ps6109TypeID(concrete.Type().(*types.Signature).Recv().Type()) != ps6109TypeID(provider) {
		return false
	}
	selected, _, _ := types.LookupFieldOrMethod(types.NewPointer(provider), true, provider.Obj().Pkg(), static.Name())
	selectedFunction, _ := selected.(*types.Func)
	return selectedFunction != nil && selectedFunction.Origin() == concrete.Origin()
}

func ps6109SignaturesCompatible(left, right *types.Func) bool {
	leftSignature, leftOK := left.Type().(*types.Signature)
	rightSignature, rightOK := right.Type().(*types.Signature)
	return leftOK && rightOK && !leftSignature.Variadic() && !rightSignature.Variadic() &&
		leftSignature.TypeParams().Len() == 0 && rightSignature.TypeParams().Len() == 0 &&
		types.Identical(leftSignature.Params(), rightSignature.Params()) && types.Identical(leftSignature.Results(), rightSignature.Results())
}

func ps6109ResultRole(function *types.Func, position int, expected types.Type) bool {
	if function == nil || position <= 0 {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	return signature != nil && position <= signature.Results().Len() && types.Identical(signature.Results().At(position-1).Type(), expected)
}

func ps6109WrapperResultRole(function *types.Func, position int, wrapper *types.Named) bool {
	if function == nil || position <= 0 || wrapper == nil {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || position > signature.Results().Len() {
		return false
	}
	result := types.Unalias(signature.Results().At(position - 1).Type())
	pointer := types.NewPointer(wrapper)
	if types.Identical(result, pointer) {
		return true
	}
	interfaceType, ok := result.Underlying().(*types.Interface)
	return ok && types.Implements(pointer, interfaceType)
}

func ps6109StatusRole(function *types.Func, position int, mode string) bool {
	if function == nil {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil {
		return false
	}
	if mode == config.ReusableOneShotAcquisitionInfallible {
		return position == 0
	}
	if mode != config.ReusableOneShotAcquisitionNilError || position <= 0 || position > signature.Results().Len() {
		return false
	}
	errorType := types.Universe.Lookup("error").Type()
	return types.Identical(signature.Results().At(position-1).Type(), errorType)
}

func ps6109ResetAsAcquisitionMode(mode string) string {
	if mode == config.ReusableOneShotResetInfallibleEmpty {
		return config.ReusableOneShotAcquisitionInfallible
	}
	return config.ReusableOneShotAcquisitionNilError
}

func ps6109ConfiguredField(structure *types.Struct, id string, owner *types.Named) *types.Var {
	prefix := owner.Obj().Pkg().Path() + "." + owner.Obj().Name() + "."
	if !strings.HasPrefix(id, prefix) {
		return nil
	}
	name := strings.TrimPrefix(id, prefix)
	for index := range structure.NumFields() {
		field := structure.Field(index)
		if field.Name() == name && field.Pkg() == owner.Obj().Pkg() {
			return field
		}
	}
	return nil
}

func ps6109DuplicateName(contracts []config.ReusableOneShotWrapperContract, index int) bool {
	for other := range contracts {
		if other != index && contracts[other].Name == contracts[index].Name {
			return true
		}
	}
	return false
}

func ps6109DuplicateAcquisition(contracts []config.ReusableOneShotWrapperContract, index int) bool {
	for other := range contracts {
		if other != index && contracts[other].Acquisition == contracts[index].Acquisition {
			return true
		}
	}
	return false
}

func (context *ps6109Context) scanFunction(function *ast.FuncDecl) {
	unreachable := ps2144Unreachable(context.pass, function.Body)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		loop := ps6103FixedLoop(context.pass, node)
		if loop == nil || ps2144PositionIn(loop.node.Pos(), unreachable) {
			return true
		}
		for index := range context.contracts {
			contract := &context.contracts[index]
			context.matchDirectLoop(function, loop, contract)
			for _, statement := range loop.body.List {
				call, ok := ps6109RepeatedCall(context.pass, statement)
				if ok {
					context.matchForwardedLoop(function, loop, call, contract)
				}
			}
		}
		return true
	})
}

func (context *ps6109Context) matchDirectLoop(function *ast.FuncDecl, loop *ps6103Loop, contract *config.ReusableOneShotWrapperContract) {
	generations := context.lifecycleGenerations(loop.body, contract, true)
	if len(generations) == 0 || !ps6109LoopIndexClosed(context.pass, loop) || !context.sameProvider(generations) ||
		!context.providerForDirectLoop(function, loop, generations[0].call, contract) ||
		!context.factoryProven(contract) {
		return
	}
	context.report(loop.count, generations, contract, nil)
}

func (context *ps6109Context) matchForwardedLoop(function *ast.FuncDecl, loop *ps6103Loop, root *ast.CallExpr, contract *config.ReusableOneShotWrapperContract) {
	if !ps6109LoopIndexClosed(context.pass, loop) || !ps6109RepeatedRootPreservesStatus(loop.body, root, contract) ||
		!ps6109ForwardedRootOnly(context.pass, loop.body, root) {
		return
	}
	chain, bindings, lifecycle, ok := context.forwardingChain(root, contract)
	if !ok || len(chain) == 0 || lifecycle == nil {
		return
	}
	generations := context.lifecycleGenerations(lifecycle.Body, contract, false)
	if len(generations) == 0 || !context.sameProvider(generations) ||
		!context.providerForChain(function, loop, chain, bindings, generations[0].call, contract) ||
		!context.factoryProven(contract) {
		return
	}
	context.report(loop.count, generations, contract, root)
}

func ps6109LoopIndexClosed(pass *analysis.Pass, loop *ps6103Loop) bool {
	closed := true
	ast.Inspect(loop.body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.ObjectOf(identifier) == loop.index {
			closed = false
			return false
		}
		return closed
	})
	return closed
}

func ps6109ForwardedRootOnly(pass *analysis.Pass, body *ast.BlockStmt, root *ast.CallExpr) bool {
	if body == nil || len(body.List) != 1 {
		return false
	}
	call, ok := ps6109RepeatedCall(pass, body.List[0])
	return ok && call == root
}

func ps6109RepeatedRootPreservesStatus(body *ast.BlockStmt, root *ast.CallExpr, contract *config.ReusableOneShotWrapperContract) bool {
	for _, statement := range body.List {
		call, ok := ps6109RepeatedCall(nil, statement)
		if !ok || call != root {
			continue
		}
		_, guarded := statement.(*ast.IfStmt)
		return guarded == (contract.AcquisitionFailureMode == config.ReusableOneShotAcquisitionNilError)
	}
	return false
}

func (context *ps6109Context) sameProvider(generations []ps6109Generation) bool {
	if len(generations) == 0 {
		return false
	}
	root, field := context.providerRoot(generations[0].call)
	if root == nil {
		return false
	}
	for _, generation := range generations[1:] {
		otherRoot, otherField := context.providerRoot(generation.call)
		if otherRoot != root || otherField != field {
			return false
		}
	}
	return true
}

func (context *ps6109Context) providerRoot(call *ast.CallExpr) (types.Object, types.Object) {
	receiver := ps6109CallReceiver(call)
	switch value := ps2110Unparen(receiver).(type) {
	case *ast.Ident:
		return context.pass.TypesInfo.ObjectOf(value), nil
	case *ast.SelectorExpr:
		root, ok := ps2110Unparen(value.X).(*ast.Ident)
		selection := context.pass.TypesInfo.Selections[value]
		if !ok || selection == nil {
			return nil, nil
		}
		return context.pass.TypesInfo.ObjectOf(root), selection.Obj()
	default:
		return nil, nil
	}
}

func (context *ps6109Context) report(repetitions int64, generations []ps6109Generation, contract *config.ReusableOneShotWrapperContract, forwardedRoot *ast.CallExpr) {
	live := len(generations)
	reports := make([]*ast.CallExpr, 0, len(generations))
	if forwardedRoot != nil {
		reports = append(reports, forwardedRoot)
	} else {
		for _, generation := range generations {
			reports = append(reports, generation.call)
		}
	}
	for _, call := range reports {
		if context.reported[call] {
			continue
		}
		context.reported[call] = true
		context.pass.Report(analysis.Diagnostic{
			Pos: call.Pos(), End: call.End(),
			Message: "fixed " + strconv.FormatInt(repetitions, 10) + "-step source path creates a fresh Go wrapper generation through " +
				contract.Acquisition + " via fresh-shell constructor " + contract.WrapperConstructor + " and terminates it with " +
				contract.Terminal.Static + "; the enclosing runtime invocation count is unknown; source proves at most " +
				strconv.Itoa(live) + " live generation(s): retain only that many Go shells and reset each with an always-fresh one-shot native handle, preserving the allocating fallback on reset failure; audit stale references, generation identity, terminal/error/panic order, races, exact outputs, and order-alternated application crossover (contract-gated advisory, no automatic fix)",
		})
	}
}

func ps6109RepeatedCall(pass *analysis.Pass, statement ast.Stmt) (*ast.CallExpr, bool) {
	switch value := statement.(type) {
	case *ast.ExprStmt:
		call, ok := ps2110Unparen(value.X).(*ast.CallExpr)
		return call, ok && !call.Ellipsis.IsValid()
	case *ast.IfStmt:
		if pass == nil {
			assignment, ok := value.Init.(*ast.AssignStmt)
			if !ok || len(assignment.Rhs) != 1 {
				return nil, false
			}
			call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
			return call, ok
		}
		assignment, ok := value.Init.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || value.Else != nil {
			return nil, false
		}
		status, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		call, called := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
		condition, compared := ps2110Unparen(value.Cond).(*ast.BinaryExpr)
		if !ok || !called || !compared || condition.Op != token.NEQ || !ps6103Object(pass, condition.X, pass.TypesInfo.Defs[status]) ||
			!ps6109Nil(pass, condition.Y) || value.Body == nil || len(value.Body.List) != 1 {
			return nil, false
		}
		returned, ok := value.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(returned.Results) != 1 || !ps6103Object(pass, returned.Results[0], pass.TypesInfo.Defs[status]) {
			return nil, false
		}
		return call, !call.Ellipsis.IsValid()
	}
	return nil, false
}

func ps6109Nil(pass *analysis.Pass, expression ast.Expr) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	object, okObject := pass.TypesInfo.Uses[identifier].(*types.Nil)
	return ok && okObject && object != nil
}

func (context *ps6109Context) forwardingChain(root *ast.CallExpr, contract *config.ReusableOneShotWrapperContract) ([]*ast.CallExpr, map[types.Object]ast.Expr, *ast.FuncDecl, bool) {
	bindings := make(map[types.Object]ast.Expr)
	chain := []*ast.CallExpr{root}
	call := root
	for depth := 0; depth < 4; depth++ {
		function, signature, ok := typedCallee(context.pass, call.Fun)
		if !ok || function.Pkg() != context.pass.Pkg || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 ||
			!ps6099ConcreteLeafCallee(context.pass, call, function, signature) {
			return nil, nil, nil, false
		}
		declaration := context.locals[function.Origin()]
		if declaration == nil || !ps6109BindParameters(context.pass, call, signature, bindings) {
			return nil, nil, nil, false
		}
		if context.hasAcquisition(declaration.Body, contract) {
			return chain, bindings, declaration, true
		}
		next, ok := ps6109OnlyForwardCall(declaration.Body)
		if !ok {
			return nil, nil, nil, false
		}
		chain = append(chain, next)
		call = next
	}
	return nil, nil, nil, false
}

func ps6109BindParameters(pass *analysis.Pass, call *ast.CallExpr, signature *types.Signature, bindings map[types.Object]ast.Expr) bool {
	offset, ok := ps6099CallSignatureOffset(pass, call, signature)
	if !ok || len(call.Args)-offset != signature.Params().Len() {
		return false
	}
	for index := range signature.Params().Len() {
		bindings[signature.Params().At(index)] = call.Args[index+offset]
	}
	return true
}

func ps6109OnlyForwardCall(body *ast.BlockStmt) (*ast.CallExpr, bool) {
	if body == nil || len(body.List) != 1 {
		return nil, false
	}
	switch statement := body.List[0].(type) {
	case *ast.ExprStmt:
		call, ok := ps2110Unparen(statement.X).(*ast.CallExpr)
		return call, ok && !call.Ellipsis.IsValid()
	case *ast.ReturnStmt:
		if len(statement.Results) != 1 {
			return nil, false
		}
		call, ok := ps2110Unparen(statement.Results[0]).(*ast.CallExpr)
		return call, ok && !call.Ellipsis.IsValid()
	}
	return nil, false
}

func (context *ps6109Context) hasAcquisition(body *ast.BlockStmt, contract *config.ReusableOneShotWrapperContract) bool {
	for _, statement := range body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || len(assignment.Rhs) != 1 {
			continue
		}
		call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
		if ok && ps6087FunctionID(context.pass, call) == contract.Acquisition {
			return true
		}
	}
	return false
}

func (context *ps6109Context) lifecycleGenerations(body *ast.BlockStmt, contract *config.ReusableOneShotWrapperContract, directLoop bool) []ps6109Generation {
	if body == nil {
		return nil
	}
	parents := ps6087Parents(body)
	var generations []ps6109Generation
	for index, statement := range body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Rhs) != 1 {
			continue
		}
		call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
		if !ok || call.Ellipsis.IsValid() || ps6087FunctionID(context.pass, call) != contract.Acquisition {
			continue
		}
		if len(generations) == 0 && index != 0 {
			return nil
		}
		function, signature, typed := typedCallee(context.pass, call.Fun)
		wrapperIndex := contract.AcquisitionWrapperResult - 1
		if !typed || signature.Results().Len() != len(assignment.Lhs) || wrapperIndex < 0 || wrapperIndex >= len(assignment.Lhs) ||
			function.Origin() == nil {
			return nil
		}
		identifier, ok := ps2110Unparen(assignment.Lhs[wrapperIndex]).(*ast.Ident)
		wrapper, okObject := context.pass.TypesInfo.Defs[identifier].(*types.Var)
		if !ok || !okObject || wrapper == nil || !context.acquisitionGuard(body, index, assignment, contract, directLoop) {
			return nil
		}
		generation, ok := context.wrapperClosed(body, parents, assignment, call, wrapper, contract)
		if !ok {
			return nil
		}
		generations = append(generations, generation)
	}
	if len(generations) == 0 || len(generations) > contract.SlotBound {
		return nil
	}
	if len(generations) == 2 {
		if contract.SlotBound != 2 || contract.AcquisitionFailureMode != config.ReusableOneShotAcquisitionInfallible ||
			generations[0].terminal.Pos() < generations[1].assignment.End() {
			return nil
		}
	}
	if !context.lifecycleBodyClosed(body, generations, contract, directLoop) {
		return nil
	}
	return generations
}

func (context *ps6109Context) lifecycleBodyClosed(body *ast.BlockStmt, generations []ps6109Generation, contract *config.ReusableOneShotWrapperContract, directLoop bool) bool {
	assignments := make(map[*ast.AssignStmt]bool, len(generations))
	wrappers := make(map[types.Object]bool, len(generations))
	for _, generation := range generations {
		assignments[generation.assignment] = true
		wrappers[generation.wrapper] = true
	}
	for index, statement := range body.List {
		switch value := statement.(type) {
		case *ast.AssignStmt:
			if !assignments[value] {
				return false
			}
		case *ast.IfStmt:
			if index == 0 || contract.AcquisitionFailureMode != config.ReusableOneShotAcquisitionNilError {
				return false
			}
			assignment, ok := body.List[index-1].(*ast.AssignStmt)
			if !ok || !assignments[assignment] || !context.acquisitionGuard(body, index-1, assignment, contract, directLoop) {
				return false
			}
		case *ast.ExprStmt:
			call, ok := ps2110Unparen(value.X).(*ast.CallExpr)
			if !ok {
				return false
			}
			selector := ps6099CallSelector(call.Fun)
			if selector == nil {
				return false
			}
			receiver, receiverOK := ps2110Unparen(selector.X).(*ast.Ident)
			if !receiverOK || !wrappers[context.pass.TypesInfo.ObjectOf(receiver)] {
				return false
			}
			id := ps6087FunctionID(context.pass, call)
			if id != contract.Terminal.Static && !ps6109AllowedMethod(id, contract.AllowedSynchronousUses) {
				return false
			}
		case *ast.ReturnStmt:
			if directLoop || index != len(body.List)-1 || len(value.Results) == 0 {
				return false
			}
			for _, result := range value.Results {
				if !ps6109Nil(context.pass, result) {
					return false
				}
			}
		default:
			return false
		}
	}
	return true
}

func (context *ps6109Context) acquisitionGuard(body *ast.BlockStmt, index int, assignment *ast.AssignStmt, contract *config.ReusableOneShotWrapperContract, directLoop bool) bool {
	if contract.AcquisitionFailureMode == config.ReusableOneShotAcquisitionInfallible {
		return contract.AcquisitionStatusResult == 0
	}
	statusIndex := contract.AcquisitionStatusResult - 1
	if statusIndex < 0 || statusIndex >= len(assignment.Lhs) || index+1 >= len(body.List) {
		return false
	}
	identifier, ok := ps2110Unparen(assignment.Lhs[statusIndex]).(*ast.Ident)
	if !ok {
		return false
	}
	object := context.pass.TypesInfo.Defs[identifier]
	guard, ok := body.List[index+1].(*ast.IfStmt)
	if !ok || guard == nil {
		return false
	}
	condition, okCondition := ps2110Unparen(guard.Cond).(*ast.BinaryExpr)
	if object == nil || guard.Init != nil || guard.Else != nil || !okCondition || condition.Op != token.NEQ ||
		!ps6103Object(context.pass, condition.X, object) || !ps6109Nil(context.pass, condition.Y) || len(guard.Body.List) != 1 {
		return false
	}
	if directLoop {
		branch, ok := guard.Body.List[0].(*ast.BranchStmt)
		return ok && branch.Tok == token.CONTINUE && branch.Label == nil
	}
	returned, ok := guard.Body.List[0].(*ast.ReturnStmt)
	return ok && len(returned.Results) == 1 && ps6103Object(context.pass, returned.Results[0], object)
}

func (context *ps6109Context) wrapperClosed(body *ast.BlockStmt, parents map[ast.Node]ast.Node, assignment *ast.AssignStmt, acquisition *ast.CallExpr, wrapper *types.Var, contract *config.ReusableOneShotWrapperContract) (ps6109Generation, bool) {
	generation := ps6109Generation{assignment: assignment, call: acquisition, wrapper: wrapper}
	valid := true
	var lastUse token.Pos
	ast.Inspect(body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || context.pass.TypesInfo.ObjectOf(identifier) != wrapper {
			return true
		}
		if context.pass.TypesInfo.Defs[identifier] == wrapper {
			return true
		}
		selector, ok := parents[identifier].(*ast.SelectorExpr)
		call, okCall := parents[selector].(*ast.CallExpr)
		statement, direct := parents[call].(*ast.ExprStmt)
		if !ok || selector.X != identifier || !okCall || call.Fun != selector || !direct || statement == nil ||
			parents[statement] != body || call.Ellipsis.IsValid() {
			valid = false
			return false
		}
		id := ps6087FunctionID(context.pass, call)
		if call.Pos() > lastUse {
			lastUse = call.Pos()
		}
		switch {
		case id == contract.Terminal.Static:
			if generation.terminal != nil {
				valid = false
				return false
			}
			generation.terminal = call
		case ps6109AllowedMethod(id, contract.AllowedSynchronousUses):
			generation.useCount++
		default:
			valid = false
			return false
		}
		return true
	})
	return generation, valid && generation.useCount > 0 && generation.terminal != nil &&
		generation.terminal.Pos() > acquisition.End() && generation.terminal.Pos() == lastUse &&
		context.straightLineUntilTerminal(body, assignment, generation.terminal, contract)
}

func (context *ps6109Context) straightLineUntilTerminal(body *ast.BlockStmt, acquisition *ast.AssignStmt, terminal *ast.CallExpr, contract *config.ReusableOneShotWrapperContract) bool {
	for index, statement := range body.List {
		if statement.Pos() < acquisition.Pos() || statement.Pos() > terminal.Pos() {
			continue
		}
		switch value := statement.(type) {
		case *ast.AssignStmt:
			if value == acquisition {
				continue
			}
			if len(value.Rhs) == 1 {
				if call, ok := ps2110Unparen(value.Rhs[0]).(*ast.CallExpr); ok && ps6087FunctionID(context.pass, call) == contract.Acquisition {
					continue
				}
			}
			return false
		case *ast.IfStmt:
			// The detailed acquisitionGuard proof validates the sole supported
			// nil-error branch. Other control flow is outside the initial grammar.
			if index == 0 || value.Init != nil || value.Else != nil || len(value.Body.List) != 1 {
				return false
			}
			previous, ok := body.List[index-1].(*ast.AssignStmt)
			if !ok || len(previous.Rhs) != 1 {
				return false
			}
			call, ok := ps2110Unparen(previous.Rhs[0]).(*ast.CallExpr)
			if !ok || ps6087FunctionID(context.pass, call) != contract.Acquisition {
				return false
			}
		case *ast.ExprStmt:
			call, ok := ps2110Unparen(value.X).(*ast.CallExpr)
			if !ok {
				return false
			}
			id := ps6087FunctionID(context.pass, call)
			if id != contract.Terminal.Static && !ps6109AllowedMethod(id, contract.AllowedSynchronousUses) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func ps6109AllowedMethod(id string, methods []config.ReusableOneShotMethod) bool {
	for _, method := range methods {
		if id == method.Static {
			return true
		}
	}
	return false
}

func (context *ps6109Context) providerForDirectLoop(function *ast.FuncDecl, loop *ps6103Loop, acquisition *ast.CallExpr, contract *config.ReusableOneShotWrapperContract) bool {
	receiver := ps6109CallReceiver(acquisition)
	return receiver != nil && context.providerBinding(function, loop.node.Pos(), receiver, nil, contract)
}

func (context *ps6109Context) providerForChain(function *ast.FuncDecl, loop *ps6103Loop, chain []*ast.CallExpr, bindings map[types.Object]ast.Expr, acquisition *ast.CallExpr, contract *config.ReusableOneShotWrapperContract) bool {
	receiver := ps6109CallReceiver(acquisition)
	if receiver == nil || !context.providerForwardingClosed(receiver, bindings, contract) {
		return false
	}
	resolved := ps6109ResolveBinding(context.pass, receiver, bindings)
	return resolved != nil && context.providerBinding(function, loop.node.Pos(), resolved, chain[0], contract)
}

func (context *ps6109Context) providerForwardingClosed(receiver ast.Expr, bindings map[types.Object]ast.Expr, contract *config.ReusableOneShotWrapperContract) bool {
	identifier, ok := ps2110Unparen(receiver).(*ast.Ident)
	if !ok {
		return false
	}
	var parameters []types.Object
	var allowedByPrevious []ast.Expr
	seen := make(map[types.Object]bool)
	for {
		object := context.pass.TypesInfo.ObjectOf(identifier)
		next := bindings[object]
		if object == nil || next == nil {
			break
		}
		if seen[object] {
			return false
		}
		seen[object] = true
		parameters = append(parameters, object)
		if len(parameters) > 1 {
			allowedByPrevious = append(allowedByPrevious, bindings[parameters[len(parameters)-2]])
		}
		identifier, ok = ps2110Unparen(next).(*ast.Ident)
		if !ok {
			break
		}
	}
	if len(parameters) == 0 {
		return false
	}
	uses := make([]int, len(parameters))
	valid := true
	for _, file := range context.pass.Files {
		parents := ps6087Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			id, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			object := context.pass.TypesInfo.Uses[id]
			for index, parameter := range parameters {
				if object != parameter {
					continue
				}
				uses[index]++
				if index == 0 {
					call := ps6109ReceiverCall(id, parents)
					if call != nil && ps6087FunctionID(context.pass, call) == contract.Acquisition {
						return true
					}
				} else if ps6109WithinExpression(id, allowedByPrevious[index-1], parents) {
					return true
				}
				valid = false
				return false
			}
			return true
		})
	}
	if !valid || uses[0] < 1 || uses[0] > contract.SlotBound {
		return false
	}
	for index := 1; index < len(uses); index++ {
		if uses[index] != 1 {
			return false
		}
	}
	return true
}

func ps6109ResolveBinding(pass *analysis.Pass, expression ast.Expr, bindings map[types.Object]ast.Expr) ast.Expr {
	seen := make(map[types.Object]bool)
	for {
		identifier, ok := ps2110Unparen(expression).(*ast.Ident)
		if !ok {
			return expression
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		next := bindings[object]
		if next == nil || seen[object] {
			return expression
		}
		seen[object] = true
		expression = next
	}
}

func ps6109CallReceiver(call *ast.CallExpr) ast.Expr {
	selector := ps6099CallSelector(call.Fun)
	if selector == nil {
		return nil
	}
	return selector.X
}

func (context *ps6109Context) providerBinding(function *ast.FuncDecl, before token.Pos, expression ast.Expr, allowedCall *ast.CallExpr, contract *config.ReusableOneShotWrapperContract) bool {
	if selector, ok := ps2110Unparen(expression).(*ast.SelectorExpr); ok {
		return context.ownerProviderBinding(function, selector, contract)
	}
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	provider, okObject := context.pass.TypesInfo.ObjectOf(identifier).(*types.Var)
	if !ok || !okObject || provider == nil {
		return false
	}
	variables, initializers, ok := ps6109ProviderInitializerChain(context.pass, function.Body, provider, before, contract.ProviderType)
	if !ok {
		return false
	}
	uses := make([]int, len(variables))
	closed := true
	parents := ps6087Parents(function.Body)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := context.pass.TypesInfo.Uses[id]
		for index, variable := range variables {
			if object != variable {
				continue
			}
			uses[index]++
			if index > 0 && ps6109WithinExpression(id, initializers[index-1], parents) {
				return true
			}
			if index == 0 && allowedCall != nil && ps6109ContainingArgument(id, parents, allowedCall) {
				return true
			}
			if index == 0 && allowedCall == nil {
				call := ps6109ReceiverCall(id, parents)
				if call != nil && ps6087FunctionID(context.pass, call) == contract.Acquisition {
					return true
				}
			}
			closed = false
			return false
		}
		return true
	})
	if !closed || uses[0] < 1 || uses[0] > contract.SlotBound {
		return false
	}
	for index := 1; index < len(uses); index++ {
		if uses[index] != 1 {
			return false
		}
	}
	return context.concreteAcquisition(contract)
}

func ps6109ProviderInitializerChain(pass *analysis.Pass, body *ast.BlockStmt, start *types.Var, before token.Pos, providerID string) ([]*types.Var, []ast.Expr, bool) {
	variables := []*types.Var{start}
	var initializers []ast.Expr
	seen := map[*types.Var]bool{start: true}
	for depth := 0; depth < 2; depth++ {
		initializer := ps6109ExactLocalInitializer(pass, body, variables[len(variables)-1], before)
		if initializer == nil {
			return nil, nil, false
		}
		initializers = append(initializers, initializer)
		if ps6109TypeID(pass.TypesInfo.TypeOf(initializer)) == providerID && ps6109FreshProvider(initializer) {
			return variables, initializers, true
		}
		identifier, ok := ps2110Unparen(initializer).(*ast.Ident)
		variable, variableOK := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
		if !ok || !variableOK || variable == nil || seen[variable] {
			return nil, nil, false
		}
		seen[variable] = true
		variables = append(variables, variable)
	}
	return nil, nil, false
}

func ps6109ExactLocalInitializer(pass *analysis.Pass, body *ast.BlockStmt, object *types.Var, before token.Pos) ast.Expr {
	var result ast.Expr
	for _, statement := range body.List {
		if statement.Pos() >= before {
			break
		}
		switch value := statement.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) != 1 || len(value.Rhs) != 1 || pass.TypesInfo.ObjectOf(ps6109Identifier(value.Lhs[0])) != object || result != nil {
				continue
			}
			result = value.Rhs[0]
		case *ast.DeclStmt:
			declaration, ok := value.Decl.(*ast.GenDecl)
			if !ok || len(declaration.Specs) != 1 {
				continue
			}
			specification, ok := declaration.Specs[0].(*ast.ValueSpec)
			if !ok || len(specification.Names) != 1 || len(specification.Values) != 1 || pass.TypesInfo.Defs[specification.Names[0]] != object || result != nil {
				continue
			}
			result = specification.Values[0]
		}
	}
	return result
}

func ps6109Identifier(expression ast.Expr) *ast.Ident {
	identifier, _ := ps2110Unparen(expression).(*ast.Ident)
	return identifier
}

func ps6109WithinExpression(identifier *ast.Ident, expression ast.Expr, parents map[ast.Node]ast.Node) bool {
	node := ast.Node(identifier)
	for node != nil && node.Pos() >= expression.Pos() && node.End() <= expression.End() {
		if node == expression {
			return true
		}
		node = parents[node]
	}
	return false
}

func (context *ps6109Context) ownerProviderBinding(function *ast.FuncDecl, selector *ast.SelectorExpr, contract *config.ReusableOneShotWrapperContract) bool {
	receiver, ok := ps2110Unparen(selector.X).(*ast.Ident)
	selection := context.pass.TypesInfo.Selections[selector]
	if !ok || selection == nil {
		return false
	}
	field, fieldOK := selection.Obj().(*types.Var)
	method, methodOK := context.pass.TypesInfo.Defs[function.Name].(*types.Func)
	methodSignature, signatureOK := method.Type().(*types.Signature)
	if !fieldOK || field.Exported() || !methodOK || !signatureOK || methodSignature.Recv() == nil {
		return false
	}
	receiverObject, ok := context.pass.TypesInfo.ObjectOf(receiver).(*types.Var)
	owner := ps6087Named(receiverObject.Type())
	if !ok || owner == nil || owner.Obj().Exported() || owner.TypeParams().Len() != 0 ||
		ps6087Named(methodSignature.Recv().Type()) != owner || ps6109TypeID(field.Type()) == contract.ProviderType {
		// The field is intentionally interface-typed in B2; equating its static
		// type with the provider would skip the required constructor proof.
		return false
	}
	if _, dynamic := types.Unalias(field.Type()).Underlying().(*types.Interface); !dynamic {
		return false
	}
	constructor, root, ok := context.ownerConstructor(owner, field, contract)
	if !ok || !context.ownerRuntimeClosed(owner, field, method, constructor, root, receiverObject, selector) {
		return false
	}
	return context.concreteAcquisition(contract)
}

func (context *ps6109Context) ownerConstructor(owner *types.Named, field *types.Var, contract *config.ReusableOneShotWrapperContract) (*types.Func, *types.Var, bool) {
	var constructor *types.Func
	var root *types.Var
	constructions := 0
	initializers := 0
	for functionObject, declaration := range context.locals {
		parents := ps6087Parents(declaration.Body)
		ast.Inspect(declaration.Body, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok || ps6109TypeID(context.pass.TypesInfo.TypeOf(literal)) != ps6109TypeID(owner) {
				return true
			}
			constructions++
			unary, addressed := parents[literal].(*ast.UnaryExpr)
			if !addressed || unary.Op != token.AND {
				return true
			}
			assignment, assigned := parents[unary].(*ast.AssignStmt)
			if !assigned || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
				return true
			}
			identifier, named := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
			if !named {
				return true
			}
			candidate, variable := context.pass.TypesInfo.Defs[identifier].(*types.Var)
			if !variable {
				return true
			}
			for _, statement := range declaration.Body.List {
				write, ok := statement.(*ast.AssignStmt)
				if !ok || len(write.Lhs) != 1 || len(write.Rhs) != 1 {
					continue
				}
				selected, ok := ps2110Unparen(write.Lhs[0]).(*ast.SelectorExpr)
				if !ok {
					continue
				}
				base, baseOK := ps2110Unparen(selected.X).(*ast.Ident)
				selection := context.pass.TypesInfo.Selections[selected]
				if !baseOK || selection == nil || selection.Obj() != field || context.pass.TypesInfo.ObjectOf(base) != candidate ||
					ps6109TypeID(context.pass.TypesInfo.TypeOf(write.Rhs[0])) != contract.ProviderType || !ps6109FreshProvider(write.Rhs[0]) {
					continue
				}
				initializers++
				constructor, root = functionObject, candidate
			}
			return true
		})
	}
	if constructions != 1 || initializers != 1 || constructor == nil || root == nil {
		return nil, nil, false
	}
	declaration := context.locals[constructor]
	if declaration == nil || len(declaration.Body.List) < 3 {
		return nil, nil, false
	}
	returned, ok := declaration.Body.List[len(declaration.Body.List)-1].(*ast.ReturnStmt)
	return constructor, root, ok && len(returned.Results) == 1 && ps6103Object(context.pass, returned.Results[0], root)
}

func (context *ps6109Context) ownerRuntimeClosed(owner *types.Named, field *types.Var, method, constructor *types.Func, root, receiver *types.Var, candidate *ast.SelectorExpr) bool {
	fieldUses := 0
	methodCalls := 0
	constructorCalls := 0
	rootUses := 0
	receiverUses := 0
	valid := true
	ownerID := ps6109TypeID(owner)
	for _, file := range context.pass.Files {
		parents := ps6087Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			if !valid {
				return false
			}
			selector, ok := node.(*ast.SelectorExpr)
			if ok {
				selection := context.pass.TypesInfo.Selections[selector]
				if selection != nil && selection.Obj() == field {
					fieldUses++
					if selector != candidate && !ps6109OwnerFieldInitializer(selector, root, parents) {
						valid = false
						return false
					}
				}
				selectedMethod, _ := context.pass.TypesInfo.Uses[selector.Sel].(*types.Func)
				if selectedMethod != nil && selectedMethod.Origin() == method.Origin() {
					methodCalls++
					receiverCall, receiverOK := ps2110Unparen(selector.X).(*ast.CallExpr)
					call, called := parents[selector].(*ast.CallExpr)
					if !receiverOK || !called || call.Fun != selector || !ps6109SynchronousOwnerCall(call, parents) {
						valid = false
						return false
					}
					callee, _, resolved := typedCallee(context.pass, receiverCall.Fun)
					if !resolved || callee.Origin() != constructor.Origin() {
						valid = false
						return false
					}
				}
			}

			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			object := context.pass.TypesInfo.ObjectOf(identifier)
			if called, ok := object.(*types.Func); ok && called.Origin() == constructor.Origin() {
				if context.pass.TypesInfo.Defs[identifier] == called {
					return true
				}
				constructorCalls++
				if !ps6109DirectOwnerMethodConstructor(identifier, method, parents, context.pass) {
					valid = false
					return false
				}
				return true
			}
			variable, ok := object.(*types.Var)
			if !ok || ps6109TypeID(variable.Type()) != ownerID {
				return true
			}
			if context.pass.TypesInfo.Defs[identifier] == variable {
				return true
			}
			if variable == root {
				rootUses++
				if ps6109OwnerRootUse(identifier, root, field, parents, context.pass) {
					return true
				}
			}
			if variable == receiver {
				receiverUses++
				if ps6109OwnerReceiverUse(identifier, candidate, parents) {
					return true
				}
			}
			valid = false
			return false
		})
	}
	return valid && fieldUses == 2 && methodCalls == 1 && constructorCalls == 1 && rootUses == 2 && receiverUses == 1
}

func ps6109OwnerFieldInitializer(selector *ast.SelectorExpr, root *types.Var, parents map[ast.Node]ast.Node) bool {
	base, ok := ps2110Unparen(selector.X).(*ast.Ident)
	assignment, assigned := parents[selector].(*ast.AssignStmt)
	return ok && assigned && assignment.Tok == token.ASSIGN && len(assignment.Lhs) == 1 && assignment.Lhs[0] == selector &&
		base != nil && root != nil
}

func ps6109OwnerRootUse(identifier *ast.Ident, root, field *types.Var, parents map[ast.Node]ast.Node, pass *analysis.Pass) bool {
	node := ast.Node(identifier)
	for {
		parenthesis, ok := parents[node].(*ast.ParenExpr)
		if !ok {
			break
		}
		node = parenthesis
	}
	if selector, ok := parents[node].(*ast.SelectorExpr); ok && selector.X == node {
		selection := pass.TypesInfo.Selections[selector]
		return selection != nil && selection.Obj() == field && pass.TypesInfo.ObjectOf(identifier) == root &&
			ps6109OwnerFieldInitializer(selector, root, parents)
	}
	returned, ok := parents[node].(*ast.ReturnStmt)
	return ok && len(returned.Results) == 1 && returned.Results[0] == node
}

func ps6109OwnerReceiverUse(identifier *ast.Ident, candidate *ast.SelectorExpr, parents map[ast.Node]ast.Node) bool {
	node := ast.Node(identifier)
	for {
		parenthesis, ok := parents[node].(*ast.ParenExpr)
		if !ok {
			break
		}
		node = parenthesis
	}
	return candidate.X == node
}

func ps6109DirectOwnerMethodConstructor(identifier *ast.Ident, method *types.Func, parents map[ast.Node]ast.Node, pass *analysis.Pass) bool {
	call, ok := parents[identifier].(*ast.CallExpr)
	if !ok || call.Fun != identifier || len(call.Args) != 0 || call.Ellipsis.IsValid() {
		return false
	}
	node := ast.Node(call)
	for {
		parenthesis, ok := parents[node].(*ast.ParenExpr)
		if !ok {
			break
		}
		node = parenthesis
	}
	selector, ok := parents[node].(*ast.SelectorExpr)
	if !ok {
		return false
	}
	selected, _ := pass.TypesInfo.Uses[selector.Sel].(*types.Func)
	outer, called := parents[selector].(*ast.CallExpr)
	return selected != nil && selected.Origin() == method.Origin() && called && outer.Fun == selector &&
		ps6109SynchronousOwnerCall(outer, parents)
}

func ps6109SynchronousOwnerCall(call *ast.CallExpr, parents map[ast.Node]ast.Node) bool {
	switch statement := parents[call].(type) {
	case *ast.ExprStmt:
		return statement.X == call
	case *ast.ReturnStmt:
		return len(statement.Results) == 1 && statement.Results[0] == call
	default:
		return false
	}
}

func ps6109FreshProvider(expression ast.Expr) bool {
	unary, ok := ps2110Unparen(expression).(*ast.UnaryExpr)
	if !ok || unary.Op != token.AND {
		return false
	}
	_, ok = ps2110Unparen(unary.X).(*ast.CompositeLit)
	return ok
}

func ps6109ContainingArgument(identifier *ast.Ident, parents map[ast.Node]ast.Node, call *ast.CallExpr) bool {
	var node ast.Node = identifier
	for parent := parents[node]; parent != nil; parent = parents[node] {
		if parent == call {
			for _, argument := range call.Args {
				if argument == node {
					return true
				}
			}
			return false
		}
		if _, ok := parent.(*ast.ParenExpr); !ok {
			return false
		}
		node = parent
	}
	return false
}

func ps6109ReceiverCall(identifier *ast.Ident, parents map[ast.Node]ast.Node) *ast.CallExpr {
	node := ast.Node(identifier)
	for {
		parenthesis, ok := parents[node].(*ast.ParenExpr)
		if !ok {
			break
		}
		node = parenthesis
	}
	selector, ok := parents[node].(*ast.SelectorExpr)
	if !ok || selector.X != node {
		return nil
	}
	call, _ := parents[selector].(*ast.CallExpr)
	if call == nil || call.Fun != selector {
		return nil
	}
	return call
}

func (context *ps6109Context) concreteAcquisition(contract *config.ReusableOneShotWrapperContract) bool {
	function := context.lookupMethod(contract.ConcreteAcquisition)
	if function == nil {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	return signature != nil && signature.Recv() != nil && ps6109TypeID(signature.Recv().Type()) == contract.ProviderType
}

func (context *ps6109Context) factoryProven(contract *config.ReusableOneShotWrapperContract) bool {
	start := context.lookupMethod(contract.ConcreteAcquisition)
	if start == nil {
		return false
	}
	if start.Pkg() != context.pass.Pkg {
		var fact ps6109FactoryFact
		return context.pass.ImportObjectFact(start.Origin(), &fact) && slices.Contains(fact.Proofs, ps6109ProducerFingerprint(contract))
	}
	return context.factoryProvenLocal(start, contract)
}

func (context *ps6109Context) factoryProvenLocal(start *types.Func, contract *config.ReusableOneShotWrapperContract) bool {
	signature, _ := start.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 {
		return false
	}
	seen := make(map[*types.Func]bool)
	return context.factoryReturns(start, signature.Recv(), contract, 0, seen)
}

func (context *ps6109Context) factoryReturns(function *types.Func, provider types.Object, contract *config.ReusableOneShotWrapperContract, depth int, seen map[*types.Func]bool) bool {
	if function == nil || depth > 2 || seen[function.Origin()] {
		return false
	}
	seen[function.Origin()] = true
	declaration := context.locals[function.Origin()]
	if declaration == nil || declaration.Body == nil {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Results() == nil || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 {
		return false
	}
	resultIndex := 0
	if depth == 0 {
		resultIndex = contract.AcquisitionWrapperResult - 1
	}
	if resultIndex < 0 || resultIndex >= signature.Results().Len() {
		return false
	}
	if len(declaration.Body.List) != 1 {
		return false
	}
	returned, ok := declaration.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != signature.Results().Len() {
		return false
	}
	for index, expression := range returned.Results {
		if index != resultIndex && !ps6109Nil(context.pass, expression) {
			return false
		}
	}
	call, ok := ps2110Unparen(returned.Results[resultIndex]).(*ast.CallExpr)
	if !ok || call.Ellipsis.IsValid() {
		return false
	}
	callee, _, ok := typedCallee(context.pass, call.Fun)
	if !ok || callee.Pkg() != context.pass.Pkg {
		return false
	}
	nextProvider, ok := ps6109ForwardedProvider(context.pass, call, callee, provider)
	if !ok {
		return false
	}
	if ps6090FunctionID(callee) == contract.WrapperConstructor {
		return context.constructorFresh(callee, nextProvider, contract)
	}
	return context.factoryReturns(callee, nextProvider, contract, depth+1, seen)
}

func (context *ps6109Context) constructorFresh(constructor *types.Func, provider types.Object, contract *config.ReusableOneShotWrapperContract) bool {
	declaration := context.locals[constructor.Origin()]
	if declaration == nil || declaration.Body == nil {
		return false
	}
	if len(declaration.Body.List) != 1 {
		return false
	}
	returned, ok := declaration.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	handleName := contract.NativeHandleField[strings.LastIndexByte(contract.NativeHandleField, '.')+1:]
	unary, ok := ps2110Unparen(returned.Results[0]).(*ast.UnaryExpr)
	if !ok || unary.Op != token.AND {
		return false
	}
	literal, ok := ps2110Unparen(unary.X).(*ast.CompositeLit)
	if !ok || ps6109TypeID(context.pass.TypesInfo.TypeOf(literal)) != contract.WrapperType {
		return false
	}
	if len(literal.Elts) != 1 {
		return false
	}
	handleInitialized := false
	for _, element := range literal.Elts {
		keyed, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		identifier, ok := ps2110Unparen(keyed.Key).(*ast.Ident)
		if !ok {
			return false
		}
		if identifier.Name != handleName {
			continue
		}
		call, ok := ps2110Unparen(keyed.Value).(*ast.CallExpr)
		if !ok || call.Ellipsis.IsValid() || ps6087FunctionID(context.pass, call) != contract.FreshNativeHandleFactory ||
			!ps6109NativeFactoryProvider(context.pass, call, provider) {
			return false
		}
		handleInitialized = true
	}
	return handleInitialized
}

func ps6109ForwardedProvider(pass *analysis.Pass, call *ast.CallExpr, callee *types.Func, provider types.Object) (types.Object, bool) {
	signature, _ := callee.Type().(*types.Signature)
	if signature == nil || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 {
		return nil, false
	}
	offset, ok := ps6099CallSignatureOffset(pass, call, signature)
	if !ok || len(call.Args)-offset != signature.Params().Len() {
		return nil, false
	}
	var next types.Object
	seen := 0
	if signature.Recv() != nil {
		receiver := ps6109CallReceiver(call)
		if receiver == nil || !ps6103Object(pass, receiver, provider) {
			return nil, false
		}
		next = signature.Recv()
		seen++
	}
	for index := range signature.Params().Len() {
		argument := call.Args[index+offset]
		if !ps6103Object(pass, argument, provider) {
			return nil, false
		}
		next = signature.Params().At(index)
		seen++
	}
	return next, seen == 1
}

func ps6109NativeFactoryProvider(pass *analysis.Pass, call *ast.CallExpr, provider types.Object) bool {
	callee, signature, ok := typedCallee(pass, call.Fun)
	if !ok || callee == nil || signature == nil || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 {
		return false
	}
	offset, ok := ps6099CallSignatureOffset(pass, call, signature)
	if !ok || len(call.Args)-offset != signature.Params().Len() {
		return false
	}
	uses := 0
	if signature.Recv() != nil {
		if !ps6103Object(pass, ps6109CallReceiver(call), provider) {
			return false
		}
		uses++
	}
	for index := range signature.Params().Len() {
		if !ps6103Object(pass, call.Args[index+offset], provider) {
			return false
		}
		uses++
	}
	return uses == 1
}

func ps6109ProducerFingerprint(contract *config.ReusableOneShotWrapperContract) string {
	parts := []string{
		contract.WrapperType, contract.ProviderType, contract.ConcreteAcquisition,
		contract.WrapperConstructor, contract.Terminal.Concrete, contract.Reset.Concrete,
		contract.FreshNativeHandleFactory, contract.NativeHandleField,
		strconv.Itoa(contract.AcquisitionWrapperResult), strconv.Itoa(contract.AcquisitionStatusResult),
		strconv.Itoa(contract.ConstructorWrapperResult), strconv.Itoa(contract.ResetStatusResult),
		contract.AcquisitionFailureMode, contract.ResetFailureState, strconv.Itoa(contract.SlotBound),
	}
	parts = append(parts, contract.MutableStateFields...)
	for _, method := range contract.AllowedSynchronousUses {
		parts = append(parts, method.Concrete)
	}
	return strings.Join(parts, "\x00")
}

func ps6109TypeID(value types.Type) string {
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	named, ok := value.(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil || named.TypeParams().Len() != 0 {
		return ""
	}
	return named.Obj().Pkg().Path() + "." + named.Obj().Name()
}

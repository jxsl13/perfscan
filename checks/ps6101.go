package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

const ps6101Message = "benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell"

var ps6101BenchmarkInputKeywords = [...]string{
	"weight", "prob", "scale", "variance", "dimension", "denom",
	"gate", "logit", "routing", "score", "mixture", "expert",
}

var ps6101AggregateKeywords = [...]string{"denom", "sum", "total", "norm", "scale", "mass", "score"}

// PS6101 starts the perfscan-original verification block. Benchmark validity is
// a verification concern even when the benchmark itself is statistically stable.
var PS6101 = register(&lint.Check{
	ID:       "PS6101",
	Category: "verify",
	Slug:     "random-input-bypasses-hot-branch",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "symmetric random benchmark inputs bypass a gated hot branch",
		Text: `A benchmark that feeds symmetric signed random values into a
sign/nonzero/threshold gate may time a cheap fallback rather than the named
kernel. This is a benchmark-validity issue: a statistically stable timing can
still measure the wrong branch mixture.

The detector follows object-specific input and aggregate values through direct
local helper and method calls, direct or stored numeric generic instantiations,
positional variadic calls, interface boxing, single/comma-ok assertions and type
switches, testing.B.Run sub-benchmarks, and testing.B.RunParallel workers.
Direct, method-expression, and stored testing.B method calls share the same
timer/callback semantics.
Pointer and named-pointer bindings preserve binding-local input facts through
multiple dereferences without relabeling the underlying storage outside scope.
Function values, receivers, arguments, deferred operands, sends, selectors,
composite elements, B.Run names/callbacks, and RunParallel callbacks are
evaluated exactly once in Go order.
It recognizes NormFloat64 or centered uniform input from math/rand and
math/rand/v2, but
rejects overwritten, shadowed, unsigned/nonnegative, untimed, or
control-flow-ambiguous values. Timer modeling accounts for conditional
ResetTimer/StartTimer/StopTimer calls (a gate is timed when any live path is
running), B.Loop setup and false-returning cleanup without stopping explicit
break, return, or goto exits, PB.Next worker loops, and the fact that a
parent benchmark using B.Run is not measured. RunParallel workers inherit the
parent timer state instead of starting a new timed sub-benchmark. Small
exact-length loops are simulated precisely; a shared bounded transfer budget
summarizes larger or nested for/range products with conservative chronological
segments at constant induction comparisons. Bounded for tails recheck their
real condition between segments and abandon fixed chronology when a body write
changes the stored bound or induction value.
Map allocation hints are never treated as lengths, and
mutations that can change map iteration invalidate fixed-trip assumptions. Unknown or capped loops preserve
inputs they cannot write and conservatively invalidate possible future writes.
Forward gotos and labeled loop controls merge
only paths that can reach their target, while backward-goto regions are treated
as control-ambiguous. Index-specific writes, nested keyed inputs, named returns,
stored closures and method values, early-return effects, and integer narrowing
retain provenance only while their bounds make that sound. Reference-bearing
fields inside copied structs and arrays keep their caller-visible pointee or
backing-storage identity across local calls, interface boxing and assertions,
promoted selections, preserving conversions, and aggregate returns; plain
value-only aggregates remain independent.
Writes through local call results preserve a must-proven caller location,
including nested reference-bearing fields of copied value aggregates, across
direct functions, concrete methods, method expressions and values, generic and
variadic wrappers, tuple bindings, and interface dispatch with one known local
dynamic target. Fresh, joined, imported, and otherwise opaque results are not
treated as caller-owned storage.
An explicit dereference of a copied pointer field consumes that field value's
direct pointee without advancing through an additional synthetic owner slot.
Active local-call traversal is capped before nested CFG snapshots can grow
quadratically; reaching the cap still evaluates operands in Go order and then
conservatively discards captured provenance.
Append modeling distinguishes reuse from allocation and accounts for slice
offsets. Conditions and tagged-switch tags are evaluated once even when proof
classification also inspects them. Switch case matching keeps true and false
short-circuit outcomes separate, so only an executed conjunction RHS can reach
its selected body while the skipped state continues through no-match. Negated conjunctions and
disjunctions follow De Morgan control semantics. Tagged switches model typed
constant cases, Boolean polarity, default complements, reachability, and
fallthrough. A product is treated as a square only when both operands carry the
same nonzero value identity, conversion path, and sign. A proof must cover the same input, aggregate,
comparison, threshold, and counter revision as the timed hot gate. Prove the
hot path with a branch-entry counter or an explicit strictly-positive input
fixture, and keep a separate zero/degenerate control cell; do not silently
replace the distribution. Counter evidence starts only from a proven explicit
or implicit numeric zero, accepts exclusively positive gate-linked increments,
and is discarded by conflicting branches, reassignment, aliases, escapes, or
opaque calls.`,
		MeasuredWin: "Issue #907: stable timing is not evidence that the intended branch ran.",
	},
	Analyzer: &analysis.Analyzer{Name: "PS6101", Doc: "random benchmark inputs bypass a gated hot branch", Run: runPS6101},
})

type ps6101Kind uint8

const (
	ps6101Unknown ps6101Kind = iota
	ps6101Zero
	ps6101Symmetric
	ps6101Nonnegative
	ps6101Positive
)

type ps6101Value struct {
	kind       ps6101Kind
	sources    map[token.Pos]bool
	eligible   bool
	aggregate  bool
	constant   constant.Value
	nonempty   bool
	threshold  bool
	testing    bool
	revision   uint64
	identity   uint64
	analysis   *ps6101ValueAnalysis
	reference  *ps6101Location
	callable   *ps6101Callable
	length     int64
	capacity   int64
	offset     int64
	elements   map[int64]ps6101Value
	lengthOK   bool
	capacityOK bool
	offsetOK   bool
}

type ps6101ValueAnalysis struct {
	lower      constant.Value
	upper      constant.Value
	excluded   map[string]bool
	squareID   uint64
	squareSig  string
	squareSign int8
	dynamic    types.Type
	fields     map[string]ps6101Value
}

type ps6101Callable struct {
	literal           *ast.FuncLit
	function          *ast.FuncDecl
	interfaceMethod   *types.Func
	receiver          ast.Expr
	receiverValue     *ps6101Value
	receiverFields    map[string]ps6101Value
	receiverFromFirst bool
	testingMethod     string
	typeArguments     map[*types.TypeParam]types.Type
}

type ps6101Location struct {
	root types.Object
	path string
}

// ps6101StoreTarget is the storage denoted by an assignment left-hand side
// after its runtime operands have been evaluated. Go evaluates those operands
// before the right-hand side, so retaining the resolved locations is both more
// accurate and necessary for lvalues rooted in a local call result.
type ps6101StoreTarget struct {
	location          ps6101Location
	indexedParent     ps6101Location
	ambiguous         []ps6101Location
	valid             bool
	untrackedIndirect bool
	isIndexed         bool
	dynamicIndex      bool
	mapIndex          bool
}

type ps6101GateKey struct {
	sources   string
	op        token.Token
	threshold string
	revision  uint64
}

type ps6101Gate struct {
	key ps6101GateKey
	pos token.Pos
}

type ps6101CounterEvidence struct {
	gates       map[ps6101GateKey]bool
	sites       map[token.Pos]bool
	revision    uint64
	incremented bool
}

type ps6101Flow uint8

const (
	ps6101FallsThrough ps6101Flow = 1 << iota
	ps6101Returns
	ps6101Breaks
	ps6101Continues
	ps6101Gotos
)

type ps6101TimerState uint8

const (
	ps6101TimerRunning ps6101TimerState = 1 << iota
	ps6101TimerStopped
)

type ps6101ExitState struct {
	state        map[ps6101Location]ps6101Value
	aliases      map[types.Object]ps6101Location
	gates        []ps6101Gate
	counterGates map[ps6101Location]ps6101CounterEvidence
	counterRevs  map[ps6101Location]uint64
	timer        ps6101TimerState
	recoverable  int
}

type ps6101ControlStates struct {
	breaks    []ps6101ExitState
	continues []ps6101ExitState
}

type ps6101Diagnostics struct {
	gates    []ps6101Gate
	subGates []ps6101Gate
	proofs   map[ps6101GateKey]bool
}

type ps6101LoopSegment struct {
	lower int64
	upper int64
}

const ps6101ExactLoopTransferLimit = 256

const ps6101ReferenceFieldSnapshotLimit = 256

const ps6101CallDepthLimit = 512

type ps6101LoopTransferBudget struct {
	remaining         int
	spent             int
	abstractRemaining int
	abstractSpent     int
}

type ps6101Engine struct {
	pass            *analysis.Pass
	functions       map[types.Object]*ast.FuncDecl
	state           map[ps6101Location]ps6101Value
	aliases         map[types.Object]ps6101Location
	active          map[types.Object]bool
	activeLiteral   map[*ast.FuncLit]bool
	gates           []ps6101Gate
	subGates        []ps6101Gate
	proofs          map[ps6101GateKey]bool
	counterGates    map[ps6101Location]ps6101CounterEvidence
	counterRevs     map[ps6101Location]uint64
	activeGates     []ps6101Gate
	returns         [][]ps6101Value
	exits           []ps6101ExitState
	resultObjects   []types.Object
	resultTypes     []types.Type
	conditional     int
	nextRevision    uint64
	timer           ps6101TimerState
	recoverable     int
	evalCache       map[ast.Expr]ps6101Value
	boolCache       map[ast.Expr]int
	jumps           map[types.Object][]ps6101ExitState
	controls        map[types.Object]*ps6101ControlStates
	breakTargets    []*ps6101ControlStates
	continueTargets []*ps6101ControlStates
	counterIndex    map[types.Object][]ps6101Location
	loopTransfers   *ps6101LoopTransferBudget
	loopAbstract    int
	callDepth       int
	typeArguments   map[*types.TypeParam]types.Type
}

func runPS6101(pass *analysis.Pass) (any, error) {
	functions := make(map[types.Object]*ast.FuncDecl)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			if object := identObject(pass, function.Name); object != nil {
				functions[object] = function
			}
		}
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			benchmark, ok := declaration.(*ast.FuncDecl)
			if !ok || !ps6101IsBenchmark(pass, benchmark) {
				continue
			}
			engine := &ps6101Engine{
				pass: pass, functions: functions, state: make(map[ps6101Location]ps6101Value),
				aliases: make(map[types.Object]ps6101Location), active: make(map[types.Object]bool), activeLiteral: make(map[*ast.FuncLit]bool),
				proofs: make(map[ps6101GateKey]bool), counterGates: make(map[ps6101Location]ps6101CounterEvidence),
				counterRevs: make(map[ps6101Location]uint64), timer: ps6101TimerRunning,
				jumps: make(map[types.Object][]ps6101ExitState), controls: make(map[types.Object]*ps6101ControlStates),
				loopTransfers: &ps6101LoopTransferBudget{
					remaining: ps6101ExactLoopTransferLimit, abstractRemaining: ps6101ExactLoopTransferLimit,
				},
			}
			engine.indexCounterLocations(benchmark.Body)
			for _, field := range benchmark.Type.Params.List {
				for _, name := range field.Names {
					engine.state[ps6101Location{root: identObject(pass, name)}] = ps6101Value{testing: true}
				}
			}
			engine.analyzeFunction(benchmark, nil, nil, nil)
			var positions []token.Pos
			for _, gate := range engine.subGates {
				if !engine.proofs[gate.key] {
					positions = append(positions, gate.pos)
				}
			}
			for _, gate := range engine.gates {
				if !engine.proofs[gate.key] {
					positions = append(positions, gate.pos)
				}
			}
			if len(positions) > 0 {
				slices.Sort(positions)
				pass.Reportf(positions[0], ps6101Message)
			}
		}
	}
	return nil, nil
}

func ps6101IsBenchmark(pass *analysis.Pass, function *ast.FuncDecl) bool {
	if function == nil || function.Body == nil || !strings.HasPrefix(function.Name.Name, "Benchmark") || function.Recv != nil {
		return false
	}
	object, ok := identObject(pass, function.Name).(*types.Func)
	if !ok {
		return false
	}
	signature, ok := object.Type().(*types.Signature)
	if !ok || signature.Params().Len() != 1 || signature.Results().Len() != 0 {
		return false
	}
	pointer, ok := types.Unalias(signature.Params().At(0).Type()).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(pointer.Elem()).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "testing" && named.Obj().Name() == "B"
}

func (engine *ps6101Engine) analyzeFunction(function *ast.FuncDecl, call *ast.CallExpr, receiver ast.Expr, callable *ps6101Callable) []ps6101Value {
	object := identObject(engine.pass, function.Name)
	oldTypeArguments := engine.typeArguments
	engine.typeArguments = engine.bindTypeArguments(function, call, callable)
	callerRoots, callerAliases := engine.callerVisibility()
	// Go evaluates a call's function value and arguments before entering the
	// callee. Bind the parameter snapshots before marking this function active:
	// f(f(x)) is two finite, non-recursive calls, while a call reached from f's
	// body still observes the outer activation after its own operands run.
	if call != nil {
		engine.bindCall(function, call, receiver, callable)
	}
	if call != nil && !engine.enterCall() {
		engine.invalidateCapturedState()
		engine.retainCallerVisibility(callerRoots, callerAliases)
		engine.typeArguments = oldTypeArguments
		return nil
	}
	if call != nil {
		defer engine.leaveCall()
	}
	if object != nil && engine.active[object] {
		engine.invalidateCapturedState()
		engine.retainCallerVisibility(callerRoots, callerAliases)
		engine.typeArguments = oldTypeArguments
		return nil
	}
	if object != nil {
		engine.active[object] = true
		defer delete(engine.active, object)
	}
	engine.clearLocals(function)
	oldReturns := engine.returns
	engine.returns = nil
	oldExits := engine.exits
	engine.exits = nil
	oldJumps := engine.jumps
	engine.jumps = make(map[types.Object][]ps6101ExitState)
	oldControls := engine.controls
	engine.controls = make(map[types.Object]*ps6101ControlStates)
	oldResultObjects := engine.resultObjects
	engine.resultObjects = engine.namedResults(function.Type.Results)
	oldResultTypes := engine.resultTypes
	engine.resultTypes = engine.fieldTypes(function.Type.Results)
	for _, result := range engine.resultObjects {
		engine.killPrefix(ps6101Location{root: result})
		delete(engine.aliases, result)
	}
	oldRecoverable := engine.recoverable
	flow := engine.analyzeBlock(function.Body)
	engine.discardProofsForUnresolvedJumps()
	if flow&ps6101FallsThrough != 0 {
		engine.captureExit()
	}
	results := engine.mergeReturns(engine.returns)
	engine.mergeExits()
	if call != nil {
		engine.retainCallerVisibility(callerRoots, callerAliases)
	}
	engine.returns = oldReturns
	engine.exits = oldExits
	engine.jumps = oldJumps
	engine.controls = oldControls
	engine.recoverable = oldRecoverable
	engine.resultObjects = oldResultObjects
	engine.resultTypes = oldResultTypes
	engine.typeArguments = oldTypeArguments
	return results
}

func (engine *ps6101Engine) bindTypeArguments(function *ast.FuncDecl, call *ast.CallExpr, callable *ps6101Callable) map[*types.TypeParam]types.Type {
	bound := maps.Clone(engine.typeArguments)
	if bound == nil {
		bound = make(map[*types.TypeParam]types.Type)
	}
	if callable != nil {
		for parameter, argument := range callable.typeArguments {
			bound[parameter] = engine.resolveTypeArgument(argument)
		}
	}
	if function == nil || call == nil {
		return bound
	}
	object, ok := identObject(engine.pass, function.Name).(*types.Func)
	if !ok {
		return bound
	}
	signature, ok := object.Type().(*types.Signature)
	if !ok || signature.TypeParams() == nil {
		return bound
	}
	identifier := engine.instantiationIdentifier(call.Fun)
	instance, ok := engine.pass.TypesInfo.Instances[identifier]
	if !ok || instance.TypeArgs == nil || instance.TypeArgs.Len() != signature.TypeParams().Len() {
		return bound
	}
	for index := 0; index < signature.TypeParams().Len(); index++ {
		bound[signature.TypeParams().At(index)] = engine.resolveTypeArgument(instance.TypeArgs.At(index))
	}
	return bound
}

func (engine *ps6101Engine) enterCall() bool {
	if engine.callDepth >= ps6101CallDepthLimit {
		return false
	}
	engine.callDepth++
	return true
}

func (engine *ps6101Engine) leaveCall() {
	if engine.callDepth > 0 {
		engine.callDepth--
	}
}

func (engine *ps6101Engine) resolveTypeArgument(typ types.Type) types.Type {
	if typ == nil {
		return nil
	}
	seen := make(map[*types.TypeParam]bool)
	for {
		parameter, ok := types.Unalias(typ).(*types.TypeParam)
		if !ok || seen[parameter] {
			return typ
		}
		seen[parameter] = true
		next := engine.typeArguments[parameter]
		if next == nil {
			return typ
		}
		typ = next
	}
}

func (engine *ps6101Engine) namedResults(results *ast.FieldList) []types.Object {
	if results == nil {
		return nil
	}
	var objects []types.Object
	for _, field := range results.List {
		for _, name := range field.Names {
			if object := identObject(engine.pass, name); object != nil {
				objects = append(objects, object)
			}
		}
	}
	return objects
}

func (engine *ps6101Engine) fieldTypes(fields *ast.FieldList) []types.Type {
	if fields == nil {
		return nil
	}
	var result []types.Type
	for _, field := range fields.List {
		count := max(1, len(field.Names))
		typ := engine.pass.TypesInfo.TypeOf(field.Type)
		for range count {
			result = append(result, typ)
		}
	}
	return result
}

func (engine *ps6101Engine) clearLocals(function *ast.FuncDecl) {
	engine.clearNodeLocals(function.Body)
}

func (engine *ps6101Engine) clearNodeLocals(root ast.Node) {
	ast.Inspect(root, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := engine.pass.TypesInfo.Defs[identifier]
		if object != nil {
			engine.killPrefix(ps6101Location{root: object})
			delete(engine.aliases, object)
		}
		return true
	})
}

func (engine *ps6101Engine) bindCall(function *ast.FuncDecl, call *ast.CallExpr, receiver ast.Expr, callable *ps6101Callable) {
	if function.Recv != nil && len(function.Recv.List) == 1 && len(function.Recv.List[0].Names) == 1 {
		if callable != nil && callable.receiverValue != nil {
			engine.bindParameterSnapshot(function.Recv.List[0].Names[0], callable)
		} else {
			engine.bindParameter(function.Recv.List[0].Names[0], receiver)
		}
	}
	engine.bindParameters(function.Type.Params, call)
}

// bindParameters evaluates call arguments in Go's left-to-right order and
// snapshots each parameter at the point its argument is evaluated. A
// positional variadic call needs an explicit fresh slice: binding only the
// first trailing expression loses both later side effects and provenance.
func (engine *ps6101Engine) bindParameters(parameters *ast.FieldList, call *ast.CallExpr) {
	if parameters == nil || call == nil {
		return
	}
	argument := 0
	for _, field := range parameters.List {
		if _, variadic := field.Type.(*ast.Ellipsis); variadic {
			if call.Ellipsis.IsValid() {
				if argument < len(call.Args) {
					if len(field.Names) > 0 {
						engine.bindParameter(field.Names[0], call.Args[argument])
					} else {
						engine.eval(call.Args[argument])
					}
					argument++
				}
			} else {
				var name *ast.Ident
				if len(field.Names) > 0 {
					name = field.Names[0]
				}
				engine.bindVariadicParameter(name, call.Args[argument:])
				argument = len(call.Args)
			}
			break
		}
		if len(field.Names) == 0 {
			if argument < len(call.Args) {
				engine.eval(call.Args[argument])
				argument++
			}
			continue
		}
		for _, name := range field.Names {
			if argument >= len(call.Args) {
				return
			}
			engine.bindParameter(name, call.Args[argument])
			argument++
		}
	}
	// A type-correct call has no arguments left here. Evaluating any remainder
	// keeps the transfer conservative when analysis is run on partially broken
	// source or a future syntax form that changes parameter grouping.
	for ; argument < len(call.Args); argument++ {
		engine.eval(call.Args[argument])
	}
}

func (engine *ps6101Engine) bindVariadicParameter(name *ast.Ident, arguments []ast.Expr) {
	var object types.Object
	var elementType types.Type
	if name != nil {
		object = identObject(engine.pass, name)
		if object != nil {
			if slice, ok := types.Unalias(object.Type()).Underlying().(*types.Slice); ok {
				elementType = slice.Elem()
			}
		}
	}
	values := make([]ps6101Value, 0, len(arguments))
	for _, argument := range arguments {
		value := engine.eval(argument)
		value = engine.assignmentValue(elementType, engine.pass.TypesInfo.TypeOf(argument), value)
		values = append(values, value)
	}
	if name == nil {
		return
	}
	if object == nil {
		return
	}
	destination := ps6101Location{root: object}
	result := ps6101ImplicitZeroValue(object.Type())
	elements := make(map[int64]ps6101Value, len(values))
	for index, value := range values {
		result = ps6101CollectionMerge(result, value)
		elements[int64(index)] = ps6101CloneValue(value)
	}
	result.elements = elements
	result.length, result.capacity = int64(len(values)), int64(len(values))
	result.lengthOK, result.capacityOK, result.offsetOK = true, true, true
	result.nonempty = len(values) > 0
	// A positional variadic call constructs fresh backing storage. Keeping a
	// self reference lets nested helpers mutate its elements without aliasing
	// any scalar caller argument.
	result.reference = ps6101Reference(destination)
	engine.store(name, result, token.ASSIGN)
}

func (engine *ps6101Engine) bindParameterSnapshot(name *ast.Ident, callable *ps6101Callable) {
	object := identObject(engine.pass, name)
	if object == nil || callable == nil || callable.receiverValue == nil {
		return
	}
	destination := ps6101Location{root: object}
	engine.killPrefix(destination)
	delete(engine.aliases, object)
	value := ps6101CloneValue(*callable.receiverValue)
	ps6101ApplyNamedValueSemantics(name.Name, &value)
	engine.state[destination] = value
	for suffix, value := range callable.receiverFields {
		location := destination
		if strings.HasPrefix(suffix, "[") {
			location.path = suffix
		} else {
			location.path = strings.TrimPrefix(suffix, ".")
		}
		engine.state[location] = ps6101CloneValue(value)
	}
	if ps6101ReferenceLike(object.Type()) && callable.receiverValue.reference != nil {
		engine.aliases[object] = *callable.receiverValue.reference
	}
}

func (engine *ps6101Engine) snapshotCallableReceiver(callable *ps6101Callable, expression ast.Expr, evaluated ps6101Value) {
	if callable == nil || expression == nil {
		return
	}
	value := ps6101CloneValue(evaluated)
	callable.receiverValue = &value
	source, ok := engine.location(expression, true)
	if !ok {
		return
	}
	fields := make(map[string]ps6101Value, len(engine.state))
	for location, stored := range engine.state {
		if !ps6101HasLocationPrefix(location, source) || location == source {
			continue
		}
		suffix := strings.TrimPrefix(location.path, source.path)
		fields[suffix] = ps6101CloneValue(stored)
	}
	callable.receiverFields = fields
}

func (engine *ps6101Engine) bindParameter(name *ast.Ident, argument ast.Expr) {
	object := identObject(engine.pass, name)
	if object == nil || argument == nil {
		return
	}
	destination := ps6101Location{root: object}
	engine.killPrefix(destination)
	delete(engine.aliases, object)
	value := engine.assignmentValue(object.Type(), engine.pass.TypesInfo.TypeOf(argument), engine.eval(argument))
	ps6101ApplyNamedValueSemantics(name.Name, &value)
	engine.state[destination] = ps6101CloneValue(value)
	engine.storeValueFields(destination, value.fieldValues())
	if composite := ps6101CompositeLiteral(argument); composite != nil {
		engine.finalizeCompositeFields(destination, composite)
	}
	if source, ok := engine.location(argument, true); ok {
		engine.copyPrefix(source, destination)
		if ps6101ReferenceLike(object.Type()) {
			// The evaluated value carries the direct pointee of a pointer
			// value. Prefer it over the transitively followed expression
			// location: for an asserted **T, location(assertion, true) is the
			// final T storage, while the parameter must still point at the
			// intermediate *T object.
			if value.reference != nil {
				engine.aliases[object] = *value.reference
			} else {
				engine.aliases[object] = source
			}
		}
	}
}

func ps6101InterfaceType(typ types.Type) bool {
	if typ == nil {
		return false
	}
	typ = types.Unalias(typ)
	if _, typeParameter := typ.(*types.TypeParam); typeParameter {
		return false
	}
	_, ok := typ.Underlying().(*types.Interface)
	return ok
}

// assignmentValue records the concrete dynamic type when an implicit or
// explicit interface conversion boxes a tracked value. This lets a later type
// assertion preserve provenance without pretending that an incompatible
// assertion can succeed.
func (engine *ps6101Engine) assignmentValue(target, source types.Type, value ps6101Value) ps6101Value {
	target = engine.resolveTypeArgument(target)
	source = engine.resolveTypeArgument(source)
	if !ps6101InterfaceType(target) {
		return value
	}
	if source != nil && !ps6101InterfaceType(source) {
		value.mutableAnalysis().dynamic = source
	}
	return value
}

func (engine *ps6101Engine) assertionCompatible(dynamic, target types.Type) bool {
	dynamic = engine.resolveTypeArgument(dynamic)
	target = engine.resolveTypeArgument(target)
	if dynamic == nil || target == nil {
		return false
	}
	if ps6101InterfaceType(target) {
		return types.AssignableTo(dynamic, target)
	}
	return types.Identical(types.Unalias(dynamic), types.Unalias(target))
}

func (engine *ps6101Engine) typeAssertionTarget(expression *ast.TypeAssertExpr) types.Type {
	if expression == nil || expression.Type == nil {
		return nil
	}
	return engine.resolveTypeArgument(engine.pass.TypesInfo.TypeOf(expression.Type))
}

// evalTypeAssertion distinguishes the panicking single-result form from the
// comma-ok form. go/types assigns a tuple type to the expression in the latter
// context; the asserted type is therefore read from expression.Type instead
// of TypesInfo.TypeOf(expression).
func (engine *ps6101Engine) evalTypeAssertion(expression *ast.TypeAssertExpr, panicking bool) (ps6101Value, ps6101Value) {
	result := engine.eval(expression.X)
	target := engine.typeAssertionTarget(expression)
	dynamic := result.analysisValue().dynamic
	if !engine.assertionCompatible(dynamic, target) {
		if dynamic != nil && panicking {
			// Only the single-result form makes later code unreachable. A failed
			// comma-ok assertion returns the target's zero value and false.
			engine.invalidateCapturedState()
		}
		failed := ps6101Value{}
		if dynamic != nil {
			failed = ps6101ImplicitZeroValue(target)
			return failed, ps6101ConstantValue(constant.MakeBool(false))
		}
		return failed, ps6101Value{}
	}
	if !ps6101InterfaceType(target) {
		result.mutableAnalysis().dynamic = nil
	}
	return result, ps6101ConstantValue(constant.MakeBool(true))
}

func ps6101ReferenceLike(typ types.Type) bool {
	switch types.Unalias(typ).Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map:
		return true
	}
	return false
}

// ps6101ContainsReference reports whether copying a value can still leave the
// copy connected to caller-visible storage. In particular, a struct or array
// containing a slice is not independent merely because the outer value was
// passed by value.
func ps6101ContainsReference(typ types.Type) bool {
	return ps6101ContainsReferenceSeen(typ, make(map[types.Type]bool))
}

func ps6101ContainsReferenceSeen(typ types.Type, seen map[types.Type]bool) bool {
	if typ == nil {
		return false
	}
	typ = types.Unalias(typ)
	if seen[typ] {
		return false
	}
	seen[typ] = true
	switch value := typ.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Interface, *types.Signature, *types.Chan:
		return true
	case *types.Array:
		return ps6101ContainsReferenceSeen(value.Elem(), seen)
	case *types.Struct:
		for index := 0; index < value.NumFields(); index++ {
			if ps6101ContainsReferenceSeen(value.Field(index).Type(), seen) {
				return true
			}
		}
	}
	return false
}

func (engine *ps6101Engine) analyzeBlock(block *ast.BlockStmt) ps6101Flow {
	if block == nil {
		return ps6101FallsThrough
	}
	flow := ps6101FallsThrough
	for _, statement := range block.List {
		if labeled, ok := statement.(*ast.LabeledStmt); ok {
			label := identObject(engine.pass, labeled.Label)
			incoming := engine.jumps[label]
			delete(engine.jumps, label)
			if len(incoming) > 0 {
				if flow&ps6101FallsThrough != 0 {
					incoming = append(incoming, engine.currentExitState())
				}
				engine.mergeJumpStates(incoming)
				flow |= ps6101FallsThrough
			}
		}
		if flow&ps6101FallsThrough == 0 {
			continue
		}
		statementFlow := engine.analyzeStatement(statement)
		flow = flow&^ps6101FallsThrough | statementFlow
	}
	return flow
}

func (engine *ps6101Engine) analyzeStatement(statement ast.Stmt) ps6101Flow {
	switch value := statement.(type) {
	case *ast.AssignStmt:
		engine.assign(value)
	case *ast.DeclStmt:
		if declaration, ok := value.Decl.(*ast.GenDecl); ok {
			for _, specification := range declaration.Specs {
				if values, ok := specification.(*ast.ValueSpec); ok {
					engine.assignValues(values.Names, values.Values)
				}
			}
		}
	case *ast.IncDecStmt:
		engine.increment(value)
	case *ast.ExprStmt:
		engine.eval(value.X)
		if call, ok := ps6101Unparen(value.X).(*ast.CallExpr); ok && engine.terminalFailureCall(call) {
			return ps6101Returns
		}
	case *ast.ReturnStmt:
		var result []ps6101Value
		if len(value.Results) == 0 && len(engine.resultObjects) > 0 {
			result = make([]ps6101Value, 0, len(engine.resultObjects))
			for _, object := range engine.resultObjects {
				location := ps6101Location{root: object}
				result = append(result, engine.valueWithStoredFields(location, engine.state[location], object.Type()))
			}
		}
		if len(value.Results) == 1 {
			if call, ok := ps6101Unparen(value.Results[0]).(*ast.CallExpr); ok {
				result = engine.evalCall(call)
			}
		}
		if result == nil {
			result = make([]ps6101Value, 0, len(value.Results))
			for _, expression := range value.Results {
				result = append(result, engine.returnValue(expression))
			}
		}
		sourceTypes := engine.expressionTypes(value.Results)
		for index := range result {
			if index < len(engine.resultTypes) {
				var sourceType types.Type
				if index < len(sourceTypes) {
					sourceType = sourceTypes[index]
				}
				result[index] = engine.assignmentValue(engine.resultTypes[index], sourceType, result[index])
			}
		}
		engine.returns = append(engine.returns, result)
		engine.captureExit()
		return ps6101Returns
	case *ast.BranchStmt:
		if value.Label != nil && (value.Tok == token.BREAK || value.Tok == token.CONTINUE) {
			target := identObject(engine.pass, value.Label)
			if target != nil {
				control := engine.controls[target]
				if control == nil {
					control = &ps6101ControlStates{}
					engine.controls[target] = control
				}
				if value.Tok == token.BREAK {
					control.breaks = append(control.breaks, engine.currentExitState())
				} else {
					control.continues = append(control.continues, engine.currentExitState())
				}
			}
			return ps6101Gotos
		}
		switch value.Tok {
		case token.BREAK:
			if len(engine.breakTargets) > 0 {
				target := engine.breakTargets[len(engine.breakTargets)-1]
				target.breaks = append(target.breaks, engine.currentExitState())
			}
			return ps6101Breaks
		case token.CONTINUE:
			if len(engine.continueTargets) > 0 {
				target := engine.continueTargets[len(engine.continueTargets)-1]
				target.continues = append(target.continues, engine.currentExitState())
			}
			return ps6101Continues
		case token.GOTO:
			if target := identObject(engine.pass, value.Label); target != nil {
				engine.jumps[target] = append(engine.jumps[target], engine.currentExitState())
			}
			return ps6101Gotos
		}
	case *ast.IfStmt:
		return engine.analyzeIf(value)
	case *ast.ForStmt:
		return engine.analyzeFor(value)
	case *ast.RangeStmt:
		return engine.analyzeRangeLabeled(value, nil)
	case *ast.BlockStmt:
		return engine.analyzeBlock(value)
	case *ast.LabeledStmt:
		label := identObject(engine.pass, value.Label)
		switch child := value.Stmt.(type) {
		case *ast.ForStmt:
			return engine.analyzeForLabeled(child, label)
		case *ast.RangeStmt:
			return engine.analyzeRangeLabeled(child, label)
		default:
			flow := engine.analyzeStatement(child)
			if control := engine.controls[label]; control != nil && len(control.breaks) > 0 {
				states := control.breaks
				if flow&ps6101FallsThrough != 0 {
					states = append(states, engine.currentExitState())
				}
				engine.mergeJumpStates(states)
				flow |= ps6101FallsThrough
				delete(engine.controls, label)
			}
			return flow
		}
	case *ast.SwitchStmt:
		return engine.analyzeSwitch(value)
	case *ast.TypeSwitchStmt:
		return engine.analyzeTypeSwitch(value)
	case *ast.SelectStmt:
		return engine.analyzeSelect(value)
	case *ast.DeferStmt:
		engine.evaluateCallOperands(value.Call)
		if engine.deferredCallCanRecover(value.Call) {
			engine.recoverable++
		}
	case *ast.SendStmt:
		engine.eval(value.Chan)
		engine.eval(value.Value)
		// A sent reference escapes to a receiver that may mutate it before
		// later benchmark code observes the same storage.
		engine.invalidateSharedArgument(value.Value)
	case *ast.GoStmt:
		outerRecoverable := engine.recoverable
		engine.recoverable = 0
		engine.evalCall(value.Call)
		if selector, ok := ps6101Unparen(value.Call.Fun).(*ast.SelectorExpr); ok {
			engine.invalidateSharedArgument(selector.X)
		}
		for _, argument := range value.Call.Args {
			engine.invalidateSharedArgument(argument)
		}
		engine.recoverable = outerRecoverable
	}
	return ps6101FallsThrough
}

func (engine *ps6101Engine) evaluateCallOperands(call *ast.CallExpr) {
	if call == nil {
		return
	}
	engine.eval(call.Fun)
	for _, argument := range call.Args {
		engine.eval(argument)
	}
}

func (engine *ps6101Engine) analyzeSelect(statement *ast.SelectStmt) ps6101Flow {
	if statement == nil || statement.Body == nil {
		return ps6101FallsThrough
	}
	// The channel operands of every case and every send RHS are evaluated once
	// on entry, before Go chooses a clause. Clause-local receive assignments are
	// applied later only on the selected branch.
	for _, raw := range statement.Body.List {
		if clause, ok := raw.(*ast.CommClause); ok {
			engine.evaluateSelectCommOperands(clause.Comm)
		}
	}
	entry := engine.currentExitState()
	control := &ps6101ControlStates{}
	engine.breakTargets = append(engine.breakTargets, control)
	flow := ps6101Flow(0)
	var exits []ps6101ExitState
	for _, raw := range statement.Body.List {
		clause, ok := raw.(*ast.CommClause)
		if !ok {
			continue
		}
		engine.state, engine.aliases = engine.copyState(entry.state), engine.copyAliases(entry.aliases)
		engine.gates = slices.Clone(entry.gates)
		engine.counterGates = ps6101CopyCounterGates(entry.counterGates)
		engine.counterRevs = ps6101CopyCounterRevisions(entry.counterRevs)
		engine.timer, engine.recoverable = entry.timer, entry.recoverable
		engine.conditional++
		clauseFlow := ps6101FallsThrough
		if clause.Comm != nil {
			clauseFlow = engine.applySelectComm(clause.Comm)
		}
		for _, child := range clause.Body {
			if clauseFlow&ps6101FallsThrough == 0 {
				break
			}
			childFlow := engine.analyzeStatement(child)
			clauseFlow = clauseFlow&^ps6101FallsThrough | childFlow
		}
		engine.conditional--
		flow |= clauseFlow &^ (ps6101FallsThrough | ps6101Breaks)
		if clauseFlow&ps6101FallsThrough != 0 {
			exits = append(exits, engine.currentExitState())
		}
	}
	engine.breakTargets = engine.breakTargets[:len(engine.breakTargets)-1]
	exits = append(exits, control.breaks...)
	if len(exits) > 0 {
		engine.mergeJumpStates(exits)
		flow |= ps6101FallsThrough
	}
	return flow
}

func (engine *ps6101Engine) evaluateSelectCommOperands(statement ast.Stmt) {
	switch value := statement.(type) {
	case nil:
		return
	case *ast.SendStmt:
		engine.eval(value.Chan)
		engine.eval(value.Value)
		engine.invalidateSharedArgument(value.Value)
	case *ast.ExprStmt:
		if receive, ok := ps6101Unparen(value.X).(*ast.UnaryExpr); ok && receive.Op == token.ARROW {
			engine.eval(receive.X)
		}
	case *ast.AssignStmt:
		for _, expression := range value.Rhs {
			if receive, ok := ps6101Unparen(expression).(*ast.UnaryExpr); ok && receive.Op == token.ARROW {
				engine.eval(receive.X)
			}
		}
	}
}

func (engine *ps6101Engine) applySelectComm(statement ast.Stmt) ps6101Flow {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok {
		return ps6101FallsThrough
	}
	for _, left := range assignment.Lhs {
		engine.store(left, ps6101Value{}, assignment.Tok)
	}
	return ps6101FallsThrough
}

func (engine *ps6101Engine) returnValue(expression ast.Expr) ps6101Value {
	value := engine.eval(expression)
	if source, ok := engine.location(expression, true); ok {
		value = engine.valueWithStoredFields(source, value, engine.pass.TypesInfo.TypeOf(expression))
		if value.reference == nil && ps6101ReferenceLike(engine.pass.TypesInfo.TypeOf(expression)) {
			value.reference = ps6101Reference(source)
		}
	}
	return value
}

func (engine *ps6101Engine) expressionTypes(expressions []ast.Expr) []types.Type {
	if len(expressions) == 1 {
		if tuple, ok := engine.pass.TypesInfo.TypeOf(expressions[0]).(*types.Tuple); ok {
			result := make([]types.Type, tuple.Len())
			for index := range tuple.Len() {
				result[index] = tuple.At(index).Type()
			}
			return result
		}
	}
	result := make([]types.Type, 0, len(expressions))
	for _, expression := range expressions {
		result = append(result, engine.pass.TypesInfo.TypeOf(expression))
	}
	return result
}

func (engine *ps6101Engine) valueWithStoredFields(source ps6101Location, value ps6101Value, typ types.Type) ps6101Value {
	fields := make(map[string]ps6101Value, len(value.fieldValues()))
	for path, field := range value.fieldValues() {
		fields[path] = ps6101CloneValue(field)
	}
	remaining := ps6101ReferenceFieldSnapshotLimit
	engine.collectReferenceFields(source, source, typ, fields, make(map[types.Type]bool), &remaining)
	if len(fields) > 0 {
		value.setFields(fields)
	}
	return value
}

func (engine *ps6101Engine) collectReferenceFields(
	root, location ps6101Location,
	typ types.Type,
	fields map[string]ps6101Value,
	seen map[types.Type]bool,
	remaining *int,
) {
	if typ == nil || remaining == nil || *remaining <= 0 || !ps6101ContainsReference(typ) {
		return
	}
	typ = types.Unalias(typ)
	if seen[typ] {
		return
	}
	seen[typ] = true
	defer delete(seen, typ)
	remember := func(child ps6101Location) {
		stored, present := engine.state[child]
		if !present {
			return
		}
		suffix := strings.TrimPrefix(child.path, root.path)
		suffix = strings.TrimPrefix(suffix, ".")
		if suffix != "" {
			fields[suffix] = ps6101CloneValue(stored)
		}
	}
	switch current := typ.Underlying().(type) {
	case *types.Struct:
		for index := 0; index < current.NumFields(); index++ {
			if *remaining <= 0 {
				return
			}
			field := current.Field(index)
			if !ps6101ContainsReference(field.Type()) {
				continue
			}
			*remaining--
			child := ps6101AppendLocation(location, "."+ps6101FieldName(field))
			remember(child)
			engine.collectReferenceFields(root, child, field.Type(), fields, seen, remaining)
		}
	case *types.Array:
		if !ps6101ContainsReference(current.Elem()) {
			return
		}
		for index := int64(0); index < current.Len() && *remaining > 0; index++ {
			*remaining--
			child := ps6101AppendLocation(location, "["+strconv.FormatInt(index, 10)+"]")
			remember(child)
			engine.collectReferenceFields(root, child, current.Elem(), fields, seen, remaining)
		}
	}
}

func (engine *ps6101Engine) currentExitState() ps6101ExitState {
	return ps6101ExitState{
		state: engine.cloneState(), aliases: engine.cloneAliases(), gates: slices.Clone(engine.gates),
		counterGates: ps6101CopyCounterGates(engine.counterGates),
		counterRevs:  ps6101CopyCounterRevisions(engine.counterRevs),
		timer:        engine.timer, recoverable: engine.recoverable,
	}
}

func (engine *ps6101Engine) loadExitState(state ps6101ExitState) {
	engine.state = engine.copyState(state.state)
	engine.aliases = engine.copyAliases(state.aliases)
	engine.gates = slices.Clone(state.gates)
	engine.counterGates = ps6101CopyCounterGates(state.counterGates)
	engine.counterRevs = ps6101CopyCounterRevisions(state.counterRevs)
	engine.timer = state.timer
	engine.recoverable = state.recoverable
}

func (engine *ps6101Engine) mergeJumpStates(states []ps6101ExitState) {
	if len(states) == 0 {
		return
	}
	values := make([]map[ps6101Location]ps6101Value, 0, len(states))
	aliases := make([]map[types.Object]ps6101Location, 0, len(states))
	counterGates := make([]map[ps6101Location]ps6101CounterEvidence, 0, len(states))
	counterRevs := make([]map[ps6101Location]uint64, 0, len(states))
	engine.gates = nil
	engine.timer = 0
	engine.recoverable = 0
	for _, state := range states {
		values = append(values, state.state)
		aliases = append(aliases, state.aliases)
		counterGates = append(counterGates, state.counterGates)
		counterRevs = append(counterRevs, state.counterRevs)
		engine.gates = ps6101MergeGates(engine.gates, state.gates)
		engine.timer |= state.timer
		engine.recoverable = max(engine.recoverable, state.recoverable)
	}
	engine.mergeFallthrough(values, aliases)
	engine.mergeCounterFlow(counterGates, counterRevs)
}

func (engine *ps6101Engine) mergeMayExitStates(states []ps6101ExitState) {
	if len(states) == 0 {
		return
	}
	engine.state = engine.copyState(states[0].state)
	engine.aliases = engine.copyAliases(states[0].aliases)
	engine.gates = nil
	engine.timer = 0
	engine.recoverable = 0
	counterGates := make([]map[ps6101Location]ps6101CounterEvidence, 0, len(states))
	counterRevs := make([]map[ps6101Location]uint64, 0, len(states))
	for index, state := range states {
		if index > 0 {
			engine.state = ps6101MergeExitStates(engine.state, state.state)
			engine.aliases = ps6101MergeAliases(engine.aliases, state.aliases)
		}
		engine.gates = ps6101MergeGates(engine.gates, state.gates)
		engine.timer |= state.timer
		engine.recoverable = max(engine.recoverable, state.recoverable)
		counterGates = append(counterGates, state.counterGates)
		counterRevs = append(counterRevs, state.counterRevs)
	}
	engine.mergeCounterFlow(counterGates, counterRevs)
}

func (engine *ps6101Engine) analyzeIf(statement *ast.IfStmt) ps6101Flow {
	if statement.Init != nil {
		if flow := engine.analyzeStatement(statement.Init); flow&ps6101FallsThrough == 0 {
			return flow
		}
	}
	restoreEvaluation := engine.beginSingleEvaluation()
	condition := engine.boolConstant(statement.Cond)
	switch condition {
	case 1:
		restoreEvaluation()
		return engine.analyzeBlock(statement.Body)
	case -1:
		restoreEvaluation()
		if statement.Else != nil {
			return engine.analyzeStatement(statement.Else)
		}
		return ps6101FallsThrough
	}
	comparisons := engine.gateComparisons(statement.Cond)
	thenFails := engine.blockDirectlyFails(statement.Body)
	elseFails := engine.statementDirectlyFails(statement.Else)
	if thenFails {
		if engine.conditional == 0 {
			if comparison, ok := engine.singleComparison(statement.Cond); ok {
				comparison.op = ps6101NegatedComparison(comparison.op)
				engine.proofs[comparison] = true
			}
			for _, proof := range engine.counterProofs(statement.Cond) {
				engine.proofs[proof] = true
			}
		}
	} else {
		if engine.timer&ps6101TimerRunning != 0 {
			for _, comparison := range comparisons {
				engine.gates = append(engine.gates, ps6101Gate{key: comparison, pos: statement.Pos()})
			}
		}
	}
	if elseFails && engine.conditional == 0 {
		if comparison, ok := engine.singleComparison(statement.Cond); ok {
			engine.proofs[comparison] = true
		}
	}
	restoreEvaluation()
	entryState, entryAliases := engine.cloneState(), engine.cloneAliases()
	entryGates := slices.Clone(engine.gates)
	entryCounterGates, entryCounterRevs := ps6101CopyCounterGates(engine.counterGates), ps6101CopyCounterRevisions(engine.counterRevs)
	entryTimer, entryRecoverable := engine.timer, engine.recoverable
	engine.conditional++
	oldActive := len(engine.activeGates)
	if engine.loopAbstract > 0 {
		engine.refineAbstractCondition(statement.Cond, true)
	}
	if !thenFails && engine.timer&ps6101TimerRunning != 0 {
		for _, comparison := range comparisons {
			engine.activeGates = append(engine.activeGates, ps6101Gate{key: comparison, pos: statement.Cond.Pos()})
		}
	}
	bodyFlow := engine.analyzeBlock(statement.Body)
	engine.activeGates = engine.activeGates[:oldActive]
	bodyState, bodyAliases := engine.cloneState(), engine.cloneAliases()
	bodyGates, bodyTimer, bodyRecoverable := slices.Clone(engine.gates), engine.timer, engine.recoverable
	bodyCounterGates, bodyCounterRevs := ps6101CopyCounterGates(engine.counterGates), ps6101CopyCounterRevisions(engine.counterRevs)
	engine.state, engine.aliases = entryState, entryAliases
	engine.gates, engine.timer, engine.recoverable = slices.Clone(entryGates), entryTimer, entryRecoverable
	engine.counterGates, engine.counterRevs = ps6101CopyCounterGates(entryCounterGates), ps6101CopyCounterRevisions(entryCounterRevs)
	if engine.loopAbstract > 0 {
		engine.refineAbstractCondition(statement.Cond, false)
	}
	elseFlow := ps6101FallsThrough
	if statement.Else != nil {
		elseFlow = engine.analyzeStatement(statement.Else)
	}
	elseState, elseAliases := engine.cloneState(), engine.cloneAliases()
	elseGates, elseTimer, elseRecoverable := slices.Clone(engine.gates), engine.timer, engine.recoverable
	elseCounterGates, elseCounterRevs := ps6101CopyCounterGates(engine.counterGates), ps6101CopyCounterRevisions(engine.counterRevs)
	engine.conditional--
	var states []map[ps6101Location]ps6101Value
	var aliases []map[types.Object]ps6101Location
	var counterGates []map[ps6101Location]ps6101CounterEvidence
	var counterRevs []map[ps6101Location]uint64
	if bodyFlow&ps6101FallsThrough != 0 {
		states, aliases = append(states, bodyState), append(aliases, bodyAliases)
		counterGates, counterRevs = append(counterGates, bodyCounterGates), append(counterRevs, bodyCounterRevs)
	}
	if elseFlow&ps6101FallsThrough != 0 {
		states, aliases = append(states, elseState), append(aliases, elseAliases)
		counterGates, counterRevs = append(counterGates, elseCounterGates), append(counterRevs, elseCounterRevs)
	}
	engine.mergeFallthrough(states, aliases)
	engine.mergeCounterFlow(counterGates, counterRevs)
	engine.gates = ps6101MergeGates(bodyGates, elseGates)
	engine.timer = 0
	engine.recoverable = entryRecoverable
	if bodyFlow&ps6101FallsThrough != 0 {
		engine.timer |= bodyTimer
		engine.recoverable = max(engine.recoverable, bodyRecoverable)
	}
	if elseFlow&ps6101FallsThrough != 0 {
		engine.timer |= elseTimer
		engine.recoverable = max(engine.recoverable, elseRecoverable)
	}
	return bodyFlow | elseFlow
}

func (engine *ps6101Engine) beginSingleEvaluation() func() {
	oldValues, oldBools := engine.evalCache, engine.boolCache
	engine.evalCache = make(map[ast.Expr]ps6101Value)
	engine.boolCache = make(map[ast.Expr]int)
	return func() {
		engine.evalCache, engine.boolCache = oldValues, oldBools
	}
}

func ps6101MergeGates(left, right []ps6101Gate) []ps6101Gate {
	merged := make([]ps6101Gate, 0, len(left)+len(right))
	seen := make(map[ps6101Gate]bool, len(left)+len(right))
	for _, gates := range [][]ps6101Gate{left, right} {
		for _, gate := range gates {
			if !seen[gate] {
				seen[gate] = true
				merged = append(merged, gate)
			}
		}
	}
	return merged
}

func (engine *ps6101Engine) abstractIntegerConstant(expression ast.Expr) constant.Value {
	expression = ps6101Unparen(expression)
	if value := engine.pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Int {
		return value
	}
	if call, ok := expression.(*ast.CallExpr); ok && len(call.Args) == 1 && engine.pass.TypesInfo.Types[call.Fun].IsType() {
		return engine.abstractIntegerConstant(call.Args[0])
	}
	if location, ok := engine.location(expression, true); ok {
		stored := engine.state[location]
		if stored.constant != nil && stored.constant.Kind() == constant.Int {
			return stored.constant
		}
		analysis := stored.analysisValue()
		if analysis.lower != nil && analysis.lower.Kind() == constant.Int &&
			analysis.upper != nil && analysis.upper.Kind() == constant.Int &&
			constant.Compare(analysis.lower, token.EQL, analysis.upper) {
			return analysis.lower
		}
	}
	return nil
}

func (engine *ps6101Engine) refineAbstractCondition(expression ast.Expr, truth bool) {
	expression = ps6101Unparen(expression)
	if unary, ok := expression.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
		engine.refineAbstractCondition(unary.X, !truth)
		return
	}
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok {
		return
	}
	if binary.Op == token.LAND && truth || binary.Op == token.LOR && !truth {
		engine.refineAbstractCondition(binary.X, truth)
		engine.refineAbstractCondition(binary.Y, truth)
		return
	}
	left, leftOK := ps6101Unparen(binary.X).(*ast.Ident)
	constantExpression := binary.Y
	operation := binary.Op
	if !leftOK {
		left, leftOK = ps6101Unparen(binary.Y).(*ast.Ident)
		if !leftOK {
			return
		}
		constantExpression = binary.X
		operation = ps6101ReverseComparison(operation)
	}
	location, located := engine.location(left, true)
	value, present := engine.state[location]
	limit := engine.abstractIntegerConstant(constantExpression)
	if !located || !present || limit == nil {
		return
	}
	if !truth {
		operation = ps6101NegatedComparison(operation)
	}
	analysis := value.mutableAnalysis()
	one := constant.MakeInt64(1)
	switch operation {
	case token.EQL:
		value.constant = limit
		value.kind = ps6101Unknown
		analysis.lower, analysis.upper, analysis.excluded = limit, limit, nil
		ps6101ClassifyBounds(&value)
	case token.NEQ:
		if analysis.lower == nil || analysis.upper == nil {
			return
		}
		if constant.Compare(limit, token.EQL, analysis.lower) {
			analysis.lower = constant.BinaryOp(limit, token.ADD, one)
		} else if constant.Compare(limit, token.EQL, analysis.upper) {
			analysis.upper = constant.BinaryOp(limit, token.SUB, one)
		} else if constant.Compare(limit, token.GTR, analysis.lower) && constant.Compare(limit, token.LSS, analysis.upper) {
			analysis.excluded = maps.Clone(analysis.excluded)
			if analysis.excluded == nil {
				analysis.excluded = make(map[string]bool)
			}
			analysis.excluded[limit.ExactString()] = true
		}
	case token.LSS:
		upper := constant.BinaryOp(limit, token.SUB, one)
		if analysis.upper == nil || constant.Compare(upper, token.LSS, analysis.upper) {
			analysis.upper = upper
		}
	case token.LEQ:
		if analysis.upper == nil || constant.Compare(limit, token.LSS, analysis.upper) {
			analysis.upper = limit
		}
	case token.GTR:
		lower := constant.BinaryOp(limit, token.ADD, one)
		if analysis.lower == nil || constant.Compare(lower, token.GTR, analysis.lower) {
			analysis.lower = lower
		}
	case token.GEQ:
		if analysis.lower == nil || constant.Compare(limit, token.GTR, analysis.lower) {
			analysis.lower = limit
		}
	default:
		return
	}
	engine.state[location] = value
}

func (engine *ps6101Engine) snapshotDiagnostics() ps6101Diagnostics {
	return ps6101Diagnostics{
		gates: slices.Clone(engine.gates), subGates: slices.Clone(engine.subGates), proofs: maps.Clone(engine.proofs),
	}
}

func (engine *ps6101Engine) restoreDiagnostics(diagnostics ps6101Diagnostics) {
	engine.gates = diagnostics.gates
	engine.subGates = diagnostics.subGates
	engine.proofs = diagnostics.proofs
}

func ps6101RestoreExitDiagnostics(states []ps6101ExitState, diagnostics ps6101Diagnostics) {
	for index := range states {
		states[index].gates = slices.Clone(diagnostics.gates)
	}
}

func (engine *ps6101Engine) analyzeSwitch(statement *ast.SwitchStmt) ps6101Flow {
	if statement.Init != nil {
		if flow := engine.analyzeStatement(statement.Init); flow&ps6101FallsThrough == 0 {
			return flow
		}
	}
	var tagValue ps6101Value
	if statement.Tag != nil {
		tagValue = engine.eval(statement.Tag)
	} else {
		tagValue = ps6101ConstantValue(constant.MakeBool(true))
	}
	clauses := make([]*ast.CaseClause, 0, len(statement.Body.List))
	for _, raw := range statement.Body.List {
		if clause, ok := raw.(*ast.CaseClause); ok {
			clauses = append(clauses, clause)
		}
	}
	entry := engine.currentExitState()
	directEntries, directReachable, noMatchEntry, noMatchPossible := engine.evaluateSwitchCases(tagValue, statement.Tag == nil, clauses)
	var states []map[ps6101Location]ps6101Value
	var aliases []map[types.Object]ps6101Location
	var timers []ps6101TimerState
	var recoverables []int
	var counterGates []map[ps6101Location]ps6101CounterEvidence
	var counterRevs []map[ps6101Location]uint64
	allGates := slices.Clone(entry.gates)
	flow := ps6101Flow(0)
	incomingFallthrough := false
	var fallthroughEntry ps6101ExitState
	control := &ps6101ControlStates{}
	engine.breakTargets = append(engine.breakTargets, control)
	for clauseIndex, clause := range clauses {
		direct := directReachable[clauseIndex]
		reachable := direct || incomingFallthrough
		if !reachable {
			incomingFallthrough = false
			continue
		}
		if incomingFallthrough {
			engine.loadExitState(fallthroughEntry)
			if direct {
				engine.mergeJumpStates([]ps6101ExitState{directEntries[clauseIndex], engine.currentExitState()})
			}
		} else {
			engine.loadExitState(directEntries[clauseIndex])
		}
		engine.conditional++
		clauseGates := []ps6101GateKey(nil)
		if direct && !incomingFallthrough && !ps6101ClauseFallsThrough(clause) && engine.timer&ps6101TimerRunning != 0 {
			clauseGates = engine.switchClauseGates(statement.Tag, clause, clauses)
			for _, comparison := range clauseGates {
				engine.gates = append(engine.gates, ps6101Gate{key: comparison, pos: clause.Pos()})
			}
		}
		oldActive := len(engine.activeGates)
		for _, gate := range clauseGates {
			engine.activeGates = append(engine.activeGates, ps6101Gate{key: gate, pos: clause.Pos()})
		}
		clauseFlow := ps6101FallsThrough
		for _, child := range clause.Body {
			if clauseFlow&ps6101FallsThrough == 0 {
				break
			}
			childFlow := engine.analyzeStatement(child)
			clauseFlow = clauseFlow&^ps6101FallsThrough | childFlow
		}
		engine.activeGates = engine.activeGates[:oldActive]
		engine.conditional--
		allGates = ps6101MergeGates(allGates, engine.gates)
		if clauseFlow&ps6101Breaks != 0 {
			clauseFlow &^= ps6101Breaks
		}
		incomingFallthrough = ps6101ClauseFallsThrough(clause) && clauseFlow&ps6101FallsThrough != 0
		if incomingFallthrough {
			fallthroughEntry = engine.currentExitState()
			clauseFlow &^= ps6101FallsThrough
		}
		flow |= clauseFlow
		if !incomingFallthrough && clauseFlow&ps6101FallsThrough != 0 {
			states = append(states, engine.cloneState())
			aliases = append(aliases, engine.cloneAliases())
			timers, recoverables = append(timers, engine.timer), append(recoverables, engine.recoverable)
			counterGates = append(counterGates, ps6101CopyCounterGates(engine.counterGates))
			counterRevs = append(counterRevs, ps6101CopyCounterRevisions(engine.counterRevs))
		}
	}
	engine.breakTargets = engine.breakTargets[:len(engine.breakTargets)-1]
	for _, exit := range control.breaks {
		flow |= ps6101FallsThrough
		states, aliases = append(states, exit.state), append(aliases, exit.aliases)
		timers, recoverables = append(timers, exit.timer), append(recoverables, exit.recoverable)
		counterGates, counterRevs = append(counterGates, exit.counterGates), append(counterRevs, exit.counterRevs)
		allGates = ps6101MergeGates(allGates, exit.gates)
	}
	if noMatchPossible {
		flow |= ps6101FallsThrough
		states, aliases = append(states, noMatchEntry.state), append(aliases, noMatchEntry.aliases)
		timers, recoverables = append(timers, noMatchEntry.timer), append(recoverables, noMatchEntry.recoverable)
		counterGates, counterRevs = append(counterGates, noMatchEntry.counterGates), append(counterRevs, noMatchEntry.counterRevs)
		allGates = ps6101MergeGates(allGates, noMatchEntry.gates)
	}
	engine.mergeFallthrough(states, aliases)
	engine.mergeCounterFlow(counterGates, counterRevs)
	engine.gates = allGates
	engine.timer = 0
	engine.recoverable = entry.recoverable
	for index, timer := range timers {
		engine.timer |= timer
		engine.recoverable = max(engine.recoverable, recoverables[index])
	}
	return flow
}

func (engine *ps6101Engine) evaluateSwitchCases(
	tag ps6101Value,
	expressionless bool,
	clauses []*ast.CaseClause,
) ([]ps6101ExitState, []bool, ps6101ExitState, bool) {
	entries := make([]ps6101ExitState, len(clauses))
	reachable := make([]bool, len(clauses))
	candidates := make([][]ps6101ExitState, len(clauses))
	defaultIndex := -1
	unmatchedEntry := engine.currentExitState()
	unmatched := true
	for index, clause := range clauses {
		if len(clause.List) == 0 {
			defaultIndex = index
			continue
		}
		if !unmatched {
			continue
		}
		for _, expression := range clause.List {
			engine.loadExitState(unmatchedEntry)
			var matching []ps6101ExitState
			var nonmatching []ps6101ExitState
			if expressionless || ps6101BooleanExpression(engine.pass, expression) {
				truths, falses := engine.evaluateBooleanOutcomes(expression)
				for _, outcome := range []struct {
					value  bool
					states []ps6101ExitState
				}{
					{value: true, states: truths},
					{value: false, states: falses},
				} {
					match := ps6101SwitchValueMatch(tag, ps6101ConstantValue(constant.MakeBool(outcome.value)))
					if match >= 0 {
						matching = append(matching, outcome.states...)
					}
					if match <= 0 {
						nonmatching = append(nonmatching, outcome.states...)
					}
				}
			} else {
				candidate := engine.eval(expression)
				state := engine.currentExitState()
				match := ps6101SwitchValueMatch(tag, candidate)
				if match >= 0 {
					matching = append(matching, state)
				}
				if match <= 0 {
					nonmatching = append(nonmatching, state)
				}
			}
			if matched, ok := engine.mergeBooleanOutcomeStates(matching); ok {
				candidates[index] = append(candidates[index], matched)
				reachable[index] = true
			}
			var ok bool
			unmatchedEntry, ok = engine.mergeBooleanOutcomeStates(nonmatching)
			if !ok {
				unmatched = false
				break
			}
		}
	}
	noMatchEntry := unmatchedEntry
	noMatchPossible := unmatched
	if defaultIndex >= 0 && unmatched {
		candidates[defaultIndex] = append(candidates[defaultIndex], noMatchEntry)
		reachable[defaultIndex] = true
		noMatchPossible = false
	}

	evaluationEnd := unmatchedEntry
	if !unmatched {
		for index := len(candidates) - 1; index >= 0; index-- {
			if len(candidates[index]) > 0 {
				evaluationEnd = candidates[index][len(candidates[index])-1]
				break
			}
		}
	}
	for index, states := range candidates {
		if len(states) == 0 {
			continue
		}
		engine.mergeJumpStates(states)
		entries[index] = engine.currentExitState()
	}
	engine.loadExitState(evaluationEnd)
	return entries, reachable, noMatchEntry, noMatchPossible
}

// evaluateBooleanOutcomes evaluates a logical expression once per runtime
// path and keeps the states in which it is true separate from those in which
// it is false. In particular, a true A && B state necessarily includes B's
// effects, while an A-false short-circuit state can only flow to no-match.
func (engine *ps6101Engine) evaluateBooleanOutcomes(expression ast.Expr) ([]ps6101ExitState, []ps6101ExitState) {
	expression = ps6101Unparen(expression)
	if unary, ok := expression.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
		truths, falses := engine.evaluateBooleanOutcomes(unary.X)
		return falses, truths
	}
	if binary, ok := expression.(*ast.BinaryExpr); ok && (binary.Op == token.LAND || binary.Op == token.LOR) {
		leftTruths, leftFalses := engine.evaluateBooleanOutcomes(binary.X)
		if binary.Op == token.LAND {
			falses := slices.Clone(leftFalses)
			if leftTrue, ok := engine.mergeBooleanOutcomeStates(leftTruths); ok {
				engine.loadExitState(leftTrue)
				rightTruths, rightFalses := engine.evaluateBooleanOutcomes(binary.Y)
				falses = append(falses, rightFalses...)
				return engine.mergeBooleanOutcomes(rightTruths, falses)
			}
			return engine.mergeBooleanOutcomes(nil, falses)
		}

		truths := slices.Clone(leftTruths)
		if leftFalse, ok := engine.mergeBooleanOutcomeStates(leftFalses); ok {
			engine.loadExitState(leftFalse)
			rightTruths, rightFalses := engine.evaluateBooleanOutcomes(binary.Y)
			truths = append(truths, rightTruths...)
			return engine.mergeBooleanOutcomes(truths, rightFalses)
		}
		return engine.mergeBooleanOutcomes(truths, nil)
	}

	// boolConstant classifies comparisons and local calls, while eval ensures
	// that selector bases and other boolean atoms whose value stays unknown are
	// still evaluated. A shared cache makes this one runtime evaluation.
	restoreEvaluation := engine.beginSingleEvaluation()
	result := engine.boolConstant(expression)
	evaluated := engine.eval(expression)
	restoreEvaluation()
	if result == 0 && evaluated.constant != nil && evaluated.constant.Kind() == constant.Bool {
		if constant.BoolVal(evaluated.constant) {
			result = 1
		} else {
			result = -1
		}
	}
	state := engine.currentExitState()
	var truths, falses []ps6101ExitState
	if result >= 0 {
		truths = append(truths, state)
	}
	if result <= 0 {
		falses = append(falses, state)
	}
	return truths, falses
}

func (engine *ps6101Engine) mergeBooleanOutcomeStates(states []ps6101ExitState) (ps6101ExitState, bool) {
	if len(states) == 0 {
		return ps6101ExitState{}, false
	}
	engine.mergeMayExitStates(states)
	return engine.currentExitState(), true
}

// mergeBooleanOutcomes keeps recursive logical evaluation linear in the
// expression size. Once paths have the same truth value, later short-circuit
// decisions cannot distinguish them, so a single conservative joined state is
// sufficient for each polarity.
func (engine *ps6101Engine) mergeBooleanOutcomes(
	truths, falses []ps6101ExitState,
) ([]ps6101ExitState, []ps6101ExitState) {
	var mergedTruths, mergedFalses []ps6101ExitState
	if truth, ok := engine.mergeBooleanOutcomeStates(truths); ok {
		mergedTruths = append(mergedTruths, truth)
	}
	if falsity, ok := engine.mergeBooleanOutcomeStates(falses); ok {
		mergedFalses = append(mergedFalses, falsity)
	}
	return mergedTruths, mergedFalses
}

func ps6101BooleanExpression(pass *analysis.Pass, expression ast.Expr) bool {
	if pass == nil || pass.TypesInfo == nil || expression == nil {
		return false
	}
	typ := pass.TypesInfo.TypeOf(expression)
	if typ == nil {
		return false
	}
	basic, ok := types.Unalias(typ).Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsBoolean != 0
}

func ps6101SwitchValueMatch(tag, candidate ps6101Value) int {
	if tag.constant != nil && candidate.constant != nil {
		if constant.Compare(tag.constant, token.EQL, candidate.constant) {
			return 1
		}
		return -1
	}
	return ps6101CompareBounds(tag, token.EQL, candidate)
}

func (engine *ps6101Engine) analyzeTypeSwitch(statement *ast.TypeSwitchStmt) ps6101Flow {
	if statement == nil || statement.Body == nil {
		return ps6101FallsThrough
	}
	if statement.Init != nil {
		if flow := engine.analyzeStatement(statement.Init); flow&ps6101FallsThrough == 0 {
			return flow
		}
	}
	assertion := ps6101TypeSwitchAssertion(statement.Assign)
	if assertion == nil {
		engine.invalidateCapturedState()
		return ps6101FallsThrough
	}
	source := engine.eval(assertion.X)
	clauses := make([]*ast.CaseClause, 0, len(statement.Body.List))
	for _, raw := range statement.Body.List {
		if clause, ok := raw.(*ast.CaseClause); ok {
			clauses = append(clauses, clause)
		}
	}
	reachable, noMatchPossible := engine.typeSwitchReachability(source.analysisValue().dynamic, clauses)
	entry := engine.currentExitState()
	allGates := slices.Clone(entry.gates)
	control := &ps6101ControlStates{}
	engine.breakTargets = append(engine.breakTargets, control)
	flow := ps6101Flow(0)
	var exits []ps6101ExitState
	for index, clause := range clauses {
		if !reachable[index] {
			continue
		}
		engine.state, engine.aliases = engine.copyState(entry.state), engine.copyAliases(entry.aliases)
		engine.gates = slices.Clone(entry.gates)
		engine.counterGates = ps6101CopyCounterGates(entry.counterGates)
		engine.counterRevs = ps6101CopyCounterRevisions(entry.counterRevs)
		engine.timer, engine.recoverable = entry.timer, entry.recoverable
		engine.bindTypeSwitchVariable(clause, assertion, source)
		engine.conditional++
		clauseFlow := ps6101FallsThrough
		for _, child := range clause.Body {
			if clauseFlow&ps6101FallsThrough == 0 {
				break
			}
			childFlow := engine.analyzeStatement(child)
			clauseFlow = clauseFlow&^ps6101FallsThrough | childFlow
		}
		engine.conditional--
		allGates = ps6101MergeGates(allGates, engine.gates)
		flow |= clauseFlow &^ ps6101Breaks
		if clauseFlow&ps6101FallsThrough != 0 {
			exits = append(exits, engine.currentExitState())
		}
	}
	engine.breakTargets = engine.breakTargets[:len(engine.breakTargets)-1]
	for _, exit := range control.breaks {
		flow |= ps6101FallsThrough
		exits = append(exits, exit)
		allGates = ps6101MergeGates(allGates, exit.gates)
	}
	if noMatchPossible {
		flow |= ps6101FallsThrough
		exits = append(exits, entry)
	}
	engine.mergeJumpStates(exits)
	engine.gates = allGates
	return flow
}

func ps6101TypeSwitchAssertion(statement ast.Stmt) *ast.TypeAssertExpr {
	switch value := statement.(type) {
	case *ast.ExprStmt:
		assertion, _ := ps6101Unparen(value.X).(*ast.TypeAssertExpr)
		return assertion
	case *ast.AssignStmt:
		if len(value.Rhs) == 1 {
			assertion, _ := ps6101Unparen(value.Rhs[0]).(*ast.TypeAssertExpr)
			return assertion
		}
	}
	return nil
}

func (engine *ps6101Engine) typeSwitchReachability(dynamic types.Type, clauses []*ast.CaseClause) ([]bool, bool) {
	reachable := make([]bool, len(clauses))
	if dynamic == nil {
		hasDefault := false
		for index, clause := range clauses {
			reachable[index] = true
			hasDefault = hasDefault || len(clause.List) == 0
		}
		return reachable, !hasDefault
	}
	defaultIndex := -1
	for index, clause := range clauses {
		if len(clause.List) == 0 {
			defaultIndex = index
			continue
		}
		for _, candidate := range clause.List {
			if engine.typeSwitchCaseMatches(dynamic, candidate) {
				reachable[index] = true
				return reachable, false
			}
		}
	}
	if defaultIndex >= 0 {
		reachable[defaultIndex] = true
		return reachable, false
	}
	return reachable, true
}

func (engine *ps6101Engine) typeSwitchCaseMatches(dynamic types.Type, expression ast.Expr) bool {
	if identifier, ok := ps6101Unparen(expression).(*ast.Ident); ok && identObject(engine.pass, identifier) == types.Universe.Lookup("nil") {
		return false
	}
	target := engine.pass.TypesInfo.TypeOf(expression)
	if dynamic == nil || target == nil {
		return false
	}
	if ps6101InterfaceType(target) {
		return types.AssignableTo(dynamic, target)
	}
	return types.Identical(types.Unalias(dynamic), types.Unalias(target))
}

func (engine *ps6101Engine) bindTypeSwitchVariable(clause *ast.CaseClause, assertion *ast.TypeAssertExpr, source ps6101Value) {
	object := engine.pass.TypesInfo.Implicits[clause]
	if object == nil {
		return
	}
	destination := ps6101Location{root: object}
	engine.killPrefix(destination)
	delete(engine.aliases, object)
	value := ps6101CloneValue(source)
	if !ps6101InterfaceType(object.Type()) {
		value.mutableAnalysis().dynamic = nil
	}
	ps6101ApplyNamedValueSemantics(object.Name(), &value)
	engine.state[destination] = value
	engine.storeValueFields(destination, value.fieldValues())
	if sourceLocation, ok := engine.location(assertion.X, true); ok {
		engine.copyPrefix(sourceLocation, destination)
	}
	if ps6101ReferenceLike(object.Type()) && source.reference != nil {
		engine.aliases[object] = *source.reference
	}
}

func (engine *ps6101Engine) switchReachability(tag ast.Expr, tagValue ps6101Value, clauses []*ast.CaseClause) ([]bool, bool) {
	reachable := make([]bool, len(clauses))
	if tag == nil {
		definiteMatch := false
		defaultIndex := -1
		for index, clause := range clauses {
			if len(clause.List) == 0 {
				defaultIndex = index
				continue
			}
			if definiteMatch {
				continue
			}
			for _, expression := range clause.List {
				switch engine.abstractLoopBool(expression) {
				case 1:
					reachable[index] = true
					definiteMatch = true
				case 0:
					reachable[index] = true
				}
				if definiteMatch {
					break
				}
			}
		}
		if defaultIndex >= 0 && !definiteMatch {
			reachable[defaultIndex] = true
		}
		return reachable, !definiteMatch
	}
	if !engine.switchTagStable(tag) {
		for index := range reachable {
			reachable[index] = true
		}
		return reachable, true
	}
	if typed := engine.pass.TypesInfo.Types[tag].Value; typed != nil {
		tagValue = ps6101ConstantValue(typed)
	}
	tagBounds := tagValue.analysisValue()
	if tagValue.constant == nil && (tagBounds.lower == nil || tagBounds.upper == nil) {
		for index := range reachable {
			reachable[index] = true
		}
		return reachable, true
	}
	definiteMatch := false
	defaultIndex := -1
	for index, clause := range clauses {
		if len(clause.List) == 0 {
			defaultIndex = index
			continue
		}
		if definiteMatch {
			continue
		}
		unknown := false
		for _, expression := range clause.List {
			caseConstant := engine.expressionConstant(expression)
			if caseConstant == nil {
				unknown = true
				continue
			}
			if tagValue.constant != nil {
				if constant.Compare(tagValue.constant, token.EQL, caseConstant) {
					reachable[index] = true
					definiteMatch = true
					break
				}
				continue
			}
			match := ps6101CompareBounds(tagValue, token.EQL, ps6101ConstantValue(caseConstant))
			if match > 0 {
				reachable[index] = true
				definiteMatch = true
				break
			}
			if match == 0 {
				unknown = true
			}
		}
		if unknown && !definiteMatch {
			reachable[index] = true
		}
	}
	if defaultIndex >= 0 && !definiteMatch {
		reachable[defaultIndex] = true
	}
	return reachable, !definiteMatch
}

func (engine *ps6101Engine) switchClauseGates(tag ast.Expr, clause *ast.CaseClause, clauses []*ast.CaseClause) []ps6101GateKey {
	if clause == nil || !engine.switchTagStable(tag) {
		return nil
	}
	for _, expression := range clause.List {
		if !engine.switchTagStable(expression) {
			// Case operands have already been evaluated in source order. Gate
			// classification must not execute an effectful operand again.
			return nil
		}
	}
	if tag == nil {
		alternatives := make([][]ps6101GateKey, 0, len(clause.List))
		for _, expression := range clause.List {
			alternatives = append(alternatives, engine.gateComparisons(expression))
		}
		return ps6101IntersectGateAlternatives(alternatives)
	}
	gateTag := engine.switchGateExpression(tag)
	normal := engine.gateComparisons(gateTag)
	negated := engine.gateComparisonsWithPolarity(gateTag, true)
	if len(clause.List) == 0 {
		return engine.switchDefaultGates(gateTag, normal, negated, clauses)
	}
	boolPolarity := 0
	allBoolean := len(clause.List) > 0
	for _, expression := range clause.List {
		value := engine.expressionConstant(expression)
		if value == nil || value.Kind() != constant.Bool {
			allBoolean = false
			break
		}
		polarity := -1
		if constant.BoolVal(value) {
			polarity = 1
		}
		if boolPolarity != 0 && boolPolarity != polarity {
			return nil
		}
		boolPolarity = polarity
	}
	if allBoolean && len(normal)+len(negated) > 0 {
		if boolPolarity > 0 {
			return normal
		}
		return negated
	}
	alternatives := make([][]ps6101GateKey, 0, len(clause.List))
	for _, expression := range clause.List {
		if engine.expressionConstant(expression) == nil {
			return nil
		}
		comparison, ok := engine.comparisonSides(tag, expression, token.EQL)
		if !ok {
			comparison, ok = engine.comparisonSides(expression, tag, token.EQL)
		}
		if !ok {
			return nil
		}
		alternatives = append(alternatives, []ps6101GateKey{comparison})
	}
	return ps6101IntersectGateAlternatives(alternatives)
}

func (engine *ps6101Engine) switchDefaultGates(tag ast.Expr, normal, negated []ps6101GateKey, clauses []*ast.CaseClause) []ps6101GateKey {
	coveredTrue, coveredFalse := false, false
	var comparisons []ps6101GateKey
	for _, candidate := range clauses {
		if candidate == nil || len(candidate.List) == 0 {
			continue
		}
		for _, expression := range candidate.List {
			value := engine.expressionConstant(expression)
			if value == nil {
				return nil
			}
			if value.Kind() == constant.Bool {
				if constant.BoolVal(value) {
					coveredTrue = true
				} else {
					coveredFalse = true
				}
				continue
			}
			comparison, ok := engine.comparisonSides(tag, expression, token.NEQ)
			if !ok {
				comparison, ok = engine.comparisonSides(expression, tag, token.NEQ)
			}
			if ok {
				comparisons = append(comparisons, comparison)
			}
		}
	}
	if coveredTrue || coveredFalse {
		if coveredTrue == coveredFalse {
			return nil
		}
		if coveredTrue {
			return negated
		}
		return normal
	}
	return comparisons
}

func (engine *ps6101Engine) expressionConstant(expression ast.Expr) constant.Value {
	if expression == nil {
		return nil
	}
	if value := engine.pass.TypesInfo.Types[expression].Value; value != nil {
		return value
	}
	if location, ok := engine.location(expression, true); ok {
		return engine.state[location].constant
	}
	return nil
}

func ps6101IntersectGateAlternatives(alternatives [][]ps6101GateKey) []ps6101GateKey {
	if len(alternatives) == 0 || len(alternatives[0]) == 0 {
		return nil
	}
	counts := make(map[ps6101GateKey]int)
	for _, alternative := range alternatives {
		if len(alternative) == 0 {
			return nil
		}
		seen := make(map[ps6101GateKey]bool)
		for _, gate := range alternative {
			if !seen[gate] {
				seen[gate] = true
				counts[gate]++
			}
		}
	}
	var result []ps6101GateKey
	for _, gate := range alternatives[0] {
		if counts[gate] == len(alternatives) {
			result = append(result, gate)
		}
	}
	return result
}

func (engine *ps6101Engine) switchTagStable(expression ast.Expr) bool {
	if expression == nil {
		return true
	}
	stable := true
	ast.Inspect(expression, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.CallExpr:
			// A one-argument type conversion evaluates only its operand and is
			// stable for switch-gate purposes.
			if len(value.Args) == 1 && engine.pass.TypesInfo.Types[value.Fun].IsType() {
				return true
			}
			stable = false
			return false
		case *ast.FuncLit:
			stable = false
			return false
		case *ast.UnaryExpr:
			if value.Op == token.ARROW {
				stable = false
				return false
			}
		}
		return stable
	})
	return stable
}

func (engine *ps6101Engine) switchGateExpression(expression ast.Expr) ast.Expr {
	for {
		call, ok := ps6101Unparen(expression).(*ast.CallExpr)
		if !ok || len(call.Args) != 1 || !engine.pass.TypesInfo.Types[call.Fun].IsType() {
			return expression
		}
		expression = call.Args[0]
	}
}

func ps6101ClauseFallsThrough(clause *ast.CaseClause) bool {
	if clause == nil || len(clause.Body) == 0 {
		return false
	}
	branch, ok := clause.Body[len(clause.Body)-1].(*ast.BranchStmt)
	return ok && branch.Tok == token.FALLTHROUGH
}

func (engine *ps6101Engine) analyzeFor(statement *ast.ForStmt) ps6101Flow {
	return engine.analyzeForLabeled(statement, nil)
}

func (engine *ps6101Engine) analyzeForLabeled(statement *ast.ForStmt, label types.Object) ps6101Flow {
	if statement.Init != nil {
		if flow := engine.analyzeStatement(statement.Init); flow&ps6101FallsThrough == 0 {
			return flow
		}
	}
	bLoop := false
	parallelLoop := false
	var exitStates []map[ps6101Location]ps6101Value
	var exitAliases []map[types.Object]ps6101Location
	var exitGates [][]ps6101Gate
	var exitTimers []ps6101TimerState
	var exitRecoverables []int
	var exitCounterGates []map[ps6101Location]ps6101CounterEvidence
	var exitCounterRevs []map[ps6101Location]uint64
	appendState := func(exit ps6101ExitState) {
		exitStates = append(exitStates, exit.state)
		exitAliases = append(exitAliases, exit.aliases)
		exitGates = append(exitGates, exit.gates)
		exitTimers = append(exitTimers, exit.timer)
		exitRecoverables = append(exitRecoverables, exit.recoverable)
		exitCounterGates = append(exitCounterGates, exit.counterGates)
		exitCounterRevs = append(exitCounterRevs, exit.counterRevs)
	}
	appendExit := func() { appendState(engine.currentExitState()) }
	appendConditionExit := func() {
		exit := engine.currentExitState()
		if bLoop {
			// Loop stops the timer only when the call itself returns false.
			// Explicit break paths never execute that call and retain the timer
			// state captured by appendExit above.
			exit.timer = ps6101TimerStopped
		}
		appendState(exit)
	}
	flow := ps6101Flow(0)
	condition := 1
	for iteration := 0; ; iteration++ {
		if iteration == 0 {
			// Resolving a returned function or method value is part of evaluating
			// the condition's function operand. Share one evaluation cache with
			// the call below so helpers, receivers, and method-expression
			// arguments run once in Go order.
			restoreEvaluation := engine.beginSingleEvaluation()
			bLoop = engine.isBLoopCondition(statement.Cond)
			parallelLoop = engine.isParallelLoopCondition(statement.Cond)
			if bLoop {
				// Receiver and method-expression arguments are evaluated before
				// B.Loop performs its first-call timer reset.
				engine.boolConstant(statement.Cond)
				engine.gates = nil
				engine.timer = ps6101TimerRunning
			} else {
				condition = engine.boolConstant(statement.Cond)
			}
			restoreEvaluation()
			if condition < 0 {
				appendConditionExit()
				break
			}
			if condition == 0 {
				appendConditionExit()
			}
		}
		if !engine.takeExactLoopTransfer() {
			abstractFlow, abstractExits := engine.analyzeForRemainder(statement, label, condition, bLoop, parallelLoop)
			flow |= abstractFlow
			for _, exit := range abstractExits {
				appendState(exit)
			}
			break
		}
		beforeState := engine.cloneState()
		if condition == 0 {
			engine.conditional++
		}
		control := &ps6101ControlStates{}
		engine.breakTargets = append(engine.breakTargets, control)
		engine.continueTargets = append(engine.continueTargets, control)
		bodyFlow := engine.analyzeBlock(statement.Body)
		engine.breakTargets = engine.breakTargets[:len(engine.breakTargets)-1]
		engine.continueTargets = engine.continueTargets[:len(engine.continueTargets)-1]
		if condition == 0 {
			engine.conditional--
		}
		flow |= bodyFlow & (ps6101Returns | ps6101Gotos)
		for _, exit := range control.breaks {
			appendState(exit)
		}
		continuations := slices.Clone(control.continues)
		if bodyFlow&ps6101FallsThrough != 0 {
			continuations = append(continuations, engine.currentExitState())
		}
		if label != nil {
			if control := engine.controls[label]; control != nil {
				for _, exit := range control.breaks {
					appendState(exit)
				}
				continuations = append(continuations, control.continues...)
				delete(engine.controls, label)
			}
		}
		if len(continuations) == 0 {
			break
		}
		engine.mergeJumpStates(continuations)
		if statement.Post != nil {
			engine.analyzeStatement(statement.Post)
		}
		nextCondition := 1
		if bLoop {
			// Every later condition executes its ordinary call operands while
			// the benchmark timer is running.
			engine.boolConstant(statement.Cond)
		} else {
			nextCondition = engine.boolConstant(statement.Cond)
			if nextCondition < 0 {
				appendExit()
				break
			}
		}
		if (bLoop || parallelLoop) && iteration > 0 && ps6101EquivalentStates(beforeState, engine.state) {
			appendConditionExit()
			break
		}
		if condition == 0 {
			appendExit()
			break
		}
		condition = nextCondition
	}
	if len(exitStates) > 0 {
		engine.mergeFallthrough(exitStates, exitAliases)
		engine.mergeCounterFlow(exitCounterGates, exitCounterRevs)
		engine.gates = nil
		engine.timer = 0
		engine.recoverable = 0
		for index, gates := range exitGates {
			engine.gates = ps6101MergeGates(engine.gates, gates)
			engine.timer |= exitTimers[index]
			engine.recoverable = max(engine.recoverable, exitRecoverables[index])
		}
		flow |= ps6101FallsThrough
	}
	return flow
}

// analyzeForRemainder preserves the chronological first iteration after budget
// exhaustion, then visits the remaining tail once with a bounded induction
// value. The shared transfer budget bounds a product of nested for and range
// trip counts rather than bounding each loop independently.
func (engine *ps6101Engine) analyzeForRemainder(
	statement *ast.ForStmt,
	label types.Object,
	condition int,
	bLoop bool,
	parallelLoop bool,
) (ps6101Flow, []ps6101ExitState) {
	flow, exits, continuations, _ := engine.analyzeForTransfer(statement, label, condition)
	if len(continuations) == 0 {
		return flow, exits
	}
	engine.mergeJumpStates(continuations)
	if statement.Post != nil {
		engine.analyzeStatement(statement.Post)
	}
	nextCondition := 1
	if bLoop {
		engine.boolConstant(statement.Cond)
	} else {
		nextCondition = engine.boolConstant(statement.Cond)
		if nextCondition < 0 {
			return flow, append(exits, engine.currentExitState())
		}
		if nextCondition == 0 {
			exits = append(exits, engine.currentExitState())
		}
	}

	inductionLocation, inductionValue, direction, bounded := engine.forRemainderInduction(statement)
	engine.invalidateFutureLoopEffects(&ast.ForStmt{Body: &ast.BlockStmt{}, Post: statement.Post})
	if bounded {
		engine.state[inductionLocation] = inductionValue
		tailFlow, tailExits := engine.analyzeBoundedForTail(statement, label, inductionLocation, inductionValue, direction)
		return flow | tailFlow, append(exits, tailExits...)
	}
	diagnostics := engine.snapshotDiagnostics()
	tailFlow, tailExits, tailContinuations, control := engine.analyzeForTransfer(statement, label, nextCondition)
	flow |= tailFlow
	allowTailDiagnostics := bounded || bLoop || parallelLoop || ps6101StaticBool(engine.pass, statement.Cond) == 1
	if control || !allowTailDiagnostics {
		// An exit at an unknown chronological index may prevent every later
		// iteration. Preserve the state summary but not speculative gates or
		// proofs from that tail.
		engine.restoreDiagnostics(diagnostics)
		ps6101RestoreExitDiagnostics(tailContinuations, diagnostics)
		if !allowTailDiagnostics {
			ps6101RestoreExitDiagnostics(tailExits, diagnostics)
		}
	}
	exits = append(exits, tailExits...)
	if len(tailContinuations) == 0 {
		return flow, exits
	}
	engine.mergeJumpStates(tailContinuations)
	if statement.Post != nil {
		engine.analyzeStatement(statement.Post)
	}
	if bLoop {
		engine.boolConstant(statement.Cond)
	}
	engine.invalidateFutureLoopEffects(statement)
	if bLoop || parallelLoop || statement.Cond != nil && ps6101StaticBool(engine.pass, statement.Cond) != 1 {
		exit := engine.currentExitState()
		if bLoop {
			exit.timer = ps6101TimerStopped
		}
		exits = append(exits, exit)
	}
	return flow, exits
}

func (engine *ps6101Engine) analyzeForTransfer(statement *ast.ForStmt, label types.Object, condition int) (ps6101Flow, []ps6101ExitState, []ps6101ExitState, bool) {
	if condition == 0 {
		engine.conditional++
	}
	engine.loopAbstract++
	control := &ps6101ControlStates{}
	engine.breakTargets = append(engine.breakTargets, control)
	engine.continueTargets = append(engine.continueTargets, control)
	bodyFlow := engine.analyzeBlock(statement.Body)
	engine.breakTargets = engine.breakTargets[:len(engine.breakTargets)-1]
	engine.continueTargets = engine.continueTargets[:len(engine.continueTargets)-1]
	engine.loopAbstract--
	if condition == 0 {
		engine.conditional--
	}

	exits := slices.Clone(control.breaks)
	continuations := slices.Clone(control.continues)
	if bodyFlow&ps6101FallsThrough != 0 {
		continuations = append(continuations, engine.currentExitState())
	}
	hasControl := len(control.breaks) > 0 || bodyFlow&(ps6101Returns|ps6101Gotos|ps6101Breaks) != 0
	if label != nil {
		if labeled := engine.controls[label]; labeled != nil {
			hasControl = hasControl || len(labeled.breaks) > 0
			exits = append(exits, labeled.breaks...)
			continuations = append(continuations, labeled.continues...)
			delete(engine.controls, label)
		}
	}
	return bodyFlow & (ps6101Returns | ps6101Gotos), exits, continuations, hasControl
}

// analyzeBoundedForTail walks comparison boundaries in execution order. An
// exact boundary transfer prevents a break, return, or goto at an earlier
// induction value from being merged with a gate that is only reachable at a
// later value. Intervals between boundaries still need just one conservative
// transfer, so large and nested loop products remain bounded.
func (engine *ps6101Engine) analyzeBoundedForTail(
	statement *ast.ForStmt,
	label types.Object,
	induction ps6101Location,
	bounds ps6101Value,
	direction int,
) (ps6101Flow, []ps6101ExitState) {
	segments := engine.loopSegments(statement.Body, induction, bounds, direction)
	if len(segments) == 0 {
		return engine.analyzeBoundedForFallback(statement, label, induction, bounds)
	}
	flow := ps6101Flow(0)
	var exits []ps6101ExitState
	for _, segment := range segments {
		if !engine.takeAbstractLoopTransfer() {
			remaining := ps6101RemainingLoopBounds(bounds, segment, direction)
			fallbackFlow, fallbackExits := engine.analyzeBoundedForFallback(statement, label, induction, remaining)
			return flow | fallbackFlow, append(exits, fallbackExits...)
		}
		exact := segment.lower == segment.upper
		engine.state[induction] = ps6101LoopIndexValue(segment.lower, segment.upper)
		segmentInduction := ps6101CloneValue(engine.state[induction])
		bodyPreservesInduction := exact || engine.loopSegmentPreservesLocation(statement.Body, induction)
		diagnostics := engine.snapshotDiagnostics()
		segmentFlow, segmentExits, continuations, control := engine.analyzeForTransfer(statement, label, 1)
		flow |= segmentFlow
		if !exact && control {
			// A control transfer at an unknown index can suppress a later
			// iteration in this interval. Its concrete exit keeps diagnostics
			// reached before the transfer; speculative continuation diagnostics
			// do not escape the interval.
			engine.restoreDiagnostics(diagnostics)
			ps6101RestoreExitDiagnostics(continuations, diagnostics)
		}
		exits = append(exits, segmentExits...)
		if len(continuations) == 0 {
			return flow, exits
		}
		engine.mergeJumpStates(continuations)
		if !exact {
			bodyPreservesInduction = bodyPreservesInduction && ps6101SameValue(segmentInduction, engine.state[induction])
			// Classify body reachability while the induction interval still
			// denotes this segment; the post statement may overwrite it.
			engine.invalidateSegmentLoopEffects(statement)
		}
		if !exact && bodyPreservesInduction {
			var next int64
			if direction >= 0 {
				if segment.upper == math.MaxInt64 {
					engine.invalidateFutureLoopEffects(statement)
					return flow, append(exits, engine.currentExitState())
				}
				next = segment.upper + 1
			} else {
				if segment.lower == math.MinInt64 {
					engine.invalidateFutureLoopEffects(statement)
					return flow, append(exits, engine.currentExitState())
				}
				next = segment.lower - 1
			}
			// The recognized post is a unit update of induction. An interval
			// summarizes all of its iterations, so its successor is the boundary
			// immediately after the interval, not one update of an arbitrary
			// representative value.
			engine.state[induction] = ps6101ConstantValue(constant.MakeInt64(next))
		} else if statement.Post != nil {
			engine.analyzeStatement(statement.Post)
		}

		// The fixed segment schedule remains valid only while the real loop
		// condition is definitely true at the next chronological transfer.
		// A body write to a stored bound or to induction is therefore observed
		// before another milestone is forced. Unknown chronology is summarized
		// conservatively without admitting diagnostics from later segments.
		nextCondition := engine.boolConstant(statement.Cond)
		if nextCondition < 0 {
			return flow, append(exits, engine.currentExitState())
		}
		if nextCondition == 0 {
			engine.invalidateFutureLoopEffects(statement)
			return flow, append(exits, engine.currentExitState())
		}
	}
	// A true condition beyond the derived interval means that a stored bound
	// grew. Do not invent indexes beyond the interval; summarize their effects.
	engine.invalidateFutureLoopEffects(statement)
	return flow, append(exits, engine.currentExitState())
}

func (engine *ps6101Engine) loopSegmentPreservesLocation(body *ast.BlockStmt, location ps6101Location) bool {
	before := engine.currentExitState()
	value, present := before.state[location]
	if !present {
		return false
	}
	engine.invalidateLoopEffects(&ast.ForStmt{Body: body}, engine.abstractLoopBool)
	after, stillPresent := engine.state[location]
	engine.loadExitState(before)
	return stillPresent && ps6101SameValue(value, after)
}

func (engine *ps6101Engine) analyzeBoundedForFallback(
	statement *ast.ForStmt,
	label types.Object,
	induction ps6101Location,
	bounds ps6101Value,
) (ps6101Flow, []ps6101ExitState) {
	engine.state[induction] = bounds
	chronologyStable := engine.loopBodyPreservesCondition(statement.Body, statement.Cond)
	diagnostics := engine.snapshotDiagnostics()
	flow, exits, continuations, control := engine.analyzeForTransfer(statement, label, 1)
	if control || !chronologyStable {
		engine.restoreDiagnostics(diagnostics)
		ps6101RestoreExitDiagnostics(continuations, diagnostics)
		if !chronologyStable {
			// Once the abstract tail can change its own condition, no gate in
			// this undivided interval has a proven chronological predecessor.
			ps6101RestoreExitDiagnostics(exits, diagnostics)
		}
	}
	if len(continuations) == 0 {
		return flow, exits
	}
	engine.mergeJumpStates(continuations)
	engine.invalidateSegmentLoopEffects(statement)
	if statement.Post != nil {
		engine.analyzeStatement(statement.Post)
	}
	return flow, append(exits, engine.currentExitState())
}

func (engine *ps6101Engine) loopBodyPreservesCondition(body *ast.BlockStmt, condition ast.Expr) bool {
	before := engine.currentExitState()
	if engine.abstractLoopBool(condition) != 1 {
		return false
	}
	engine.invalidateLoopEffects(&ast.ForStmt{Body: body}, engine.abstractLoopBool)
	preserved := engine.abstractLoopBool(condition) == 1
	engine.loadExitState(before)
	return preserved
}

func ps6101RemainingLoopBounds(bounds ps6101Value, segment ps6101LoopSegment, direction int) ps6101Value {
	analysis := bounds.analysisValue()
	if analysis.lower == nil || analysis.upper == nil {
		return bounds
	}
	lower, lowerOK := constant.Int64Val(analysis.lower)
	upper, upperOK := constant.Int64Val(analysis.upper)
	if !lowerOK || !upperOK {
		return bounds
	}
	if direction >= 0 {
		lower = segment.lower
	} else {
		upper = segment.upper
	}
	return ps6101LoopIndexValue(lower, upper)
}

func ps6101LoopIndexValue(lower, upper int64) ps6101Value {
	if lower == upper {
		return ps6101ConstantValue(constant.MakeInt64(lower))
	}
	value := ps6101Value{}
	if lower >= 0 {
		value.kind = ps6101Nonnegative
	}
	analysis := value.mutableAnalysis()
	analysis.lower = constant.MakeInt64(lower)
	analysis.upper = constant.MakeInt64(upper)
	return value
}

// loopSegments splits a bounded induction interval at every constant compared
// with that induction value. A comparison is therefore either evaluated at
// its exact boundary or over an interval on which its truth cannot change.
func (engine *ps6101Engine) loopSegments(
	body *ast.BlockStmt,
	induction ps6101Location,
	bounds ps6101Value,
	direction int,
) []ps6101LoopSegment {
	analysis := bounds.analysisValue()
	if analysis.lower == nil || analysis.upper == nil {
		return nil
	}
	lower, lowerOK := constant.Int64Val(analysis.lower)
	upper, upperOK := constant.Int64Val(analysis.upper)
	if !lowerOK || !upperOK || lower > upper {
		return nil
	}

	var milestones []int64
	addMilestone := func(expression ast.Expr) {
		value := engine.abstractIntegerConstant(expression)
		if value == nil || value.Kind() != constant.Int {
			return
		}
		milestone, ok := constant.Int64Val(value)
		if ok && milestone >= lower && milestone <= upper {
			milestones = append(milestones, milestone)
		}
	}
	comparison := func(expression *ast.BinaryExpr) {
		switch expression.Op {
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
		default:
			return
		}
		if location, ok := engine.loopInductionLocation(expression.X); ok && location == induction {
			addMilestone(expression.Y)
			return
		}
		if location, ok := engine.loopInductionLocation(expression.Y); ok && location == induction {
			addMilestone(expression.X)
		}
	}
	ast.Inspect(body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.BinaryExpr:
			comparison(value)
		case *ast.SwitchStmt:
			if value.Tag == nil {
				return true
			}
			location, ok := engine.loopInductionLocation(value.Tag)
			if !ok || location != induction {
				return true
			}
			for _, raw := range value.Body.List {
				clause, ok := raw.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, expression := range clause.List {
					addMilestone(expression)
				}
			}
		}
		return true
	})
	slices.Sort(milestones)
	milestones = slices.Compact(milestones)

	segments := make([]ps6101LoopSegment, 0, len(milestones)*2+1)
	if direction >= 0 {
		cursor := lower
		for _, milestone := range milestones {
			if cursor < milestone {
				segments = append(segments, ps6101LoopSegment{lower: cursor, upper: milestone - 1})
			}
			segments = append(segments, ps6101LoopSegment{lower: milestone, upper: milestone})
			if milestone == upper {
				return segments
			}
			cursor = milestone + 1
		}
		if cursor <= upper {
			segments = append(segments, ps6101LoopSegment{lower: cursor, upper: upper})
		}
		return segments
	}

	cursor := upper
	for index := len(milestones) - 1; index >= 0; index-- {
		milestone := milestones[index]
		if milestone < cursor {
			segments = append(segments, ps6101LoopSegment{lower: milestone + 1, upper: cursor})
		}
		segments = append(segments, ps6101LoopSegment{lower: milestone, upper: milestone})
		if milestone == lower {
			return segments
		}
		cursor = milestone - 1
	}
	if cursor >= lower {
		segments = append(segments, ps6101LoopSegment{lower: lower, upper: cursor})
	}
	return segments
}

func (engine *ps6101Engine) loopInductionLocation(expression ast.Expr) (ps6101Location, bool) {
	for {
		call, ok := ps6101Unparen(expression).(*ast.CallExpr)
		if !ok || len(call.Args) != 1 || !engine.pass.TypesInfo.Types[call.Fun].IsType() {
			return engine.location(expression, true)
		}
		expression = call.Args[0]
	}
}

// forRemainderInduction recognizes the canonical unit-step loops whose tail
// can be represented as a closed interval without admitting indexes outside
// the real trip count.
func (engine *ps6101Engine) forRemainderInduction(statement *ast.ForStmt) (ps6101Location, ps6101Value, int, bool) {
	var identifier *ast.Ident
	direction := 0
	switch post := statement.Post.(type) {
	case *ast.IncDecStmt:
		identifier, _ = ps6101Unparen(post.X).(*ast.Ident)
		if post.Tok == token.INC {
			direction = 1
		} else if post.Tok == token.DEC {
			direction = -1
		}
	case *ast.AssignStmt:
		if len(post.Lhs) == 1 && len(post.Rhs) == 1 {
			identifier, _ = ps6101Unparen(post.Lhs[0]).(*ast.Ident)
			step := engine.abstractIntegerConstant(post.Rhs[0])
			if step != nil && constant.Compare(step, token.EQL, constant.MakeInt64(1)) {
				if post.Tok == token.ADD_ASSIGN {
					direction = 1
				} else if post.Tok == token.SUB_ASSIGN {
					direction = -1
				}
			}
		}
	}
	condition, ok := ps6101Unparen(statement.Cond).(*ast.BinaryExpr)
	if !ok || identifier == nil || direction == 0 {
		return ps6101Location{}, ps6101Value{}, 0, false
	}
	inductionObject := identObject(engine.pass, identifier)
	conditionIdentifier, left := ps6101Unparen(condition.X).(*ast.Ident)
	boundExpression := condition.Y
	operation := condition.Op
	if !left || identObject(engine.pass, conditionIdentifier) != inductionObject {
		conditionIdentifier, left = ps6101Unparen(condition.Y).(*ast.Ident)
		if !left || identObject(engine.pass, conditionIdentifier) != inductionObject {
			return ps6101Location{}, ps6101Value{}, 0, false
		}
		boundExpression = condition.X
		operation = ps6101ReverseComparison(operation)
	}
	location, located := engine.location(identifier, true)
	current, present := engine.state[location]
	bound := engine.abstractIntegerConstant(boundExpression)
	if !located || !present || current.constant == nil || bound == nil {
		return ps6101Location{}, ps6101Value{}, 0, false
	}
	lower, upper := current.constant, current.constant
	one := constant.MakeInt64(1)
	switch {
	case direction > 0 && operation == token.LSS:
		upper = constant.BinaryOp(bound, token.SUB, one)
	case direction > 0 && operation == token.LEQ:
		upper = bound
	case direction < 0 && operation == token.GTR:
		lower = constant.BinaryOp(bound, token.ADD, one)
	case direction < 0 && operation == token.GEQ:
		lower = bound
	default:
		return ps6101Location{}, ps6101Value{}, 0, false
	}
	if constant.Compare(lower, token.GTR, upper) {
		return ps6101Location{}, ps6101Value{}, 0, false
	}
	if _, ok := constant.Int64Val(lower); !ok {
		return ps6101Location{}, ps6101Value{}, 0, false
	}
	if _, ok := constant.Int64Val(upper); !ok {
		return ps6101Location{}, ps6101Value{}, 0, false
	}
	value := ps6101Value{}
	if constant.Sign(lower) >= 0 {
		value.kind = ps6101Nonnegative
	}
	analysis := value.mutableAnalysis()
	analysis.lower, analysis.upper = lower, upper
	return location, value, direction, true
}

func (engine *ps6101Engine) isBLoopCondition(expression ast.Expr) bool {
	call, ok := ps6101Unparen(expression).(*ast.CallExpr)
	return ok && engine.loopConditionTestingMethod(call) == "Loop"
}

func (engine *ps6101Engine) isParallelLoopCondition(expression ast.Expr) bool {
	call, ok := ps6101Unparen(expression).(*ast.CallExpr)
	return ok && engine.loopConditionTestingMethod(call) == "Next"
}

func (engine *ps6101Engine) loopConditionTestingMethod(call *ast.CallExpr) string {
	if call == nil {
		return ""
	}
	// A call's function operand can itself be a local helper call returning a
	// testing method value. Evaluating just that operand both resolves the
	// callable and preserves the required function-before-arguments order.
	// analyzeForLabeled keeps this value in its single-evaluation cache until
	// the complete condition call has consumed its arguments.
	if target := engine.eval(call.Fun); target.callable != nil && target.callable.testingMethod != "" {
		return target.callable.testingMethod
	}
	return engine.testingMethod(call)
}

func ps6101EquivalentStates(left, right map[ps6101Location]ps6101Value) bool {
	if len(left) != len(right) {
		return false
	}
	for location, leftValue := range left {
		rightValue, ok := right[location]
		if !ok {
			return false
		}
		leftValue.revision, leftValue.identity = 0, 0
		rightValue.revision, rightValue.identity = 0, 0
		leftAnalysis := leftValue.mutableAnalysis()
		rightAnalysis := rightValue.mutableAnalysis()
		leftAnalysis.squareID, leftAnalysis.squareSig, leftAnalysis.squareSign = 0, "", 0
		rightAnalysis.squareID, rightAnalysis.squareSig, rightAnalysis.squareSign = 0, "", 0
		if !ps6101SameValue(leftValue, rightValue) {
			return false
		}
	}
	return true
}

func ps6101StaticBool(pass *analysis.Pass, expression ast.Expr) int {
	if expression == nil {
		return 1
	}
	value := pass.TypesInfo.Types[expression].Value
	if value == nil || value.Kind() != constant.Bool {
		return 0
	}
	if constant.BoolVal(value) {
		return 1
	}
	return -1
}

func (engine *ps6101Engine) invalidateFutureLoopEffects(statement *ast.ForStmt) {
	engine.invalidateLoopEffects(statement, nil)
}

func (engine *ps6101Engine) invalidateSegmentLoopEffects(statement *ast.ForStmt) {
	engine.invalidateLoopEffects(statement, engine.abstractLoopBool)
}

func (engine *ps6101Engine) abstractLoopBool(expression ast.Expr) int {
	if expression == nil {
		return 1
	}
	expression = ps6101Unparen(expression)
	if value := engine.pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Bool {
		if constant.BoolVal(value) {
			return 1
		}
		return -1
	}
	switch value := expression.(type) {
	case *ast.Ident, *ast.SelectorExpr:
		if location, ok := engine.location(value, true); ok {
			stored := engine.state[location].constant
			if stored != nil && stored.Kind() == constant.Bool {
				if constant.BoolVal(stored) {
					return 1
				}
				return -1
			}
		}
	case *ast.UnaryExpr:
		if value.Op == token.NOT {
			return -engine.abstractLoopBool(value.X)
		}
	case *ast.BinaryExpr:
		left := engine.abstractLoopBool(value.X)
		switch value.Op {
		case token.LAND:
			if left < 0 {
				return -1
			}
			right := engine.abstractLoopBool(value.Y)
			if right < 0 {
				return -1
			}
			if left > 0 && right > 0 {
				return 1
			}
		case token.LOR:
			if left > 0 {
				return 1
			}
			right := engine.abstractLoopBool(value.Y)
			if right > 0 {
				return 1
			}
			if left < 0 && right < 0 {
				return -1
			}
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			leftValue, leftOK := engine.abstractLoopValue(value.X)
			rightValue, rightOK := engine.abstractLoopValue(value.Y)
			if leftOK && rightOK {
				return ps6101CompareBounds(leftValue, value.Op, rightValue)
			}
		}
	}
	return 0
}

func (engine *ps6101Engine) abstractLoopValue(expression ast.Expr) (ps6101Value, bool) {
	expression = ps6101Unparen(expression)
	if value := engine.pass.TypesInfo.Types[expression].Value; value != nil {
		return ps6101ConstantValue(value), true
	}
	if call, ok := expression.(*ast.CallExpr); ok && len(call.Args) == 1 && engine.pass.TypesInfo.Types[call.Fun].IsType() {
		return engine.abstractLoopValue(call.Args[0])
	}
	if location, ok := engine.location(expression, true); ok {
		if value, present := engine.state[location]; present {
			return value, true
		}
	}
	return ps6101Value{}, false
}

func (engine *ps6101Engine) invalidateLoopEffects(statement *ast.ForStmt, condition func(ast.Expr) int) {
	var writes []ps6101Location
	timerMayChange := false
	remember := func(expression ast.Expr) {
		if expression == nil {
			return
		}
		if location, ok := engine.location(expression, true); ok {
			writes = append(writes, location)
		}
	}
	var inspect func(ast.Node)
	inspect = func(root ast.Node) {
		if root == nil {
			return
		}
		ast.Inspect(root, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.IfStmt:
				if condition == nil {
					return true
				}
				inspect(value.Init)
				inspect(value.Cond)
				switch condition(value.Cond) {
				case 1:
					inspect(value.Body)
				case -1:
					inspect(value.Else)
				default:
					inspect(value.Body)
					inspect(value.Else)
				}
				return false
			case *ast.SwitchStmt:
				if condition == nil {
					return true
				}
				inspect(value.Init)
				inspect(value.Tag)
				clauses := make([]*ast.CaseClause, 0, len(value.Body.List))
				for _, raw := range value.Body.List {
					clause, ok := raw.(*ast.CaseClause)
					if !ok {
						continue
					}
					clauses = append(clauses, clause)
					for _, expression := range clause.List {
						inspect(expression)
					}
				}
				tagValue := ps6101Value{}
				if value.Tag != nil {
					var ok bool
					tagValue, ok = engine.abstractLoopValue(value.Tag)
					if !ok {
						inspect(value.Body)
						return false
					}
				}
				reachable, _ := engine.switchReachability(value.Tag, tagValue, clauses)
				for index := range reachable {
					if index > 0 && reachable[index-1] && ps6101ClauseFallsThrough(clauses[index-1]) {
						reachable[index] = true
					}
					if reachable[index] {
						inspect(&ast.BlockStmt{List: clauses[index].Body})
					}
				}
				return false
			case *ast.AssignStmt:
				for _, expression := range value.Lhs {
					remember(expression)
				}
			case *ast.IncDecStmt:
				remember(value.X)
			case *ast.RangeStmt:
				remember(value.Key)
				remember(value.Value)
			case *ast.CallExpr:
				if method := engine.testingMethod(value); method != "" {
					switch method {
					case "ResetTimer", "StartTimer", "StopTimer", "Run":
						timerMayChange = true
					}
					return true
				}
				if identifier, ok := ps6101Unparen(value.Fun).(*ast.Ident); ok {
					if _, ok := identObject(engine.pass, identifier).(*types.Builtin); ok {
						return true
					}
				}
				for _, argument := range value.Args {
					if engine.testingExpression(argument) {
						timerMayChange = true
						continue
					}
					typed := engine.pass.TypesInfo.Types[argument].Type
					if typed != nil && ps6101ContainsReference(typed) {
						remember(argument)
					}
				}
			}
			return true
		})
	}
	inspect(statement.Body)
	inspect(statement.Post)
	for _, location := range writes {
		engine.invalidatePrefix(location)
	}
	if timerMayChange {
		engine.timer |= ps6101TimerRunning | ps6101TimerStopped
	}
}

func (engine *ps6101Engine) testingExpression(expression ast.Expr) bool {
	if expression == nil {
		return false
	}
	typ := engine.pass.TypesInfo.TypeOf(expression)
	if typ == nil {
		return false
	}
	typ = types.Unalias(typ)
	pointer, ok := typ.(*types.Pointer)
	if ok {
		named, namedOK := types.Unalias(pointer.Elem()).(*types.Named)
		if namedOK && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "testing" && named.Obj().Name() == "B" {
			return true
		}
	}
	if location, ok := engine.location(expression, true); ok {
		return engine.state[location].testing
	}
	return false
}

func (engine *ps6101Engine) invalidatePrefix(prefix ps6101Location) {
	engine.killPrefix(prefix)
	// Keep an explicit unknown at the written location so an indexed or field
	// read cannot fall back to stale aggregate provenance from its parent.
	engine.state[prefix] = ps6101Value{}
	for location := range engine.counterGates {
		if ps6101HasLocationPrefix(location, prefix) {
			delete(engine.counterGates, location)
		}
	}
	for location := range engine.counterRevs {
		if ps6101HasLocationPrefix(location, prefix) {
			delete(engine.counterRevs, location)
		}
	}
	for object, location := range engine.aliases {
		if object == prefix.root || ps6101HasLocationPrefix(location, prefix) {
			delete(engine.aliases, object)
		}
	}
}

func (engine *ps6101Engine) invalidateLocationAndReferences(prefix ps6101Location) {
	pending := []ps6101Location{prefix}
	seen := make(map[ps6101Location]bool)
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if current.root == nil || seen[current] {
			continue
		}
		seen[current] = true
		for location, value := range engine.state {
			if ps6101HasLocationPrefix(location, current) && value.reference != nil && !seen[*value.reference] {
				pending = append(pending, *value.reference)
			}
		}
		engine.invalidatePrefix(current)
	}
}

func (engine *ps6101Engine) rangeMapMayMutate(statement *ast.RangeStmt) bool {
	if statement == nil {
		return false
	}
	typ := engine.pass.TypesInfo.TypeOf(statement.X)
	if typ == nil {
		return false
	}
	if _, ok := types.Unalias(typ).Underlying().(*types.Map); !ok {
		return false
	}
	ranged, ok := engine.location(statement.X, true)
	if !ok {
		return true
	}
	touches := func(expression ast.Expr) bool {
		location, present := engine.location(expression, true)
		return present && (ps6101HasLocationPrefix(location, ranged) || ps6101HasLocationPrefix(ranged, location))
	}
	mutates := false
	ast.Inspect(statement.Body, func(node ast.Node) bool {
		if mutates {
			return false
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			// A stored closure is inert until its call is inspected separately.
			return false
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				if indexed, ok := ps6101Unparen(left).(*ast.IndexExpr); ok && touches(indexed.X) {
					mutates = true
					return false
				}
			}
		case *ast.IncDecStmt:
			if indexed, ok := ps6101Unparen(value.X).(*ast.IndexExpr); ok && touches(indexed.X) {
				mutates = true
				return false
			}
		case *ast.CallExpr:
			if identifier, ok := ps6101Unparen(value.Fun).(*ast.Ident); ok {
				if builtin, ok := identObject(engine.pass, identifier).(*types.Builtin); ok {
					if (builtin.Name() == "clear" || builtin.Name() == "delete") && len(value.Args) > 0 && touches(value.Args[0]) {
						mutates = true
						return false
					}
					return true
				}
			}
			for _, argument := range value.Args {
				if touches(argument) {
					mutates = true
					return false
				}
			}
			if engine.callableMayMutateRangeMap(value.Fun, ranged, false) {
				mutates = true
				return false
			}
		}
		return true
	})
	return mutates
}

func (engine *ps6101Engine) callableMayMutateRangeMap(expression ast.Expr, ranged ps6101Location, shrinkOnly bool) bool {
	if !ps6101SideEffectFreeCallableExpression(expression) {
		return true
	}
	value := engine.eval(expression)
	callable := value.callable
	if callable == nil {
		return false
	}
	touchesReference := func(value ps6101Value) bool {
		return value.reference != nil && (ps6101HasLocationPrefix(*value.reference, ranged) || ps6101HasLocationPrefix(ranged, *value.reference))
	}
	if callable.receiverValue != nil && touchesReference(*callable.receiverValue) {
		return true
	}
	for _, field := range callable.receiverFields {
		if touchesReference(field) {
			return true
		}
	}
	var body *ast.BlockStmt
	switch {
	case callable.literal != nil:
		body = callable.literal.Body
	case callable.function != nil:
		body = callable.function.Body
	}
	if body == nil {
		return false
	}
	touches := func(candidate ast.Expr) bool {
		location, present := engine.location(candidate, true)
		return present && (ps6101HasLocationPrefix(location, ranged) || ps6101HasLocationPrefix(ranged, location))
	}
	mutates := false
	ast.Inspect(body, func(node ast.Node) bool {
		if mutates {
			return false
		}
		switch candidate := node.(type) {
		case *ast.FuncLit:
			return candidate == callable.literal
		case *ast.AssignStmt:
			if shrinkOnly {
				return true
			}
			for _, left := range candidate.Lhs {
				if indexed, ok := ps6101Unparen(left).(*ast.IndexExpr); ok && touches(indexed.X) {
					mutates = true
					return false
				}
			}
		case *ast.IncDecStmt:
			if shrinkOnly {
				return true
			}
			if indexed, ok := ps6101Unparen(candidate.X).(*ast.IndexExpr); ok && touches(indexed.X) {
				mutates = true
				return false
			}
		case *ast.CallExpr:
			if identifier, ok := ps6101Unparen(candidate.Fun).(*ast.Ident); ok {
				if builtin, ok := identObject(engine.pass, identifier).(*types.Builtin); ok {
					if (builtin.Name() == "clear" || builtin.Name() == "delete") && len(candidate.Args) > 0 && touches(candidate.Args[0]) {
						mutates = true
						return false
					}
					return true
				}
			}
			for _, argument := range candidate.Args {
				if touches(argument) {
					mutates = true
					return false
				}
			}
		}
		return true
	})
	return mutates
}

func ps6101SideEffectFreeCallableExpression(expression ast.Expr) bool {
	switch value := ps6101Unparen(expression).(type) {
	case *ast.Ident:
		return true
	case *ast.SelectorExpr:
		return ps6101SideEffectFreeCallableExpression(value.X)
	case *ast.StarExpr:
		return ps6101SideEffectFreeCallableExpression(value.X)
	}
	return false
}

func (engine *ps6101Engine) rangeMapDefinitelyClears(statement *ast.RangeStmt) bool {
	if statement == nil {
		return false
	}
	ranged, ok := engine.location(statement.X, true)
	if !ok {
		return false
	}
	touches := func(expression ast.Expr) bool {
		location, present := engine.location(expression, true)
		return present && (ps6101HasLocationPrefix(location, ranged) || ps6101HasLocationPrefix(ranged, location))
	}
	var statementsClear func([]ast.Stmt) bool
	statementsClear = func(statements []ast.Stmt) bool {
		for _, statement := range statements {
			switch value := statement.(type) {
			case *ast.ExprStmt:
				call, ok := ps6101Unparen(value.X).(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					continue
				}
				identifier, identifierOK := ps6101Unparen(call.Fun).(*ast.Ident)
				if !identifierOK {
					continue
				}
				builtin, ok := identObject(engine.pass, identifier).(*types.Builtin)
				if ok && builtin.Name() == "clear" && touches(call.Args[0]) {
					return true
				}
			case *ast.BlockStmt:
				if statementsClear(value.List) {
					return true
				}
			case *ast.ReturnStmt, *ast.BranchStmt:
				return false
			}
		}
		return false
	}
	return statementsClear(statement.Body.List)
}

func (engine *ps6101Engine) rangeMapMayShrink(statement *ast.RangeStmt) bool {
	if statement == nil {
		return false
	}
	ranged, ok := engine.location(statement.X, true)
	if !ok {
		return true
	}
	touches := func(expression ast.Expr) bool {
		location, present := engine.location(expression, true)
		return present && (ps6101HasLocationPrefix(location, ranged) || ps6101HasLocationPrefix(ranged, location))
	}
	shrinks := false
	ast.Inspect(statement.Body, func(node ast.Node) bool {
		if shrinks {
			return false
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			if identifier, ok := ps6101Unparen(value.Fun).(*ast.Ident); ok {
				if builtin, ok := identObject(engine.pass, identifier).(*types.Builtin); ok {
					if (builtin.Name() == "clear" || builtin.Name() == "delete") && len(value.Args) > 0 && touches(value.Args[0]) {
						shrinks = true
						return false
					}
					return true
				}
			}
			for _, argument := range value.Args {
				if touches(argument) {
					shrinks = true
					return false
				}
			}
			if engine.callableMayMutateRangeMap(value.Fun, ranged, true) {
				shrinks = true
				return false
			}
		}
		return true
	})
	return shrinks
}

func (engine *ps6101Engine) analyzeRangeLabeled(statement *ast.RangeStmt, label types.Object) ps6101Flow {
	ranged := engine.eval(statement.X)
	entryState, entryAliases := engine.cloneState(), engine.cloneAliases()
	entryGates := slices.Clone(engine.gates)
	entryTimer, entryRecoverable := engine.timer, engine.recoverable
	entryCounterGates, entryCounterRevs := ps6101CopyCounterGates(engine.counterGates), ps6101CopyCounterRevisions(engine.counterRevs)
	count, countOK := ranged.length, ranged.lengthOK
	if !countOK && ranged.constant != nil && ranged.constant.Kind() == constant.Int {
		count, countOK = constant.Int64Val(ranged.constant)
		countOK = countOK && count >= 0
	}
	mapType := false
	if typ := engine.pass.TypesInfo.TypeOf(statement.X); typ != nil {
		_, mapType = types.Unalias(typ).Underlying().(*types.Map)
	}
	knownNonemptyMap := mapType && countOK && count > 0
	mapMayShrink := knownNonemptyMap && engine.rangeMapMayShrink(statement)
	if countOK && engine.rangeMapMayMutate(statement) {
		if engine.rangeMapDefinitelyClears(statement) {
			count = min(count, 1)
		} else {
			countOK = false
		}
	}
	if countOK && count == 0 {
		return ps6101FallsThrough
	}
	limit := int64(1)
	if countOK {
		limit = count
	}
	flow := ps6101Flow(0)
	var breakStates []ps6101ExitState
	var completionStates []ps6101ExitState
	exhausted := false
	iteration := int64(0)
	for ; iteration < limit; iteration++ {
		if countOK && !engine.takeExactLoopTransfer() {
			exhausted = true
			break
		}
		engine.bindRangeIteration(statement, ranged, iteration, countOK, count, countOK)
		if !countOK {
			engine.conditional++
		}
		control := &ps6101ControlStates{}
		engine.breakTargets = append(engine.breakTargets, control)
		engine.continueTargets = append(engine.continueTargets, control)
		bodyFlow := engine.analyzeBlock(statement.Body)
		engine.breakTargets = engine.breakTargets[:len(engine.breakTargets)-1]
		engine.continueTargets = engine.continueTargets[:len(engine.continueTargets)-1]
		if !countOK {
			engine.conditional--
		}
		flow |= bodyFlow & (ps6101Returns | ps6101Gotos)
		breakStates = append(breakStates, control.breaks...)
		continuations := slices.Clone(control.continues)
		if bodyFlow&ps6101FallsThrough != 0 {
			continuations = append(continuations, engine.currentExitState())
		}
		if label != nil {
			if control := engine.controls[label]; control != nil {
				breakStates = append(breakStates, control.breaks...)
				continuations = append(continuations, control.continues...)
				delete(engine.controls, label)
			}
		}
		if len(continuations) == 0 {
			if len(breakStates) > 0 {
				engine.mergeMayExitStates(breakStates)
				return flow | ps6101FallsThrough
			}
			return flow
		}
		if countOK && iteration == count-1 {
			completionStates = append(completionStates, continuations...)
			break
		}
		engine.mergeJumpStates(continuations)
	}
	if countOK {
		if exhausted {
			abstractFlow, abstractBreaks, abstractCompletions := engine.analyzeRangeRemainder(statement, ranged, label, iteration, count-iteration)
			flow |= abstractFlow
			breakStates = append(breakStates, abstractBreaks...)
			completionStates = append(completionStates, abstractCompletions...)
			if len(abstractBreaks) == 0 && len(abstractCompletions) == 0 {
				return flow
			}
		}
		completionStates = append(completionStates, breakStates...)
		if len(completionStates) > 0 {
			engine.mergeMayExitStates(completionStates)
		}
		return flow | ps6101FallsThrough
	}
	firstIterationExit := engine.currentExitState()
	engine.invalidateFutureLoopEffects(&ast.ForStmt{Body: statement.Body})
	iterationState, iterationAliases := engine.cloneState(), engine.cloneAliases()
	iterationGates := slices.Clone(engine.gates)
	iterationTimer, iterationRecoverable := engine.timer, engine.recoverable
	iterationCounterGates, iterationCounterRevs := ps6101CopyCounterGates(engine.counterGates), ps6101CopyCounterRevisions(engine.counterRevs)
	if knownNonemptyMap {
		if mapMayShrink {
			engine.mergeMayExitStates([]ps6101ExitState{firstIterationExit, engine.currentExitState()})
		}
	} else {
		engine.mergeFallthrough(
			[]map[ps6101Location]ps6101Value{entryState, iterationState},
			[]map[types.Object]ps6101Location{entryAliases, iterationAliases},
		)
		engine.mergeCounterFlow(
			[]map[ps6101Location]ps6101CounterEvidence{entryCounterGates, iterationCounterGates},
			[]map[ps6101Location]uint64{entryCounterRevs, iterationCounterRevs},
		)
		engine.gates = ps6101MergeGates(entryGates, iterationGates)
		engine.timer = entryTimer | iterationTimer
		engine.recoverable = max(entryRecoverable, iterationRecoverable)
	}
	if len(breakStates) > 0 {
		breakStates = append(breakStates, engine.currentExitState())
		engine.mergeMayExitStates(breakStates)
	}
	return flow | ps6101FallsThrough
}

func (engine *ps6101Engine) takeExactLoopTransfer() bool {
	if engine.loopTransfers == nil {
		engine.loopTransfers = &ps6101LoopTransferBudget{
			remaining: ps6101ExactLoopTransferLimit, abstractRemaining: ps6101ExactLoopTransferLimit,
		}
	}
	if engine.loopTransfers.remaining == 0 {
		return false
	}
	engine.loopTransfers.remaining--
	engine.loopTransfers.spent++
	return true
}

func (engine *ps6101Engine) takeAbstractLoopTransfer() bool {
	if engine.loopTransfers == nil {
		engine.loopTransfers = &ps6101LoopTransferBudget{
			remaining: ps6101ExactLoopTransferLimit, abstractRemaining: ps6101ExactLoopTransferLimit,
		}
	}
	if engine.loopTransfers.abstractRemaining == 0 {
		return false
	}
	engine.loopTransfers.abstractRemaining--
	engine.loopTransfers.abstractSpent++
	return true
}

// analyzeRangeRemainder preserves the first two chronological transfers after
// budget exhaustion, then visits the remaining tail once with a bounded index.
// This discovers late gates without multiplying nested trip counts. The
// possible abstract completion is joined with a summary of subsequent writes.
func (engine *ps6101Engine) analyzeRangeRemainder(
	statement *ast.RangeStmt,
	ranged ps6101Value,
	label types.Object,
	iteration int64,
	remaining int64,
) (ps6101Flow, []ps6101ExitState, []ps6101ExitState) {
	count := iteration + remaining
	if engine.loopAbstract > 0 {
		if induction, ok := engine.location(statement.Key, true); ok {
			bounds := ps6101LoopIndexValue(iteration, count-1)
			return engine.analyzeBoundedRangeTail(statement, ranged, label, induction, bounds)
		}
		diagnostics := engine.snapshotDiagnostics()
		flow, breaks, continuations, control := engine.analyzeRangeTransfer(statement, ranged, label, iteration, false, count, true)
		if control {
			engine.restoreDiagnostics(diagnostics)
			ps6101RestoreExitDiagnostics(continuations, diagnostics)
		}
		if len(continuations) == 0 {
			return flow, breaks, nil
		}
		engine.mergeJumpStates(continuations)
		completion := engine.currentExitState()
		engine.invalidateFutureLoopEffects(&ast.ForStmt{Body: statement.Body})
		return flow, breaks, []ps6101ExitState{completion, engine.currentExitState()}
	}
	engine.loopAbstract++
	defer func() { engine.loopAbstract-- }()

	flow, breaks, continuations, _ := engine.analyzeRangeTransfer(statement, ranged, label, iteration, true, count, true)
	if len(continuations) == 0 {
		return flow, breaks, nil
	}
	engine.mergeJumpStates(continuations)
	if remaining == 1 {
		return flow, breaks, []ps6101ExitState{engine.currentExitState()}
	}

	secondFlow, secondBreaks, secondContinuations, _ := engine.analyzeRangeTransfer(statement, ranged, label, iteration+1, true, count, true)
	flow |= secondFlow
	breaks = append(breaks, secondBreaks...)
	if len(secondContinuations) == 0 {
		return flow, breaks, nil
	}
	engine.mergeJumpStates(secondContinuations)
	if remaining == 2 {
		return flow, breaks, []ps6101ExitState{engine.currentExitState()}
	}

	if induction, ok := engine.location(statement.Key, true); ok {
		bounds := ps6101LoopIndexValue(iteration+2, count-1)
		tailFlow, tailBreaks, tailCompletions := engine.analyzeBoundedRangeTail(statement, ranged, label, induction, bounds)
		return flow | tailFlow, append(breaks, tailBreaks...), tailCompletions
	}

	diagnostics := engine.snapshotDiagnostics()
	tailFlow, tailBreaks, tailContinuations, control := engine.analyzeRangeTransfer(statement, ranged, label, iteration+2, false, count, true)
	if control {
		// A terminating exit at an unknown chronological position can prevent
		// every later iteration. Keep the state summary, but do not turn a gate
		// from another possible index into a diagnostic.
		engine.restoreDiagnostics(diagnostics)
		ps6101RestoreExitDiagnostics(tailContinuations, diagnostics)
	}
	flow |= tailFlow
	breaks = append(breaks, tailBreaks...)
	if len(tailContinuations) == 0 {
		return flow, breaks, nil
	}
	engine.mergeJumpStates(tailContinuations)
	completion := engine.currentExitState()
	engine.invalidateFutureLoopEffects(&ast.ForStmt{Body: statement.Body})
	return flow, breaks, []ps6101ExitState{completion, engine.currentExitState()}
}

func (engine *ps6101Engine) analyzeBoundedRangeTail(
	statement *ast.RangeStmt,
	ranged ps6101Value,
	label types.Object,
	induction ps6101Location,
	bounds ps6101Value,
) (ps6101Flow, []ps6101ExitState, []ps6101ExitState) {
	segments := engine.loopSegments(statement.Body, induction, bounds, 1)
	if len(segments) == 0 {
		return engine.analyzeBoundedRangeFallback(statement, ranged, label, bounds)
	}
	flow := ps6101Flow(0)
	var breaks []ps6101ExitState
	for _, segment := range segments {
		if !engine.takeAbstractLoopTransfer() {
			remaining := ps6101RemainingLoopBounds(bounds, segment, 1)
			fallbackFlow, fallbackBreaks, fallbackCompletions := engine.analyzeBoundedRangeFallback(
				statement, ranged, label, remaining,
			)
			return flow | fallbackFlow, append(breaks, fallbackBreaks...), fallbackCompletions
		}
		exact := segment.lower == segment.upper
		diagnostics := engine.snapshotDiagnostics()
		segmentFlow, segmentBreaks, continuations, control := engine.analyzeRangeTransfer(
			statement, ranged, label, segment.lower, exact, segment.upper+1, true,
		)
		flow |= segmentFlow
		if !exact && control {
			engine.restoreDiagnostics(diagnostics)
			ps6101RestoreExitDiagnostics(continuations, diagnostics)
		}
		breaks = append(breaks, segmentBreaks...)
		if len(continuations) == 0 {
			return flow, breaks, nil
		}
		engine.mergeJumpStates(continuations)
		if !exact {
			engine.invalidateSegmentLoopEffects(&ast.ForStmt{Body: statement.Body})
		}
	}
	return flow, breaks, []ps6101ExitState{engine.currentExitState()}
}

func (engine *ps6101Engine) analyzeBoundedRangeFallback(
	statement *ast.RangeStmt,
	ranged ps6101Value,
	label types.Object,
	bounds ps6101Value,
) (ps6101Flow, []ps6101ExitState, []ps6101ExitState) {
	analysis := bounds.analysisValue()
	if analysis.lower == nil || analysis.upper == nil {
		return 0, nil, []ps6101ExitState{engine.currentExitState()}
	}
	lower, lowerOK := constant.Int64Val(analysis.lower)
	upper, upperOK := constant.Int64Val(analysis.upper)
	if !lowerOK || !upperOK || lower > upper || upper == math.MaxInt64 {
		return 0, nil, []ps6101ExitState{engine.currentExitState()}
	}
	diagnostics := engine.snapshotDiagnostics()
	flow, breaks, continuations, control := engine.analyzeRangeTransfer(statement, ranged, label, lower, false, upper+1, true)
	if control {
		engine.restoreDiagnostics(diagnostics)
		ps6101RestoreExitDiagnostics(continuations, diagnostics)
	}
	if len(continuations) == 0 {
		return flow, breaks, nil
	}
	engine.mergeJumpStates(continuations)
	engine.invalidateSegmentLoopEffects(&ast.ForStmt{Body: statement.Body})
	return flow, breaks, []ps6101ExitState{engine.currentExitState()}
}

func (engine *ps6101Engine) analyzeRangeTransfer(
	statement *ast.RangeStmt,
	ranged ps6101Value,
	label types.Object,
	iteration int64,
	exact bool,
	count int64,
	countOK bool,
) (ps6101Flow, []ps6101ExitState, []ps6101ExitState, bool) {
	engine.bindRangeIteration(statement, ranged, iteration, exact, count, countOK)
	if !exact {
		engine.conditional++
	}
	control := &ps6101ControlStates{}
	engine.breakTargets = append(engine.breakTargets, control)
	engine.continueTargets = append(engine.continueTargets, control)
	bodyFlow := engine.analyzeBlock(statement.Body)
	engine.breakTargets = engine.breakTargets[:len(engine.breakTargets)-1]
	engine.continueTargets = engine.continueTargets[:len(engine.continueTargets)-1]
	if !exact {
		engine.conditional--
	}

	breaks := slices.Clone(control.breaks)
	continuations := slices.Clone(control.continues)
	if bodyFlow&ps6101FallsThrough != 0 {
		continuations = append(continuations, engine.currentExitState())
	}
	if label != nil {
		if labeled := engine.controls[label]; labeled != nil {
			controlled := len(labeled.breaks) > 0
			breaks = append(breaks, labeled.breaks...)
			continuations = append(continuations, labeled.continues...)
			delete(engine.controls, label)
			if controlled {
				bodyFlow |= ps6101Gotos
			}
		}
	}
	hasControl := len(control.breaks) > 0 || bodyFlow&(ps6101Returns|ps6101Gotos|ps6101Breaks) != 0
	if len(continuations) == 0 {
		return bodyFlow & (ps6101Returns | ps6101Gotos), breaks, nil, hasControl
	}
	return bodyFlow & (ps6101Returns | ps6101Gotos), breaks, continuations, hasControl
}

func (engine *ps6101Engine) bindRangeIteration(statement *ast.RangeStmt, ranged ps6101Value, iteration int64, exact bool, count int64, countOK bool) {
	if identifier, ok := statement.Key.(*ast.Ident); ok && identifier.Name != "_" {
		value := ps6101Value{}
		if exact {
			value = ps6101ConstantValue(constant.MakeInt64(iteration))
		} else if countOK && iteration >= 0 && iteration < count {
			basic, integer := types.Unalias(engine.pass.TypesInfo.TypeOf(identifier)).Underlying().(*types.Basic)
			if integer && basic.Info()&types.IsInteger != 0 {
				value.kind = ps6101Nonnegative
				analysis := value.mutableAnalysis()
				analysis.lower = constant.MakeInt64(iteration)
				analysis.upper = constant.MakeInt64(count - 1)
			}
		}
		engine.store(identifier, value, token.ASSIGN)
	}
	if identifier, ok := statement.Value.(*ast.Ident); ok && identifier.Name != "_" {
		value := ranged
		if location, ok := engine.location(statement.X, true); ok && exact {
			element := location
			element.path += "[" + strconv.FormatInt(iteration, 10) + "]"
			if stored, present := engine.state[element]; present {
				value = stored
			}
		}
		engine.store(identifier, value, token.ASSIGN)
	}
}

func (engine *ps6101Engine) mergeFallthrough(states []map[ps6101Location]ps6101Value, aliases []map[types.Object]ps6101Location) {
	if len(states) == 0 {
		engine.state = make(map[ps6101Location]ps6101Value)
		engine.aliases = make(map[types.Object]ps6101Location)
		return
	}
	engine.state = engine.copyState(states[0])
	for _, state := range states[1:] {
		engine.state = ps6101MergeStates(engine.state, state)
	}
	engine.aliases = engine.copyAliases(aliases[0])
	for _, alias := range aliases[1:] {
		engine.aliases = ps6101MergeAliases(engine.aliases, alias)
	}
}

func (engine *ps6101Engine) captureExit() {
	engine.exits = append(engine.exits, ps6101ExitState{
		state: engine.cloneState(), aliases: engine.cloneAliases(), gates: slices.Clone(engine.gates),
		counterGates: ps6101CopyCounterGates(engine.counterGates),
		counterRevs:  ps6101CopyCounterRevisions(engine.counterRevs),
		timer:        engine.timer, recoverable: engine.recoverable,
	})
}

func (engine *ps6101Engine) mergeExits() {
	if len(engine.exits) == 0 {
		return
	}
	engine.state = engine.copyState(engine.exits[0].state)
	engine.aliases = engine.copyAliases(engine.exits[0].aliases)
	engine.gates = nil
	engine.timer = 0
	engine.recoverable = 0
	counterGates := make([]map[ps6101Location]ps6101CounterEvidence, 0, len(engine.exits))
	counterRevs := make([]map[ps6101Location]uint64, 0, len(engine.exits))
	for index, exit := range engine.exits {
		if index > 0 {
			engine.state = ps6101MergeExitStates(engine.state, exit.state)
			engine.aliases = ps6101MergeAliases(engine.aliases, exit.aliases)
		}
		engine.gates = ps6101MergeGates(engine.gates, exit.gates)
		engine.timer |= exit.timer
		engine.recoverable = max(engine.recoverable, exit.recoverable)
		counterGates = append(counterGates, exit.counterGates)
		counterRevs = append(counterRevs, exit.counterRevs)
	}
	engine.mergeCounterFlow(counterGates, counterRevs)
}

// ps6101MergeExitStates is deliberately a may-risk merge. When one normal
// return path leaves a symmetric benchmark input intact while another path
// overwrites it, the caller can still execute the signed-input path and must
// retain that provenance. Ordinary branch merges remain must-provenance joins.
func ps6101MergeExitStates(left, right map[ps6101Location]ps6101Value) map[ps6101Location]ps6101Value {
	merged := make(map[ps6101Location]ps6101Value)
	locations := make(map[ps6101Location]bool, len(left)+len(right))
	for location := range left {
		locations[location] = true
	}
	for location := range right {
		locations[location] = true
	}
	for location := range locations {
		leftValue, leftOK := left[location]
		rightValue, rightOK := right[location]
		switch {
		case leftOK && rightOK && ps6101SameValue(leftValue, rightValue):
			merged[location] = ps6101CloneValue(leftValue)
		case leftOK && ps6101RiskValue(leftValue) && (!rightOK || !ps6101RiskValue(rightValue)):
			merged[location] = ps6101CloneValue(leftValue)
		case rightOK && ps6101RiskValue(rightValue) && (!leftOK || !ps6101RiskValue(leftValue)):
			merged[location] = ps6101CloneValue(rightValue)
		case leftOK && rightOK && ps6101SameRisk(leftValue, rightValue):
			merged[location] = ps6101CloneValue(leftValue)
		}
	}
	return merged
}

func ps6101RiskValue(value ps6101Value) bool {
	return value.kind == ps6101Symmetric && value.eligible && len(value.sources) > 0
}

func ps6101SameRisk(left, right ps6101Value) bool {
	return ps6101RiskValue(left) && ps6101RiskValue(right) && left.aggregate == right.aggregate &&
		left.identity == right.identity && ps6101SameDynamicType(left.analysisValue().dynamic, right.analysisValue().dynamic) &&
		ps6101SourceSignature(left.sources) == ps6101SourceSignature(right.sources)
}

func (engine *ps6101Engine) callerVisibility() (map[types.Object]bool, map[types.Object]bool) {
	roots := make(map[types.Object]bool, len(engine.state)+len(engine.counterRevs))
	for location := range engine.state {
		roots[location.root] = true
	}
	for location := range engine.counterRevs {
		roots[location.root] = true
	}
	aliases := make(map[types.Object]bool, len(engine.aliases))
	for object := range engine.aliases {
		aliases[object] = true
	}
	return roots, aliases
}

func (engine *ps6101Engine) retainCallerVisibility(roots, aliases map[types.Object]bool) {
	visible := func(object types.Object) bool {
		return roots[object] || ps6101PackageObject(object)
	}
	for location := range engine.state {
		if !visible(location.root) {
			delete(engine.state, location)
		}
	}
	for location := range engine.counterGates {
		if !visible(location.root) {
			delete(engine.counterGates, location)
		}
	}
	for location := range engine.counterRevs {
		if !visible(location.root) {
			delete(engine.counterRevs, location)
		}
	}
	for object := range engine.aliases {
		if !aliases[object] && !ps6101PackageObject(object) {
			delete(engine.aliases, object)
		}
	}
}

func ps6101PackageObject(object types.Object) bool {
	return object != nil && object.Pkg() != nil && object.Parent() == object.Pkg().Scope()
}

func (engine *ps6101Engine) assign(statement *ast.AssignStmt) {
	if statement == nil || len(statement.Lhs) == 0 || len(statement.Rhs) == 0 {
		return
	}
	targets := make([]ps6101StoreTarget, len(statement.Lhs))
	for index, left := range statement.Lhs {
		targets[index] = engine.prepareStoreTarget(left)
	}
	values := engine.evalAssignmentRHS(statement.Rhs, len(statement.Lhs))
	for index, left := range statement.Lhs {
		if index >= len(values) {
			break
		}
		right := statement.Rhs[min(index, len(statement.Rhs)-1)]
		if statement.Tok == token.ASSIGN || statement.Tok == token.DEFINE {
			var sourceType types.Type
			if len(statement.Rhs) != 1 || len(statement.Lhs) == 1 {
				sourceType = engine.pass.TypesInfo.TypeOf(right)
			}
			values[index] = engine.assignmentValue(engine.pass.TypesInfo.TypeOf(left), sourceType, values[index])
		}
		prior, priorOK := engine.counterEvidence(left)
		positiveIncrement := statement.Tok == token.ADD_ASSIGN && len(statement.Rhs) == 1 && ps6101StrictPositiveConstant(engine.pass, statement.Rhs[0]) ||
			(statement.Tok == token.ASSIGN && engine.counterAddAssignment(left, right))
		zeroInitialization := (statement.Tok == token.ASSIGN || statement.Tok == token.DEFINE) && ps6101ZeroConstant(engine.pass, right)
		if (statement.Tok == token.ASSIGN || statement.Tok == token.DEFINE) && ps6101ReferenceLike(engine.pass.TypesInfo.TypeOf(right)) {
			if source, ok := engine.location(right, true); ok && values[index].reference == nil {
				// eval already records the direct pointee for pointer values.
				// A type assertion's followed location may be several pointer
				// levels deeper, so use it only as a fallback for values whose
				// direct reference is otherwise unavailable.
				values[index].reference = ps6101Reference(source)
			} else if destination, ok := engine.location(left, false); ok && values[index].reference == nil {
				values[index].reference = ps6101Reference(destination)
			}
		}
		engine.storeTarget(left, targets[index], values[index], statement.Tok)
		if zeroInitialization {
			engine.recordCounterZero(left)
		} else if positiveIncrement && priorOK {
			engine.recordCounterIncrement(left, prior)
		}
		if statement.Tok == token.ASSIGN || statement.Tok == token.DEFINE {
			if composite := ps6101CompositeLiteral(right); composite != nil {
				if destination, ok := engine.location(left, false); ok {
					engine.finalizeCompositeFields(destination, composite)
					engine.seedImplicitCounterZeros(destination.root)
				}
			}
			if source, ok := engine.location(right, true); ok {
				if destination, ok := engine.location(left, false); ok {
					engine.copyPrefix(source, destination)
					if ps6101PackageObject(destination.root) && ps6101ContainsReference(engine.pass.TypesInfo.TypeOf(right)) {
						engine.invalidateLocationAndReferences(source)
					}
				}
				if identifier, ok := ps6101Unparen(left).(*ast.Ident); ok {
					if object := identObject(engine.pass, identifier); object != nil && !ps6101PackageObject(object) && ps6101ReferenceLike(object.Type()) {
						if values[index].reference != nil {
							engine.aliases[object] = *values[index].reference
						} else {
							engine.aliases[object] = source
						}
					}
				}
			}
		}
	}
}

// finalizeCompositeFields adds destination-dependent backing references after
// eval has already evaluated every literal operand exactly once.
func (engine *ps6101Engine) finalizeCompositeFields(destination ps6101Location, composite *ast.CompositeLit) {
	parent := engine.state[destination]
	for index, element := range composite.Elts {
		segment, name := engine.compositeSegment(composite, element, index)
		expression := ast.Expr(element)
		if keyed, ok := element.(*ast.KeyValueExpr); ok {
			expression = keyed.Value
		}
		if segment == "" {
			continue
		}
		location := ps6101AppendLocation(destination, segment)
		value, present := engine.state[location]
		if !present {
			continue
		}
		if ps6101ReferenceLike(engine.pass.TypesInfo.TypeOf(expression)) {
			if value.reference == nil && ps6101BackingReference(engine.pass.TypesInfo.TypeOf(expression)) {
				value.reference = ps6101Reference(location)
			}
		}
		value.eligible = value.eligible || parent.eligible
		value.aggregate = value.aggregate || parent.aggregate
		if ps6101BenchmarkInputName(name) && len(value.sources) > 0 {
			value.eligible = true
		}
		if ps6101ThresholdName(name) {
			value.threshold = true
		}
		engine.state[location] = ps6101CloneValue(value)
		if nested := ps6101CompositeLiteral(expression); nested != nil {
			engine.finalizeCompositeFields(location, nested)
		}
	}
}

func ps6101BackingReference(typ types.Type) bool {
	if typ == nil {
		return false
	}
	switch types.Unalias(typ).Underlying().(type) {
	case *types.Slice, *types.Map:
		return true
	}
	return false
}

func (engine *ps6101Engine) indexCounterLocations(root ast.Node) {
	engine.counterIndex = make(map[types.Object][]ps6101Location)
	seen := make(map[ps6101Location]bool)
	ast.Inspect(root, func(node ast.Node) bool {
		expression, ok := node.(ast.Expr)
		if !ok || !ps6101BranchCounterName(ps6101ExpressionName(expression)) ||
			!ps6101NumericType(engine.pass.TypesInfo.TypeOf(expression)) {
			return true
		}
		location, ok := engine.location(expression, false)
		if !ok || location.root == nil || seen[location] {
			return true
		}
		seen[location] = true
		engine.counterIndex[location.root] = append(engine.counterIndex[location.root], location)
		return true
	})
}

func (engine *ps6101Engine) seedImplicitCounterZeros(root types.Object) {
	for _, location := range engine.counterIndex[root] {
		if value, present := engine.state[location]; present && value.kind != ps6101Zero {
			continue
		}
		engine.counterGates[location] = ps6101CounterEvidence{revision: engine.counterRevs[location]}
	}
}

func (engine *ps6101Engine) compositeSegment(composite *ast.CompositeLit, element ast.Expr, index int) (string, string) {
	if keyed, ok := element.(*ast.KeyValueExpr); ok {
		if identifier, ok := ps6101Unparen(keyed.Key).(*ast.Ident); ok {
			field := identifier.Name
			if object := identObject(engine.pass, identifier); object != nil && object.Pkg() != nil {
				field = object.Pkg().Path() + "." + object.Name()
			}
			return "." + field, identifier.Name
		}
		if value := engine.pass.TypesInfo.Types[keyed.Key].Value; value != nil {
			return "[" + value.ExactString() + "]", ""
		}
		return "", ""
	}
	typ := types.Unalias(engine.pass.TypesInfo.TypeOf(composite))
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = types.Unalias(pointer.Elem())
	}
	if named, ok := typ.(*types.Named); ok {
		typ = named.Underlying()
	}
	if structure, ok := typ.(*types.Struct); ok && index < structure.NumFields() {
		field := structure.Field(index)
		name := field.Name()
		if field.Pkg() != nil {
			name = field.Pkg().Path() + "." + name
		}
		return "." + name, field.Name()
	}
	return "[" + strconv.Itoa(index) + "]", ""
}

func (engine *ps6101Engine) compositeElementType(composite *ast.CompositeLit, element ast.Expr, index int) types.Type {
	if composite == nil {
		return nil
	}
	typ := engine.pass.TypesInfo.TypeOf(composite)
	if typ == nil {
		return nil
	}
	typ = types.Unalias(typ)
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = types.Unalias(pointer.Elem())
	}
	if named, ok := typ.(*types.Named); ok {
		typ = named.Underlying()
	}
	switch value := typ.Underlying().(type) {
	case *types.Struct:
		if keyed, ok := element.(*ast.KeyValueExpr); ok {
			if identifier, ok := ps6101Unparen(keyed.Key).(*ast.Ident); ok {
				if field, ok := identObject(engine.pass, identifier).(*types.Var); ok && field.IsField() {
					return field.Type()
				}
			}
		}
		if index >= 0 && index < value.NumFields() {
			return value.Field(index).Type()
		}
	case *types.Array:
		return value.Elem()
	case *types.Slice:
		return value.Elem()
	case *types.Map:
		return value.Elem()
	}
	return nil
}

func ps6101AppendLocation(location ps6101Location, segment string) ps6101Location {
	if strings.HasPrefix(segment, ".") && location.path == "" {
		segment = strings.TrimPrefix(segment, ".")
	}
	location.path += segment
	return location
}

func (engine *ps6101Engine) assignValues(names []*ast.Ident, expressions []ast.Expr) {
	if len(expressions) == 0 {
		for _, name := range names {
			value := ps6101ImplicitZeroValue(engine.pass.TypesInfo.TypeOf(name))
			engine.store(name, value, token.ASSIGN)
			if value.kind == ps6101Zero {
				engine.recordCounterZero(name)
			}
			engine.seedImplicitCounterZeros(identObject(engine.pass, name))
		}
		return
	}
	values := engine.evalAssignmentRHS(expressions, len(names))
	for index, name := range names {
		if index < len(values) {
			var expression ast.Expr
			if len(expressions) != 1 || len(names) == 1 {
				expression = expressions[min(index, len(expressions)-1)]
			}
			var sourceType types.Type
			if expression != nil {
				sourceType = engine.pass.TypesInfo.TypeOf(expression)
			}
			values[index] = engine.assignmentValue(engine.pass.TypesInfo.TypeOf(name), sourceType, values[index])
			if values[index].reference == nil && ps6101ReferenceLike(engine.pass.TypesInfo.TypeOf(name)) {
				if destination, ok := engine.location(name, false); ok {
					values[index].reference = ps6101Reference(destination)
				}
			}
			engine.store(name, values[index], token.ASSIGN)
			if composite := ps6101CompositeLiteral(expression); composite != nil {
				if destination, ok := engine.location(name, false); ok {
					engine.finalizeCompositeFields(destination, composite)
					engine.seedImplicitCounterZeros(destination.root)
				}
			}
			if source, ok := engine.location(expression, true); ok {
				if destination, ok := engine.location(name, false); ok {
					engine.copyPrefix(source, destination)
					if ps6101PackageObject(destination.root) && ps6101ContainsReference(engine.pass.TypesInfo.TypeOf(expression)) {
						engine.invalidateLocationAndReferences(source)
					}
				}
			}
			// Declaration initializers bind pointer values exactly like short
			// declarations. Preserve the direct pointee so each explicit * advances
			// one location instead of collapsing a deep chain to its final scalar.
			if object := identObject(engine.pass, name); object != nil && !ps6101PackageObject(object) &&
				ps6101ReferenceLike(object.Type()) && values[index].reference != nil {
				engine.aliases[object] = *values[index].reference
			}
			if expression != nil && ps6101ZeroConstant(engine.pass, expression) {
				engine.recordCounterZero(name)
			}
		}
	}
}

func ps6101ImplicitZeroValue(typ types.Type) ps6101Value {
	if ps6101NumericType(typ) {
		return ps6101ConstantValue(constant.MakeInt64(0))
	}
	result := ps6101Value{}
	if typ == nil {
		return result
	}
	switch value := types.Unalias(typ).Underlying().(type) {
	case *types.Array:
		result.kind = ps6101Zero
		result.length, result.capacity = value.Len(), value.Len()
		result.lengthOK, result.capacityOK = true, true
		result.offsetOK = true
		result.nonempty = value.Len() > 0
	case *types.Slice:
		result.lengthOK, result.capacityOK = true, true
		result.offsetOK = true
	case *types.Map:
		result.lengthOK = true
	}
	return result
}

func (engine *ps6101Engine) evalAssignmentRHS(expressions []ast.Expr, wanted int) []ps6101Value {
	if len(expressions) == 1 {
		switch expression := ps6101Unparen(expressions[0]).(type) {
		case *ast.CallExpr:
			if values := engine.evalCall(expression); len(values) > 0 {
				return values
			}
		case *ast.TypeAssertExpr:
			if wanted == 2 {
				value, ok := engine.evalTypeAssertion(expression, false)
				return []ps6101Value{value, ok}
			}
		}
	}
	values := make([]ps6101Value, 0, wanted)
	for _, expression := range expressions {
		values = append(values, engine.eval(expression))
	}
	return values
}

func (engine *ps6101Engine) prepareStoreTarget(expression ast.Expr) ps6101StoreTarget {
	restore := engine.beginSingleEvaluation()
	engine.eval(expression)
	target := engine.resolveStoreTarget(expression)
	restore()
	return target
}

func (engine *ps6101Engine) resolveStoreTarget(expression ast.Expr) ps6101StoreTarget {
	var target ps6101StoreTarget
	if pointer, indirect := ps6101Unparen(expression).(*ast.StarExpr); indirect {
		targets := engine.possibleReferenceTargets(pointer.X)
		switch len(targets) {
		case 0:
			target.untrackedIndirect = true
		case 1:
			target.location, target.valid = targets[0], true
		default:
			target.ambiguous = targets
		}
	} else {
		target.location, target.valid = engine.location(expression, false)
	}
	if !target.valid || target.location.root == nil {
		return target
	}
	indexExpression, isIndexed := ps6101Unparen(expression).(*ast.IndexExpr)
	target.isIndexed = isIndexed
	if target.isIndexed {
		target.dynamicIndex = engine.indexConstant(indexExpression.Index) == nil
		target.indexedParent, _ = engine.location(indexExpression.X, true)
		if typ := engine.pass.TypesInfo.TypeOf(indexExpression.X); typ != nil {
			_, target.mapIndex = types.Unalias(typ).Underlying().(*types.Map)
		}
	}
	return target
}

func (engine *ps6101Engine) store(expression ast.Expr, value ps6101Value, operation token.Token) {
	engine.storeTarget(expression, engine.resolveStoreTarget(expression), value, operation)
}

func (engine *ps6101Engine) storeTarget(expression ast.Expr, target ps6101StoreTarget, value ps6101Value, operation token.Token) {
	if target.untrackedIndirect {
		// The write escapes through an untracked pointer. It can still reach
		// any captured benchmark input, so retaining provenance would create a
		// false positive.
		engine.invalidateCapturedState()
		return
	}
	if len(target.ambiguous) > 0 {
		// A dynamic selection may point at any tracked element. Invalidate
		// every possible pointee instead of choosing one arbitrarily.
		for _, location := range target.ambiguous {
			engine.invalidateLocationAndReferences(location)
		}
		return
	}
	if !target.valid || target.location.root == nil {
		return
	}
	location := target.location
	if target.dynamicIndex && operation == token.ASSIGN {
		// An unknown index may replace any one element, but not the entire
		// collection. Retain only a conservative collection summary and drop
		// exact per-index facts below when killPrefix runs.
		value = ps6101CollectionMerge(engine.state[location], value)
	}
	if operation == token.ADD_ASSIGN || operation == token.SUB_ASSIGN {
		old, present := engine.state[location]
		if !present {
			if root, ok := engine.state[ps6101Location{root: location.root}]; ok && root.kind == ps6101Zero {
				old.kind = ps6101Zero
			}
		}
		if operation == token.ADD_ASSIGN {
			value = ps6101Add(old, value)
		} else {
			value = ps6101Subtract(old, value)
		}
	}
	mapElementPresent := false
	mapLength, mapLengthOK := int64(0), false
	if target.isIndexed {
		value.nonempty = value.nonempty || engine.state[target.indexedParent].nonempty
		if target.mapIndex {
			_, mapElementPresent = engine.state[location]
			parent := engine.state[target.indexedParent]
			mapLength, mapLengthOK = parent.length, parent.lengthOK
		}
	}
	ps6101ApplyNamedValueSemantics(ps6101ExpressionName(expression), &value)
	elements := value.elements
	fields := value.takeFields()
	value.elements = nil
	engine.killPrefix(location)
	engine.nextRevision++
	value.revision = engine.nextRevision
	if value.identity == 0 {
		value.identity = engine.nextRevision
	}
	if value.analysisValue().squareID == 0 {
		analysis := value.mutableAnalysis()
		analysis.squareID, analysis.squareSign = engine.nextRevision, 1
	}
	engine.state[location] = ps6101CloneValue(value)
	for index, element := range elements {
		child := location
		child.path += "[" + strconv.FormatInt(index, 10) + "]"
		element.elements = nil
		element.eligible = element.eligible || value.eligible
		element.aggregate = element.aggregate || value.aggregate
		engine.nextRevision++
		element.revision = engine.nextRevision
		if element.identity == 0 {
			element.identity = engine.nextRevision
		}
		if element.analysisValue().squareID == 0 {
			analysis := element.mutableAnalysis()
			analysis.squareID, analysis.squareSign = engine.nextRevision, 1
		}
		engine.state[child] = ps6101CloneValue(element)
	}
	engine.storeValueFields(location, fields)
	engine.bumpCounterRevision(location)
	engine.deleteCounterEvidence(location)
	if target.indexedParent.root != nil && target.indexedParent != location {
		engine.recomputeIndexedValue(target.indexedParent)
		if target.mapIndex {
			parent := engine.state[target.indexedParent]
			if mapLengthOK && !target.dynamicIndex && !mapElementPresent {
				parent.length, parent.lengthOK = mapLength+1, true
				parent.nonempty = true
			} else {
				parent.lengthOK = mapLengthOK && !target.dynamicIndex && mapElementPresent
				if parent.lengthOK {
					parent.length = mapLength
				}
			}
			engine.state[target.indexedParent] = parent
		}
	}
	if identifier, ok := ps6101Unparen(expression).(*ast.Ident); ok {
		delete(engine.aliases, identObject(engine.pass, identifier))
	}
}

// ps6101ApplyNamedValueSemantics keeps the vocabulary-driven facts attached
// to every binding form. Type-switch variables are implicit go/types objects,
// so they cannot pass through store with their own source identifier; applying
// the same name rules explicitly prevents the switch guard from being the
// first "weights" binding that silently loses eligibility.
func ps6101ApplyNamedValueSemantics(name string, value *ps6101Value) {
	if value == nil {
		return
	}
	if ps6101BenchmarkInputName(name) && len(value.sources) > 0 {
		value.eligible = true
	}
	if ps6101AggregateName(name) && value.eligible && len(value.sources) > 0 {
		value.aggregate = true
	}
	if ps6101ThresholdName(name) {
		value.threshold = true
	}
}

func (engine *ps6101Engine) storeValueFields(destination ps6101Location, fields map[string]ps6101Value) {
	parent := engine.state[destination]
	for suffix, field := range fields {
		if suffix == "" {
			continue
		}
		location := destination
		if location.path == "" {
			location.path = strings.TrimPrefix(suffix, ".")
		} else if strings.HasPrefix(suffix, "[") {
			location.path += suffix
		} else {
			location.path += "." + strings.TrimPrefix(suffix, ".")
		}
		field.takeFields()
		field.eligible = field.eligible || parent.eligible
		field.aggregate = field.aggregate || parent.aggregate
		engine.nextRevision++
		field.revision = engine.nextRevision
		if field.identity == 0 {
			field.identity = engine.nextRevision
		}
		if field.analysisValue().squareID == 0 {
			analysis := field.mutableAnalysis()
			analysis.squareID, analysis.squareSign = engine.nextRevision, 1
		}
		engine.state[location] = ps6101CloneValue(field)
	}
}

func (engine *ps6101Engine) possibleReferenceTargets(expression ast.Expr) []ps6101Location {
	expression = ps6101Unparen(expression)
	if unary, ok := expression.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		return engine.storageTargets(unary.X)
	}
	if indirect, ok := expression.(*ast.StarExpr); ok {
		return engine.referenceTargets(engine.possibleReferenceTargets(indirect.X))
	}
	if reference, ok := engine.directEvaluatedReference(expression); ok {
		// eval has already resolved the direct pointee of this pointer value.
		// This is especially important for a pointer field selected from a
		// copied call-result aggregate: the selector has no addressable owner
		// slot, so its cached reference is the pointee, not another pointer
		// location that referenceTargets should advance through.
		return []ps6101Location{reference}
	}
	raw, ok := engine.location(expression, false)
	if !ok || raw.root == nil {
		return nil
	}
	targets := engine.referenceTargets([]ps6101Location{raw})
	if len(targets) == 0 {
		if identifier, ok := expression.(*ast.Ident); ok {
			if alias, present := engine.aliases[identObject(engine.pass, identifier)]; present && alias != raw {
				return []ps6101Location{alias}
			}
		}
	}
	return targets
}

func (engine *ps6101Engine) directEvaluatedReference(expression ast.Expr) (ps6101Location, bool) {
	expression = ps6101Unparen(expression)
	cached, present := engine.evalCache[expression]
	if !present || cached.reference == nil {
		return ps6101Location{}, false
	}
	switch value := expression.(type) {
	case *ast.SelectorExpr:
		// An addressable selector still denotes its pointer owner slot. Only a
		// selector whose base has no location (notably a copied aggregate call
		// result) needs to expose the selected value's cached direct pointee.
		if _, ok := engine.location(value.X, true); ok {
			return ps6101Location{}, false
		}
	case *ast.TypeAssertExpr:
		if _, ok := engine.directEvaluatedReference(value.X); !ok {
			return ps6101Location{}, false
		}
	case *ast.CallExpr:
		// A conversion keeps the owner/location rules of its operand. In
		// particular, unsafe.Pointer conversions must remain conservative.
		if typed := engine.pass.TypesInfo.Types[value.Fun]; typed.IsType() {
			return ps6101Location{}, false
		}
	default:
		return ps6101Location{}, false
	}
	return *cached.reference, true
}

// storageTargets resolves the storage denoted by an lvalue without following
// the value stored there. Each explicit * consumes exactly one pointer level;
// this distinction matters for writes through ***T and deeper chains.
func (engine *ps6101Engine) storageTargets(expression ast.Expr) []ps6101Location {
	expression = ps6101Unparen(expression)
	if indirect, ok := expression.(*ast.StarExpr); ok {
		return engine.possibleReferenceTargets(indirect.X)
	}
	location, ok := engine.location(expression, false)
	if !ok || location.root == nil {
		return nil
	}
	return []ps6101Location{location}
}

// referenceTargets follows one direct reference from every possible owner.
// It deliberately does not call followReference: doing so collapses **T and
// ***T to the final T storage before the remaining explicit stars are seen.
func (engine *ps6101Engine) referenceTargets(owners []ps6101Location) []ps6101Location {
	seen := make(map[ps6101Location]bool)
	add := func(target ps6101Location, owner ps6101Location) {
		if target.root != nil && target != owner {
			seen[target] = true
		}
	}
	for _, owner := range owners {
		if value, present := engine.state[owner]; present && value.reference != nil && *value.reference != owner {
			add(*value.reference, owner)
			continue
		}
		for location, value := range engine.state {
			if ps6101HasLocationPrefix(location, owner) && value.reference != nil {
				add(*value.reference, location)
			}
		}
	}
	targets := make([]ps6101Location, 0, len(seen))
	for target := range seen {
		targets = append(targets, target)
	}
	slices.SortFunc(targets, func(left, right ps6101Location) int {
		if left.root.String() != right.root.String() {
			return strings.Compare(left.root.String(), right.root.String())
		}
		return strings.Compare(left.path, right.path)
	})
	return targets
}

func (engine *ps6101Engine) recomputeIndexedValue(parent ps6101Location) {
	prefix := parent.path + "["
	var combined ps6101Value
	found := false
	for location, value := range engine.state {
		if location.root != parent.root || !strings.HasPrefix(location.path, prefix) {
			continue
		}
		remainder := strings.TrimPrefix(location.path, prefix)
		closing := strings.IndexByte(remainder, ']')
		if closing < 0 || closing != len(remainder)-1 {
			continue
		}
		combined = ps6101CollectionMerge(combined, value)
		found = true
	}
	if !found {
		return
	}
	old := engine.state[parent]
	combined.eligible = combined.eligible || old.eligible
	combined.aggregate = combined.aggregate || old.aggregate
	combined.threshold = combined.threshold || old.threshold
	combined.testing = combined.testing || old.testing
	combined.nonempty = combined.nonempty || old.nonempty
	if old.reference != nil {
		// Reference-bearing collection values keep pointing at the same
		// backing storage when one of their exact elements is recomputed.
		// Without this, a by-value struct parameter containing a slice loses
		// the caller-visible backing-array link after the first element write.
		combined.reference = old.reference
	}
	if old.lengthOK {
		combined.length, combined.lengthOK = old.length, true
	}
	if old.capacityOK {
		combined.capacity, combined.capacityOK = old.capacity, true
	}
	if old.offsetOK {
		combined.offset, combined.offsetOK = old.offset, true
	}
	engine.nextRevision++
	combined.revision = engine.nextRevision
	combined.identity = engine.nextRevision
	combinedAnalysis := combined.mutableAnalysis()
	combinedAnalysis.squareID, combinedAnalysis.squareSign, combinedAnalysis.squareSig = engine.nextRevision, 1, ""
	engine.state[parent] = ps6101CloneValue(combined)
	engine.bumpCounterRevision(parent)
}

func (engine *ps6101Engine) bumpCounterRevision(prefix ps6101Location) {
	seen := false
	for location := range engine.counterRevs {
		if ps6101HasLocationPrefix(location, prefix) {
			engine.counterRevs[location]++
			seen = seen || location == prefix
		}
	}
	if !seen {
		engine.counterRevs[prefix]++
	}
}

func (engine *ps6101Engine) increment(statement *ast.IncDecStmt) {
	target := engine.prepareStoreTarget(statement.X)
	if target.untrackedIndirect {
		engine.invalidateCapturedState()
		return
	}
	if len(target.ambiguous) > 0 {
		for _, location := range target.ambiguous {
			engine.invalidateLocationAndReferences(location)
		}
		return
	}
	if !target.valid {
		return
	}
	location := target.location
	prior, priorOK := engine.counterEvidence(statement.X)
	value := engine.state[location]
	if value.constant != nil && (value.constant.Kind() == constant.Int || value.constant.Kind() == constant.Float) {
		operation := token.ADD
		if statement.Tok == token.DEC {
			operation = token.SUB
		}
		constantValue := constant.BinaryOp(value.constant, operation, constant.MakeInt64(1))
		next := ps6101ConstantValue(constantValue)
		next.eligible, next.aggregate, next.threshold, next.testing = value.eligible, value.aggregate, value.threshold, value.testing
		value = next
	} else {
		value.constant = nil
		analysis := value.mutableAnalysis()
		analysis.lower, analysis.upper = nil, nil
	}
	engine.nextRevision++
	value.revision = engine.nextRevision
	value.identity = engine.nextRevision
	analysis := value.mutableAnalysis()
	analysis.squareID, analysis.squareSign, analysis.squareSig = engine.nextRevision, 1, ""
	engine.state[location] = value
	engine.counterRevs[location]++
	engine.deleteCounterEvidence(location)
	if statement.Tok == token.INC && priorOK {
		engine.recordCounterIncrement(statement.X, prior)
	}
}

func (engine *ps6101Engine) recordCounterZero(expression ast.Expr) {
	location, ok := engine.counterLocation(expression)
	if !ok || !ps6101BranchCounterLocation(location) {
		return
	}
	engine.counterGates[location] = ps6101CounterEvidence{revision: engine.counterRevs[location]}
}

func (engine *ps6101Engine) recordCounterIncrement(expression ast.Expr, prior ps6101CounterEvidence) {
	if len(engine.activeGates) == 0 {
		return
	}
	location, ok := engine.counterLocation(expression)
	if !ok || !ps6101BranchCounterLocation(location) {
		return
	}
	gates := make(map[ps6101GateKey]bool, len(engine.activeGates))
	sites := make(map[token.Pos]bool, len(engine.activeGates))
	for _, gate := range engine.activeGates {
		gates[gate.key] = true
		sites[gate.pos] = true
	}
	if prior.incremented {
		if !ps6101SameCounterSites(prior.sites, sites) {
			return
		}
		for gate := range prior.gates {
			gates[gate] = true
		}
	}
	engine.counterGates[location] = ps6101CounterEvidence{
		gates: gates, sites: sites, revision: engine.counterRevs[location], incremented: true,
	}
}

func (engine *ps6101Engine) counterEvidence(expression ast.Expr) (ps6101CounterEvidence, bool) {
	location, ok := engine.counterLocation(expression)
	if !ok {
		return ps6101CounterEvidence{}, false
	}
	evidence, ok := engine.counterGates[location]
	if ok && evidence.revision == engine.counterRevs[location] {
		return ps6101CloneCounterEvidence(evidence), true
	}
	// Counter indexing happens before benchmark execution. If an index is a
	// runtime constant, the one-time index conservatively records the aggregate
	// root; refine that implicit-zero evidence when execution resolves the exact
	// element.
	bestLength := -1
	for candidate, candidateEvidence := range engine.counterGates {
		if !ps6101HasLocationPrefix(location, candidate) || candidateEvidence.revision != engine.counterRevs[candidate] {
			continue
		}
		if len(candidate.path) > bestLength {
			bestLength = len(candidate.path)
			evidence, ok = candidateEvidence, true
		}
	}
	return ps6101CloneCounterEvidence(evidence), ok
}

func (engine *ps6101Engine) counterLocation(expression ast.Expr) (ps6101Location, bool) {
	location, ok := engine.location(expression, true)
	if !ok || location.root == nil {
		return ps6101Location{}, false
	}
	best := location
	bestLength := -1
	for _, indexed := range engine.counterIndex[location.root] {
		if !ps6101HasLocationPrefix(location, indexed) {
			continue
		}
		if indexed == location {
			return indexed, true
		}
		if len(indexed.path) > bestLength {
			best, bestLength = indexed, len(indexed.path)
		}
	}
	return best, true
}

func (engine *ps6101Engine) counterAddAssignment(left, right ast.Expr) bool {
	binary, ok := ps6101Unparen(right).(*ast.BinaryExpr)
	if !ok || binary.Op != token.ADD {
		return false
	}
	target, ok := engine.counterLocation(left)
	if !ok {
		return false
	}
	for _, pair := range [][2]ast.Expr{{binary.X, binary.Y}, {binary.Y, binary.X}} {
		if !ps6101StrictPositiveConstant(engine.pass, pair[1]) {
			continue
		}
		if source, ok := engine.counterLocation(pair[0]); ok && source == target {
			return true
		}
	}
	return false
}

func (engine *ps6101Engine) deleteCounterEvidence(prefix ps6101Location) {
	for location := range engine.counterGates {
		if ps6101HasLocationPrefix(location, prefix) || ps6101HasLocationPrefix(prefix, location) {
			delete(engine.counterGates, location)
		}
	}
}

func ps6101NumericType(typ types.Type) bool {
	kinds, ok := ps6101BasicKinds(typ, make(map[types.Type]bool))
	if !ok || kinds == 0 {
		return false
	}
	for kind, basic := range types.Typ {
		if kinds&(1<<uint(kind)) != 0 && (basic == nil || basic.Info()&types.IsNumeric == 0) {
			return false
		}
	}
	return true
}

// ps6101BasicKinds expands the finite basic-type terms of a type parameter's
// constraint. Interface embeddings are intersections and union terms are
// alternatives; method-only/comparable embeddings can further restrict a
// numeric set but cannot make it nonnumeric.
func ps6101BasicKinds(typ types.Type, seen map[types.Type]bool) (uint64, bool) {
	if typ == nil {
		return 0, false
	}
	typ = types.Unalias(typ)
	if seen[typ] {
		return 0, false
	}
	seen[typ] = true
	defer delete(seen, typ)
	switch value := typ.(type) {
	case *types.Basic:
		return 1 << uint(value.Kind()), true
	case *types.Named:
		return ps6101BasicKinds(value.Underlying(), seen)
	case *types.TypeParam:
		return ps6101BasicKinds(value.Constraint(), seen)
	case *types.Union:
		var result uint64
		for index := 0; index < value.Len(); index++ {
			kinds, ok := ps6101BasicKinds(value.Term(index).Type(), seen)
			if !ok {
				return 0, false
			}
			result |= kinds
		}
		return result, result != 0
	case *types.Interface:
		value = value.Complete()
		var result uint64
		found := false
		for index := 0; index < value.NumEmbeddeds(); index++ {
			kinds, ok := ps6101BasicKinds(value.EmbeddedType(index), seen)
			if !ok {
				continue
			}
			if !found {
				result, found = kinds, true
				continue
			}
			result &= kinds
		}
		return result, found && result != 0
	}
	return 0, false
}

func ps6101BranchCounterLocation(location ps6101Location) bool {
	return location.root != nil && (ps6101BranchCounterName(location.root.Name()) || ps6101BranchCounterName(location.path))
}

func ps6101SameCounterSites(left, right map[token.Pos]bool) bool {
	return maps.Equal(left, right)
}

func ps6101CompositeLiteral(expression ast.Expr) *ast.CompositeLit {
	switch value := ps6101Unparen(expression).(type) {
	case *ast.CompositeLit:
		return value
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return ps6101CompositeLiteral(value.X)
		}
	}
	return nil
}

func (engine *ps6101Engine) eval(expression ast.Expr) ps6101Value {
	if expression == nil {
		return ps6101Value{}
	}
	expression = ps6101Unparen(expression)
	if cached, ok := engine.evalCache[expression]; ok {
		return ps6101CloneValue(cached)
	}
	result := engine.evalUncached(expression)
	if engine.evalCache != nil {
		engine.evalCache[expression] = ps6101CloneValue(result)
	}
	return result
}

func (engine *ps6101Engine) evalUncached(expression ast.Expr) ps6101Value {
	if expression == nil {
		return ps6101Value{}
	}
	if typed := engine.pass.TypesInfo.Types[expression].Value; typed != nil {
		return ps6101ConstantValue(typed)
	}
	// Location lookup itself is intentionally side-effect free. Evaluate every
	// runtime operand first and retain its value so fallbacks never execute a
	// side-effectful base expression twice.
	var baseValue ps6101Value
	hasBaseValue := false
	switch value := expression.(type) {
	case *ast.IndexExpr:
		if base, instantiated := engine.instantiationBase(value); instantiated {
			return engine.instantiatedValue(engine.eval(base), value)
		}
		baseValue, hasBaseValue = engine.eval(value.X), true
		engine.eval(value.Index)
	case *ast.IndexListExpr:
		// Type arguments have no runtime evaluation. The instantiated
		// function value keeps both callable provenance and its specialization.
		return engine.instantiatedValue(engine.eval(value.X), value)
	case *ast.SelectorExpr:
		baseValue, hasBaseValue = engine.eval(value.X), true
	}
	switch value := expression.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.IndexExpr:
		if identifier, ok := value.(*ast.Ident); ok {
			if declaration := engine.functions[identObject(engine.pass, identifier)]; declaration != nil {
				return ps6101Value{callable: &ps6101Callable{function: declaration}}
			}
		}
		if selector, ok := value.(*ast.SelectorExpr); ok {
			if selection := engine.pass.TypesInfo.Selections[selector]; selection != nil {
				if method := ps6101TestingMethodName(selection); method != "" {
					callable := &ps6101Callable{testingMethod: method}
					switch selection.Kind() {
					case types.MethodVal:
						if !baseValue.testing {
							return ps6101Value{}
						}
						callable.receiver = selector.X
						engine.snapshotCallableReceiver(callable, selector.X, baseValue)
					case types.MethodExpr:
						callable.receiverFromFirst = true
					}
					return ps6101Value{callable: callable}
				}
				if declaration := engine.functionDeclaration(selection.Obj()); declaration != nil {
					switch selection.Kind() {
					case types.MethodVal:
						callable := &ps6101Callable{function: declaration, receiver: selector.X}
						engine.snapshotCallableReceiver(callable, selector.X, baseValue)
						return ps6101Value{callable: callable}
					case types.MethodExpr:
						return ps6101Value{callable: &ps6101Callable{function: declaration, receiverFromFirst: true}}
					}
				}
				if method, ok := selection.Obj().(*types.Func); ok && ps6101InterfaceType(selection.Recv()) {
					switch selection.Kind() {
					case types.MethodVal:
						if declaration := engine.dynamicMethodDeclaration(baseValue, method); declaration != nil {
							callable := &ps6101Callable{function: declaration, receiver: selector.X}
							engine.snapshotCallableReceiver(callable, selector.X, baseValue)
							return ps6101Value{callable: callable}
						}
					case types.MethodExpr:
						return ps6101Value{callable: &ps6101Callable{interfaceMethod: method, receiverFromFirst: true}}
					}
				}
			}
		}
		if selector, ok := value.(*ast.SelectorExpr); ok && engine.testingN(selector) {
			result := ps6101Value{kind: ps6101Positive}
			analysis := result.mutableAnalysis()
			analysis.lower, analysis.upper = constant.MakeInt64(1), constant.MakeInt64(1<<63-1)
			return result
		}
		if location, ok := engine.location(value, false); ok {
			if stored, present := engine.state[location]; present {
				typ := engine.pass.TypesInfo.TypeOf(value)
				if ps6101InterfaceType(typ) && stored.analysisValue().dynamic != nil {
					typ = stored.analysisValue().dynamic
				}
				return engine.valueWithStoredFields(location, ps6101CloneValue(stored), typ)
			}
		}
		if location, ok := engine.location(value, true); ok {
			if stored, ok := engine.state[location]; ok {
				typ := engine.pass.TypesInfo.TypeOf(value)
				if ps6101InterfaceType(typ) && stored.analysisValue().dynamic != nil {
					typ = stored.analysisValue().dynamic
				}
				return engine.valueWithStoredFields(location, ps6101CloneValue(stored), typ)
			}
		}
		if location, ok := engine.location(value, true); ok && ps6101ThresholdName(ps6101ExpressionName(value)) {
			return engine.materializeThreshold(location)
		}
		if selector, ok := value.(*ast.SelectorExpr); ok && hasBaseValue {
			if selected, ok := engine.selectValue(baseValue, selector); ok {
				return selected
			}
		}
		if indexed, ok := value.(*ast.IndexExpr); ok && hasBaseValue {
			if selected, ok := engine.selectIndexValue(baseValue, indexed); ok {
				return selected
			}
			return baseValue
		}
	case *ast.StarExpr:
		pointer := engine.eval(value.X)
		targets := engine.possibleReferenceTargets(value.X)
		if len(targets) == 0 && pointer.reference != nil {
			targets = append(targets, *pointer.reference)
		}
		var result ps6101Value
		found := false
		for _, target := range targets {
			stored, present := engine.state[target]
			if !present {
				// One possible pointee has been invalidated, so a dynamic
				// dereference no longer has must-provenance.
				return ps6101Value{}
			}
			if !found {
				result, found = ps6101CloneValue(stored), true
				continue
			}
			if !ps6101SameValue(result, stored) {
				return ps6101Value{}
			}
		}
		if found {
			// A pointer binding can be the first syntactic name that classifies
			// the referenced benchmark value (notably a type-switch or assertion
			// variable named weight). Keep that classification on the read value
			// without mutating the pointee: the binding is branch-scoped, while
			// the referenced storage may remain visible after the branch.
			result.eligible = result.eligible || pointer.eligible
			result.aggregate = result.aggregate || pointer.aggregate
			result.threshold = result.threshold || pointer.threshold
			result.testing = result.testing || pointer.testing
			return result
		}
		return ps6101Value{}
	case *ast.UnaryExpr:
		result := engine.eval(value.X)
		switch value.Op {
		case token.ADD:
		case token.AND:
			// Address-of targets the selected variable or element itself. In
			// particular, &pointer is **T and must not collapse through pointer's
			// existing *T alias to the final T pointee.
			if locations := engine.storageTargets(value.X); len(locations) == 1 {
				result.reference = ps6101Reference(locations[0])
			}
		case token.SUB:
			if result.analysisValue().squareID != 0 {
				analysis := result.mutableAnalysis()
				analysis.squareSign = -analysis.squareSign
			}
			result.identity = 0
		default:
			result.identity = 0
			analysis := result.mutableAnalysis()
			analysis.squareID, analysis.squareSign, analysis.squareSig = 0, 0, ""
		}
		return result
	case *ast.SliceExpr:
		result := engine.eval(value.X)
		engine.applySliceBounds(&result, value)
		result.identity = 0
		return result
	case *ast.TypeAssertExpr:
		result, _ := engine.evalTypeAssertion(value, true)
		return result
	case *ast.BinaryExpr:
		if bound, ok := ps6101CenteredRandomBound(engine.pass, value); ok {
			engine.eval(value.X)
			engine.eval(value.Y)
			return ps6101BoundedRandomValue(value.Pos(), bound)
		}
		if value.Op == token.LAND || value.Op == token.LOR {
			switch engine.boolConstant(value) {
			case 1:
				return ps6101ConstantValue(constant.MakeBool(true))
			case -1:
				return ps6101ConstantValue(constant.MakeBool(false))
			}
			return ps6101Value{}
		}
		left, right := engine.eval(value.X), engine.eval(value.Y)
		switch value.Op {
		case token.ADD:
			return ps6101Add(left, right)
		case token.SUB:
			return ps6101Subtract(left, right)
		case token.MUL:
			return ps6101Multiply(left, right)
		case token.QUO:
			return ps6101Divide(left, right)
		}
	case *ast.CallExpr:
		if results := engine.evalCall(value); len(results) > 0 {
			return results[0]
		}
	case *ast.CompositeLit:
		result := ps6101Value{kind: ps6101Zero, nonempty: len(value.Elts) > 0}
		fields := make(map[string]ps6101Value)
		captureFields, captureAllFields := false, false
		if typ := engine.pass.TypesInfo.TypeOf(value); typ != nil {
			switch types.Unalias(typ).Underlying().(type) {
			case *types.Struct, *types.Array, *types.Slice, *types.Map:
				captureFields, captureAllFields = true, true
			}
		}
		_, mapLiteral := types.Unalias(engine.pass.TypesInfo.TypeOf(value)).Underlying().(*types.Map)
		for index, element := range value.Elts {
			expression := ast.Expr(element)
			segment, name := engine.compositeSegment(value, element, index)
			switch item := element.(type) {
			case *ast.KeyValueExpr:
				if mapLiteral {
					engine.eval(item.Key)
				}
				expression = item.Value
			}
			elementType := engine.compositeElementType(value, element, index)
			evaluated := engine.assignmentValue(elementType, engine.pass.TypesInfo.TypeOf(expression), engine.eval(expression))
			if ps6101BenchmarkInputName(name) && len(evaluated.sources) > 0 {
				evaluated.eligible = true
			}
			if ps6101ThresholdName(name) {
				evaluated.threshold = true
			}
			result = ps6101CollectionMerge(result, evaluated)
			if !captureFields || !captureAllFields && !ps6101ContainsReference(engine.pass.TypesInfo.TypeOf(expression)) && !ps6101InterfaceType(elementType) {
				continue
			}
			segment = strings.TrimPrefix(segment, ".")
			if segment == "" {
				continue
			}
			children := evaluated.takeFields()
			fields[segment] = ps6101CloneValue(evaluated)
			for suffix, child := range children {
				separator := "."
				if strings.HasPrefix(suffix, "[") {
					separator = ""
				}
				fields[segment+separator+strings.TrimPrefix(suffix, ".")] = ps6101CloneValue(child)
			}
		}
		result.identity = 0
		result.reference = nil
		result.callable = nil
		engine.applyCompositeLength(&result, value)
		result.setFields(fields)
		return result
	case *ast.FuncLit:
		return ps6101Value{callable: &ps6101Callable{literal: value}}
	}
	return ps6101Value{}
}

func (engine *ps6101Engine) instantiatedValue(value ps6101Value, expression ast.Expr) ps6101Value {
	if value.callable == nil {
		return value
	}
	identifier := engine.instantiationIdentifier(expression)
	instance, ok := engine.pass.TypesInfo.Instances[identifier]
	if !ok || instance.TypeArgs == nil {
		return value
	}
	callable := *value.callable
	callable.typeArguments = maps.Clone(callable.typeArguments)
	if callable.typeArguments == nil {
		callable.typeArguments = make(map[*types.TypeParam]types.Type)
	}
	object, _ := identObject(engine.pass, identifier).(*types.Func)
	if object == nil {
		return value
	}
	signature, _ := object.Type().(*types.Signature)
	if signature == nil {
		return value
	}
	parameters := signature.TypeParams()
	if parameters != nil && parameters.Len() == instance.TypeArgs.Len() {
		for index := range parameters.Len() {
			callable.typeArguments[parameters.At(index)] = engine.resolveTypeArgument(instance.TypeArgs.At(index))
		}
	} else if receiver := signature.RecvTypeParams(); receiver != nil && receiver.Len() == instance.TypeArgs.Len() {
		for index := range receiver.Len() {
			callable.typeArguments[receiver.At(index)] = engine.resolveTypeArgument(instance.TypeArgs.At(index))
		}
	}
	value.callable = &callable
	return value
}

func (engine *ps6101Engine) selectValue(base ps6101Value, selector *ast.SelectorExpr) (ps6101Value, bool) {
	selection := engine.pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.FieldVal {
		return ps6101Value{}, false
	}
	path := strings.Join(ps6101SelectionFieldPath(selection), ".")
	if path == "" {
		path = ps6101FieldName(selection.Obj().(*types.Var))
	}
	fields := base.fieldValues()
	selected, ok := fields[path]
	if !ok {
		return ps6101Value{}, false
	}
	children := make(map[string]ps6101Value)
	for suffix, child := range fields {
		if !strings.HasPrefix(suffix, path) || len(suffix) == len(path) || suffix[len(path)] != '.' && suffix[len(path)] != '[' {
			continue
		}
		remainder := strings.TrimPrefix(suffix[len(path):], ".")
		if remainder != "" {
			children[remainder] = ps6101CloneValue(child)
		}
	}
	selected.setFields(children)
	return ps6101CloneValue(selected), true
}

func (engine *ps6101Engine) selectIndexValue(base ps6101Value, expression *ast.IndexExpr) (ps6101Value, bool) {
	if expression == nil {
		return ps6101Value{}, false
	}
	index := engine.indexConstant(expression.Index)
	if index == nil {
		return ps6101Value{}, false
	}
	exact, ok := constant.Int64Val(index)
	if !ok {
		return ps6101Value{}, false
	}
	selected, found := base.elements[exact]
	prefix := "[" + index.ExactString() + "]"
	children := make(map[string]ps6101Value)
	for path, child := range base.fieldValues() {
		if path == prefix {
			selected, found = ps6101CloneValue(child), true
			continue
		}
		if !strings.HasPrefix(path, prefix) || len(path) == len(prefix) || path[len(prefix)] != '.' && path[len(prefix)] != '[' {
			continue
		}
		remainder := strings.TrimPrefix(path[len(prefix):], ".")
		if remainder != "" {
			children[remainder] = ps6101CloneValue(child)
			found = true
		}
	}
	if !found {
		return ps6101Value{}, false
	}
	selected.setFields(children)
	return ps6101CloneValue(selected), true
}

func ps6101TestingMethodName(selection *types.Selection) string {
	if selection == nil {
		return ""
	}
	method, ok := selection.Obj().(*types.Func)
	if !ok || method.Pkg() == nil || method.Pkg().Path() != "testing" {
		return ""
	}
	return method.Name()
}

func (engine *ps6101Engine) testingN(selector *ast.SelectorExpr) bool {
	selection := engine.pass.TypesInfo.Selections[selector]
	field, ok := ps6101SelectionObject(selection).(*types.Var)
	return ok && field.Name() == "N" && field.Pkg() != nil && field.Pkg().Path() == "testing"
}

func ps6101SelectionObject(selection *types.Selection) types.Object {
	if selection == nil {
		return nil
	}
	return selection.Obj()
}

func (engine *ps6101Engine) functionDeclaration(object types.Object) *ast.FuncDecl {
	function, ok := object.(*types.Func)
	if !ok || function == nil {
		return nil
	}
	if declaration := engine.functions[function]; declaration != nil {
		return declaration
	}
	return engine.functions[function.Origin()]
}

// dynamicMethodDeclaration resolves interface dispatch only when the tracked
// interface value has one concrete dynamic type and that type's method body is
// local. Unknown, joined, and imported implementations remain opaque.
func (engine *ps6101Engine) dynamicMethodDeclaration(receiver ps6101Value, method *types.Func) *ast.FuncDecl {
	if method == nil {
		return nil
	}
	dynamic := engine.resolveTypeArgument(receiver.analysisValue().dynamic)
	if dynamic == nil {
		return nil
	}
	implementation, _, _ := types.LookupFieldOrMethod(dynamic, false, method.Pkg(), method.Name())
	return engine.functionDeclaration(implementation)
}

func (engine *ps6101Engine) evalCall(call *ast.CallExpr) []ps6101Value {
	if call == nil {
		return nil
	}
	// evalAssignmentRHS and return analysis dispatch calls directly here. Keep
	// compile-time conversion results exact just as evalUncached does; treating
	// int64(300) as an unknown runtime call would prematurely abandon a bounded
	// loop before its chronological tail can be analyzed.
	if value := engine.pass.TypesInfo.Types[call].Value; value != nil {
		return []ps6101Value{ps6101ConstantValue(value)}
	}
	// A call evaluates its function value before its arguments. Keeping this
	// result also prevents method receivers and returned function values from
	// being evaluated again during dispatch.
	target := engine.eval(call.Fun)
	if target.callable != nil && target.callable.testingMethod != "" {
		if results, handled := engine.invokeTestingMethod(call, target.callable); handled {
			return results
		}
		for _, argument := range call.Args {
			engine.eval(argument)
			engine.invalidateSharedArgument(argument)
		}
		return nil
	}
	if ps6101SymmetricRandomCall(engine.pass, call) {
		return []ps6101Value{ps6101BoundedRandomValue(call.Pos(), math.MaxFloat64)}
	}
	if ps6101AbsCall(engine.pass, call) && len(call.Args) == 1 {
		value := engine.eval(call.Args[0])
		bounds := value.analysisValue()
		upper := ps6101AbsoluteBound(bounds.lower, bounds.upper)
		value.kind, value.constant = ps6101Nonnegative, nil
		analysis := value.mutableAnalysis()
		analysis.lower, analysis.upper = constant.MakeInt64(0), upper
		value.identity = 0
		return []ps6101Value{value}
	}
	if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Abs" && len(call.Args) == 1 {
		// A lookalike Abs is not a positivity proof. Preserve the argument's
		// signed provenance rather than granting math.Abs semantics.
		value := engine.eval(call.Args[0])
		value.identity = 0
		return []ps6101Value{value}
	}
	if typed := engine.pass.TypesInfo.Types[call.Fun]; typed.IsType() && len(call.Args) == 1 {
		return []ps6101Value{engine.convertValue(typed.Type, engine.pass.TypesInfo.TypeOf(call.Args[0]), engine.eval(call.Args[0]))}
	}
	if identifier, ok := call.Fun.(*ast.Ident); ok {
		if builtin, ok := identObject(engine.pass, identifier).(*types.Builtin); ok {
			switch builtin.Name() {
			case "append":
				values := make([]ps6101Value, len(call.Args))
				for index, argument := range call.Args {
					values[index] = engine.eval(argument)
				}
				var result ps6101Value
				if len(values) > 0 {
					result = engine.appendPrefixValue(call.Args[0], values[0])
				}
				for _, value := range values[1:] {
					result = ps6101CollectionMerge(result, value)
				}
				result.nonempty = len(call.Args) > 1 || result.nonempty
				result.identity = 0
				added, addedOK := ps6101AppendCount(call, values)
				if len(values) > 0 {
					destination := values[0]
					elements := engine.appendedElements(call, values, destination, added, addedOK)
					reusesBacking := destination.lengthOK && destination.capacityOK && addedOK &&
						destination.length+added <= destination.capacity
					if !destination.lengthOK || !destination.capacityOK || !addedOK || reusesBacking {
						engine.invalidateAppendBacking(call.Args[0], destination, added, addedOK)
					}
					if reusesBacking {
						result.reference = destination.reference
						result.offset, result.offsetOK = destination.offset, destination.offsetOK
					} else if destination.lengthOK && destination.capacityOK && addedOK {
						result.offset, result.offsetOK = 0, true
					}
					if destination.lengthOK && addedOK {
						result.length, result.lengthOK = destination.length+added, true
					}
					if destination.capacityOK && result.lengthOK && result.length <= destination.capacity {
						result.capacity, result.capacityOK = destination.capacity, true
					}
					result.elements = elements
				}
				return []ps6101Value{result}
			case "make":
				return []ps6101Value{engine.makeValue(call)}
			case "len", "cap":
				if len(call.Args) == 1 {
					value := engine.eval(call.Args[0])
					known, exact := value.lengthOK, value.length
					if builtin.Name() == "cap" {
						known, exact = value.capacityOK, value.capacity
					}
					if known {
						return []ps6101Value{ps6101ConstantValue(constant.MakeInt64(exact))}
					}
				}
				result := ps6101Value{kind: ps6101Nonnegative}
				result.mutableAnalysis().lower = constant.MakeInt64(0)
				return []ps6101Value{result}
			case "clear", "delete", "copy":
				for _, argument := range call.Args {
					engine.eval(argument)
				}
				if len(call.Args) > 0 {
					engine.invalidateSharedArgument(call.Args[0])
				}
				return nil
			default:
				for _, argument := range call.Args {
					engine.eval(argument)
				}
				return nil
			}
		}
	}
	if target.callable != nil {
		switch {
		case target.callable.literal != nil:
			return engine.analyzeLiteral(target.callable.literal, call)
		case target.callable.interfaceMethod != nil && target.callable.receiverFromFirst:
			if len(call.Args) == 0 {
				engine.invalidateCapturedState()
				return nil
			}
			receiver := engine.eval(call.Args[0])
			declaration := engine.dynamicMethodDeclaration(receiver, target.callable.interfaceMethod)
			if declaration == nil {
				engine.invalidateSharedArgument(call.Args[0])
				for _, argument := range call.Args[1:] {
					engine.eval(argument)
					engine.invalidateSharedArgument(argument)
				}
				return nil
			}
			adjusted := *call
			adjusted.Args = call.Args[1:]
			callable := *target.callable
			callable.function = declaration
			callable.interfaceMethod = nil
			callable.receiverFromFirst = false
			engine.snapshotCallableReceiver(&callable, call.Args[0], receiver)
			return engine.analyzeFunction(declaration, &adjusted, call.Args[0], &callable)
		case target.callable.function != nil:
			if target.callable.receiverFromFirst {
				if len(call.Args) == 0 {
					engine.invalidateCapturedState()
					return nil
				}
				adjusted := *call
				adjusted.Args = call.Args[1:]
				return engine.analyzeFunction(target.callable.function, &adjusted, call.Args[0], target.callable)
			}
			return engine.analyzeFunction(target.callable.function, call, target.callable.receiver, target.callable)
		}
	}
	function, receiver := engine.resolveCall(call)
	if function == nil {
		if engine.indirectCallable(call.Fun) {
			engine.invalidateCapturedState()
		}
		if selector, ok := ps6101Unparen(call.Fun).(*ast.SelectorExpr); ok {
			engine.invalidateSharedArgument(selector.X)
		}
		for _, argument := range call.Args {
			engine.eval(argument)
			engine.invalidateSharedArgument(argument)
		}
		return nil
	}
	return engine.analyzeFunction(function, call, receiver, nil)
}

func (engine *ps6101Engine) invokeTestingMethod(call *ast.CallExpr, callable *ps6101Callable) ([]ps6101Value, bool) {
	if call == nil || callable == nil || callable.testingMethod == "" {
		return nil, false
	}
	adjusted := *call
	if callable.receiverFromFirst {
		if len(call.Args) == 0 {
			return nil, true
		}
		receiver := engine.eval(call.Args[0])
		adjusted.Args = call.Args[1:]
		if !receiver.testing {
			for _, argument := range adjusted.Args {
				engine.eval(argument)
			}
			return nil, true
		}
	}
	switch callable.testingMethod {
	case "ResetTimer":
		for _, argument := range adjusted.Args {
			engine.eval(argument)
		}
		engine.gates = nil
		engine.timer = ps6101TimerRunning
		return nil, true
	case "StartTimer":
		for _, argument := range adjusted.Args {
			engine.eval(argument)
		}
		engine.timer = ps6101TimerRunning
		return nil, true
	case "StopTimer":
		for _, argument := range adjusted.Args {
			engine.eval(argument)
		}
		engine.timer = ps6101TimerStopped
		return nil, true
	case "Run":
		// The name and callback are ordinary arguments and are evaluated in
		// order. analyzeSubbenchmark evaluates the callback value once before
		// taking the child's entry snapshot.
		if len(adjusted.Args) > 0 {
			engine.eval(adjusted.Args[0])
		}
		engine.analyzeSubbenchmark(&adjusted)
		engine.gates = nil
		engine.timer = ps6101TimerStopped
		return nil, true
	case "RunParallel":
		engine.analyzeBenchmarkCallback(&adjusted, 0, engine.timer)
		return nil, true
	case "Loop", "Next":
		for _, argument := range adjusted.Args {
			engine.eval(argument)
			engine.invalidateSharedArgument(argument)
		}
		return []ps6101Value{{}}, true
	default:
		// The receiver of a method expression was already consumed above.
		// Handle all remaining testing methods here so the generic call path
		// cannot evaluate that receiver a second time.
		for _, argument := range adjusted.Args {
			engine.eval(argument)
			engine.invalidateSharedArgument(argument)
		}
		return nil, true
	}
}

func (engine *ps6101Engine) appendedElements(call *ast.CallExpr, values []ps6101Value, destination ps6101Value, added int64, addedOK bool) map[int64]ps6101Value {
	if call == nil || len(values) < 2 || !destination.lengthOK || !addedOK || added < 0 || added > 256 {
		return nil
	}
	result := make(map[int64]ps6101Value, added)
	if !call.Ellipsis.IsValid() {
		for index, value := range values[1:] {
			result[destination.length+int64(index)] = ps6101CloneValue(value)
		}
		return result
	}
	last := values[len(values)-1]
	base, baseOK := engine.location(call.Args[len(call.Args)-1], true)
	for index := int64(0); index < last.length; index++ {
		value := last
		if baseOK && last.offsetOK {
			location := base
			location.path += "[" + strconv.FormatInt(last.offset+index, 10) + "]"
			if stored, present := engine.state[location]; present {
				value = stored
			}
		}
		result[destination.length+index] = ps6101CloneValue(value)
	}
	return result
}

func (engine *ps6101Engine) appendPrefixValue(expression ast.Expr, source ps6101Value) ps6101Value {
	if !source.lengthOK || !source.offsetOK || source.length > 256 {
		return source
	}
	result := ps6101Value{length: source.length, lengthOK: true, capacity: source.capacity, capacityOK: source.capacityOK}
	result.offset, result.offsetOK = source.offset, true
	result.reference = source.reference
	if source.length == 0 {
		return result
	}
	baseExpression := expression
	if sliced, ok := ps6101Unparen(expression).(*ast.SliceExpr); ok {
		baseExpression = sliced.X
	}
	base, ok := engine.location(baseExpression, true)
	if !ok {
		return source
	}
	for index := int64(0); index < source.length; index++ {
		location := base
		location.path += "[" + strconv.FormatInt(source.offset+index, 10) + "]"
		value, present := engine.state[location]
		if !present {
			return source
		}
		result = ps6101CollectionMerge(result, value)
	}
	result.length, result.lengthOK = source.length, true
	result.capacity, result.capacityOK = source.capacity, source.capacityOK
	result.offset, result.offsetOK = source.offset, true
	result.reference = source.reference
	return result
}

func (engine *ps6101Engine) indirectCallable(expression ast.Expr) bool {
	if base, ok := engine.instantiationBase(expression); ok {
		expression = base
	}
	typ := engine.pass.TypesInfo.TypeOf(expression)
	if typ == nil {
		return false
	}
	if _, ok := types.Unalias(typ).Underlying().(*types.Signature); !ok {
		return false
	}
	switch value := ps6101Unparen(expression).(type) {
	case *ast.Ident:
		_, direct := identObject(engine.pass, value).(*types.Func)
		return !direct
	case *ast.SelectorExpr:
		if selection := engine.pass.TypesInfo.Selections[value]; selection != nil {
			_, direct := selection.Obj().(*types.Func)
			return !direct
		}
		_, direct := identObject(engine.pass, value.Sel).(*types.Func)
		return !direct
	default:
		return true
	}
}

func (engine *ps6101Engine) instantiationBase(expression ast.Expr) (ast.Expr, bool) {
	var base ast.Expr
	switch value := ps6101Unparen(expression).(type) {
	case *ast.IndexExpr:
		base = value.X
	case *ast.IndexListExpr:
		base = value.X
	default:
		return nil, false
	}
	identifier := engine.instantiationIdentifier(base)
	if identifier == nil {
		return nil, false
	}
	_, instantiated := engine.pass.TypesInfo.Instances[identifier]
	return base, instantiated
}

func (engine *ps6101Engine) instantiationIdentifier(expression ast.Expr) *ast.Ident {
	expression = ps6101Unparen(expression)
	switch value := expression.(type) {
	case *ast.IndexExpr:
		return engine.instantiationIdentifier(value.X)
	case *ast.IndexListExpr:
		return engine.instantiationIdentifier(value.X)
	case *ast.Ident:
		return value
	case *ast.SelectorExpr:
		return value.Sel
	}
	return nil
}

func (engine *ps6101Engine) invalidateCapturedState() {
	locations := make([]ps6101Location, 0, len(engine.state)+len(engine.counterGates))
	for location, value := range engine.state {
		if len(value.sources) > 0 || value.threshold || value.reference != nil {
			locations = append(locations, location)
		}
	}
	for location := range engine.counterGates {
		locations = append(locations, location)
	}
	for _, location := range locations {
		engine.invalidateLocationAndReferences(location)
	}
}

func (engine *ps6101Engine) invalidateSharedArgument(argument ast.Expr) {
	if argument == nil || !ps6101ContainsReference(engine.pass.TypesInfo.TypeOf(argument)) {
		return
	}
	if location, ok := engine.location(argument, true); ok {
		engine.invalidateLocationAndReferences(location)
	}
}

func (engine *ps6101Engine) analyzeLiteral(literal *ast.FuncLit, call *ast.CallExpr) []ps6101Value {
	if literal == nil || literal.Body == nil || literal.Type == nil {
		return nil
	}
	callerRoots, callerAliases := engine.callerVisibility()
	// Match analyzeFunction's call ordering: a finite f(f(x)) evaluates and
	// completes the inner literal before the outer invocation becomes active.
	// A call reached from the literal body still sees the active marker, but
	// only after its own arguments have been evaluated in Go order.
	engine.bindParameters(literal.Type.Params, call)
	if !engine.enterCall() {
		engine.invalidateCapturedState()
		engine.retainCallerVisibility(callerRoots, callerAliases)
		return nil
	}
	defer engine.leaveCall()
	if engine.activeLiteral[literal] {
		engine.invalidateCapturedState()
		engine.retainCallerVisibility(callerRoots, callerAliases)
		return nil
	}
	engine.activeLiteral[literal] = true
	defer delete(engine.activeLiteral, literal)
	engine.clearNodeLocals(literal.Body)
	oldReturns, oldExits := engine.returns, engine.exits
	oldJumps := engine.jumps
	oldControls := engine.controls
	oldResultObjects, oldResultTypes, oldRecoverable := engine.resultObjects, engine.resultTypes, engine.recoverable
	engine.returns = nil
	engine.exits = nil
	engine.jumps = make(map[types.Object][]ps6101ExitState)
	engine.controls = make(map[types.Object]*ps6101ControlStates)
	engine.resultObjects = engine.namedResults(literal.Type.Results)
	engine.resultTypes = engine.fieldTypes(literal.Type.Results)
	for _, result := range engine.resultObjects {
		engine.killPrefix(ps6101Location{root: result})
	}
	flow := engine.analyzeBlock(literal.Body)
	engine.discardProofsForUnresolvedJumps()
	if flow&ps6101FallsThrough != 0 {
		engine.captureExit()
	}
	results := engine.mergeReturns(engine.returns)
	engine.mergeExits()
	engine.retainCallerVisibility(callerRoots, callerAliases)
	engine.returns, engine.exits = oldReturns, oldExits
	engine.jumps = oldJumps
	engine.controls = oldControls
	engine.resultObjects, engine.resultTypes, engine.recoverable = oldResultObjects, oldResultTypes, oldRecoverable
	return results
}

func (engine *ps6101Engine) discardProofsForUnresolvedJumps() {
	for _, states := range engine.jumps {
		if len(states) == 0 {
			continue
		}
		// A remaining target is a backward jump. The bounded intraprocedural
		// pass has already visited its label, so it cannot prove which input,
		// gate, validation, or counter revision survives another iteration.
		// Drop candidate evidence for this control-ambiguous function instead
		// of accidentally treating the jump as one-shot fallthrough.
		engine.invalidateCapturedState()
		engine.gates = nil
		engine.subGates = nil
		engine.proofs = make(map[ps6101GateKey]bool)
		engine.counterGates = make(map[ps6101Location]ps6101CounterEvidence)
		return
	}
}

func (engine *ps6101Engine) testingMethod(call *ast.CallExpr) string {
	if call == nil {
		return ""
	}
	expression := ps6101Unparen(call.Fun)
	if selector, ok := expression.(*ast.SelectorExpr); ok {
		return ps6101TestingMethodName(engine.pass.TypesInfo.Selections[selector])
	}
	if location, ok := engine.location(expression, true); ok {
		if callable := engine.state[location].callable; callable != nil {
			return callable.testingMethod
		}
	}
	return ""
}

func (engine *ps6101Engine) analyzeSubbenchmark(call *ast.CallExpr) {
	engine.analyzeBenchmarkCallback(call, 1, ps6101TimerRunning)
}

func (engine *ps6101Engine) analyzeBenchmarkCallback(call *ast.CallExpr, callbackIndex int, timer ps6101TimerState) {
	if call == nil || callbackIndex < 0 || callbackIndex >= len(call.Args) {
		return
	}
	callbackExpression := call.Args[callbackIndex]
	var body *ast.BlockStmt
	var parameters *ast.FieldList
	var declaration *ast.FuncDecl
	callable := engine.eval(callbackExpression).callable
	if callable != nil {
		switch {
		case callable.literal != nil:
			body, parameters = callable.literal.Body, callable.literal.Type.Params
		case callable.function != nil:
			declaration = callable.function
		}
	}
	if declaration == nil && body == nil {
		declaration = engine.resolveFunctionExpression(callbackExpression)
	}
	if declaration != nil {
		body, parameters = declaration.Body, declaration.Type.Params
	}
	if body == nil || parameters == nil {
		return
	}
	var typeArguments map[*types.TypeParam]types.Type
	if callable != nil {
		typeArguments = maps.Clone(callable.typeArguments)
	}
	entryState, entryAliases := engine.cloneState(), engine.cloneAliases()
	child := &ps6101Engine{
		pass: engine.pass, functions: engine.functions, state: engine.copyState(entryState), aliases: engine.copyAliases(entryAliases),
		active: make(map[types.Object]bool), activeLiteral: make(map[*ast.FuncLit]bool), proofs: make(map[ps6101GateKey]bool),
		counterGates: ps6101CopyCounterGates(engine.counterGates), counterRevs: ps6101CopyCounterRevisions(engine.counterRevs),
		nextRevision: engine.nextRevision, timer: timer, jumps: make(map[types.Object][]ps6101ExitState),
		controls: make(map[types.Object]*ps6101ControlStates), loopTransfers: engine.loopTransfers, loopAbstract: engine.loopAbstract,
		typeArguments: typeArguments,
	}
	child.indexCounterLocations(body)
	for proof := range engine.proofs {
		child.proofs[proof] = true
	}
	if declaration != nil {
		child.clearLocals(declaration)
		if declaration.Recv != nil && len(declaration.Recv.List) == 1 && len(declaration.Recv.List[0].Names) == 1 {
			if callable != nil && callable.receiverValue != nil {
				child.bindParameterSnapshot(declaration.Recv.List[0].Names[0], callable)
			} else if selector, ok := ps6101Unparen(callbackExpression).(*ast.SelectorExpr); ok {
				child.bindParameter(declaration.Recv.List[0].Names[0], selector.X)
			}
		}
	} else {
		child.clearNodeLocals(body)
	}
	for _, field := range parameters.List {
		for _, name := range field.Names {
			child.state[ps6101Location{root: identObject(engine.pass, name)}] = ps6101Value{testing: true}
		}
	}
	flow := child.analyzeBlock(body)
	child.discardProofsForUnresolvedJumps()
	if flow&ps6101FallsThrough != 0 {
		child.captureExit()
	}
	child.mergeExits()
	for location := range entryState {
		value, ok := child.state[location]
		if !ok {
			delete(engine.state, location)
		} else {
			engine.state[location] = ps6101CloneValue(value)
		}
		engine.counterRevs[location] = child.counterRevs[location]
		if evidence, present := child.counterGates[location]; present {
			engine.counterGates[location] = ps6101CloneCounterEvidence(evidence)
		} else {
			delete(engine.counterGates, location)
		}
	}
	for object := range entryAliases {
		alias, ok := child.aliases[object]
		if !ok {
			delete(engine.aliases, object)
			continue
		}
		engine.aliases[object] = alias
	}
	engine.nextRevision = max(engine.nextRevision, child.nextRevision)
	for _, gate := range child.gates {
		if !child.proofs[gate.key] {
			engine.subGates = append(engine.subGates, gate)
		}
	}
	for _, gate := range child.subGates {
		if !child.proofs[gate.key] {
			engine.subGates = append(engine.subGates, gate)
		}
	}
}

func (engine *ps6101Engine) resolveFunctionExpression(expression ast.Expr) *ast.FuncDecl {
	if base, ok := engine.instantiationBase(expression); ok {
		return engine.resolveFunctionExpression(base)
	}
	switch function := ps6101Unparen(expression).(type) {
	case *ast.Ident:
		return engine.functions[identObject(engine.pass, function)]
	case *ast.SelectorExpr:
		if selection := engine.pass.TypesInfo.Selections[function]; selection != nil {
			return engine.functions[selection.Obj()]
		}
		return engine.functions[identObject(engine.pass, function.Sel)]
	}
	return nil
}

func ps6101CopyCounterRevisions(source map[ps6101Location]uint64) map[ps6101Location]uint64 {
	result := make(map[ps6101Location]uint64, len(source))
	for location, revision := range source {
		result[location] = revision
	}
	return result
}

func ps6101CloneCounterEvidence(evidence ps6101CounterEvidence) ps6101CounterEvidence {
	result := ps6101CounterEvidence{
		revision: evidence.revision, incremented: evidence.incremented,
		gates: make(map[ps6101GateKey]bool, len(evidence.gates)),
		sites: make(map[token.Pos]bool, len(evidence.sites)),
	}
	for gate := range evidence.gates {
		result.gates[gate] = true
	}
	for site := range evidence.sites {
		result.sites[site] = true
	}
	return result
}

func ps6101CopyCounterGates(source map[ps6101Location]ps6101CounterEvidence) map[ps6101Location]ps6101CounterEvidence {
	result := make(map[ps6101Location]ps6101CounterEvidence, len(source))
	for location, evidence := range source {
		result[location] = ps6101CloneCounterEvidence(evidence)
	}
	return result
}

func (engine *ps6101Engine) mergeCounterFlow(
	gateStates []map[ps6101Location]ps6101CounterEvidence,
	revisionStates []map[ps6101Location]uint64,
) {
	engine.counterGates = make(map[ps6101Location]ps6101CounterEvidence)
	engine.counterRevs = make(map[ps6101Location]uint64)
	if len(revisionStates) == 0 {
		return
	}
	locations := make(map[ps6101Location]bool)
	for _, revisions := range revisionStates {
		for location := range revisions {
			locations[location] = true
		}
	}
	for location := range locations {
		revision := revisionStates[0][location]
		consistent := true
		maximum := revision
		for _, revisions := range revisionStates[1:] {
			if revisions[location] != revision {
				consistent = false
			}
			maximum = max(maximum, revisions[location])
		}
		if len(gateStates) != len(revisionStates) {
			engine.counterRevs[location] = maximum + 1
			continue
		}
		var evidence ps6101CounterEvidence
		present := len(gateStates) == len(revisionStates)
		for _, gates := range gateStates {
			candidate, ok := gates[location]
			if !ok {
				present = false
				break
			}
			if candidate.incremented {
				if evidence.incremented {
					if !ps6101SameCounterSites(evidence.sites, candidate.sites) {
						present = false
						break
					}
					for gate := range candidate.gates {
						evidence.gates[gate] = true
					}
					continue
				}
				evidence = ps6101CloneCounterEvidence(candidate)
			}
		}
		if !consistent {
			revision = maximum + 1
		}
		engine.counterRevs[location] = revision
		if present {
			evidence.revision = revision
			engine.counterGates[location] = ps6101CloneCounterEvidence(evidence)
		}
	}
}

func (engine *ps6101Engine) convertValue(target, source types.Type, value ps6101Value) ps6101Value {
	target = engine.resolveTypeArgument(target)
	source = engine.resolveTypeArgument(source)
	if ps6101InterfaceType(target) {
		return engine.assignmentValue(target, source, value)
	}
	if ps6101ContainsReference(target) && ps6101ContainsReference(source) {
		value.constant = nil
		value.identity = 0
		analysis := value.mutableAnalysis()
		analysis.squareID, analysis.squareSign, analysis.squareSig = 0, 0, ""
		return value
	}
	basic, ok := types.Unalias(target).Underlying().(*types.Basic)
	if !ok || basic.Info()&types.IsNumeric == 0 {
		return ps6101Value{}
	}
	value.constant = nil
	value.identity = 0
	if value.analysisValue().squareID != 0 {
		analysis := value.mutableAnalysis()
		analysis.squareSig += "|" + types.TypeString(target, func(pkg *types.Package) string {
			if pkg == nil {
				return ""
			}
			return pkg.Path()
		})
	}
	if basic.Info()&types.IsFloat != 0 {
		return value
	}
	if basic.Info()&types.IsInteger == 0 {
		value.kind = ps6101Unknown
		analysis := value.mutableAnalysis()
		analysis.lower, analysis.upper = nil, nil
		return value
	}
	wordBits := 0
	if basic.Kind() == types.Int || basic.Kind() == types.Uint || basic.Kind() == types.Uintptr {
		if engine.pass.TypesSizes != nil {
			wordBits = int(engine.pass.TypesSizes.Sizeof(target) * 8)
		}
	}
	lower, upper, ok := ps6101IntegerTypeBounds(basic.Kind(), wordBits)
	if !ok {
		value.kind = ps6101Unknown
		analysis := value.mutableAnalysis()
		analysis.lower, analysis.upper = nil, nil
		return value
	}
	bounds := value.analysisValue()
	if ps6101BoundsWithin(bounds.lower, bounds.upper, lower, upper) {
		ps6101ClassifyBounds(&value)
		return value
	}
	analysis := value.mutableAnalysis()
	analysis.lower, analysis.upper = lower, upper
	if basic.Info()&types.IsUnsigned != 0 {
		value.kind = ps6101Nonnegative
	} else if len(value.sources) > 0 {
		value.kind = ps6101Symmetric
	} else {
		value.kind = ps6101Unknown
	}
	return value
}

func ps6101IntegerTypeBounds(kind types.BasicKind, wordBits int) (constant.Value, constant.Value, bool) {
	bits, signed := 0, true
	switch kind {
	case types.Int8:
		bits = 8
	case types.Int16:
		bits = 16
	case types.Int32:
		bits = 32
	case types.Int64:
		bits = 64
	case types.Int:
		bits = wordBits
	case types.Uint8:
		bits, signed = 8, false
	case types.Uint16:
		bits, signed = 16, false
	case types.Uint32:
		bits, signed = 32, false
	case types.Uint64:
		bits, signed = 64, false
	case types.Uint, types.Uintptr:
		bits, signed = wordBits, false
	default:
		return nil, nil, false
	}
	if bits != 8 && bits != 16 && bits != 32 && bits != 64 {
		return nil, nil, false
	}
	one := constant.MakeInt64(1)
	limit := constant.Shift(one, token.SHL, uint(bits))
	if !signed {
		return constant.MakeInt64(0), constant.BinaryOp(limit, token.SUB, one), true
	}
	half := constant.Shift(one, token.SHL, uint(bits-1))
	return constant.UnaryOp(token.SUB, half, 0), constant.BinaryOp(half, token.SUB, one), true
}

func ps6101BoundsWithin(lower, upper, targetLower, targetUpper constant.Value) bool {
	return lower != nil && upper != nil && constant.Compare(lower, token.GEQ, targetLower) &&
		constant.Compare(upper, token.LEQ, targetUpper)
}

func (engine *ps6101Engine) makeValue(call *ast.CallExpr) ps6101Value {
	result := ps6101Value{offsetOK: true}
	if len(call.Args) < 2 {
		return result
	}
	if typ := engine.pass.TypesInfo.TypeOf(call); typ != nil {
		if _, ok := types.Unalias(typ).Underlying().(*types.Map); ok {
			result.lengthOK = true
			if capacity, exact := engine.exactInteger(call.Args[1]); exact && capacity >= 0 {
				result.capacity, result.capacityOK = capacity, true
			}
			return result
		}
	}
	if length, ok := engine.exactInteger(call.Args[1]); ok && length >= 0 {
		result.length, result.lengthOK = length, true
		result.nonempty = length > 0
	}
	if len(call.Args) >= 3 {
		if capacity, ok := engine.exactInteger(call.Args[2]); ok && capacity >= 0 {
			result.capacity, result.capacityOK = capacity, true
		}
	} else if result.lengthOK {
		result.capacity, result.capacityOK = result.length, true
	}
	return result
}

func (engine *ps6101Engine) exactInteger(expression ast.Expr) (int64, bool) {
	if expression == nil {
		return 0, false
	}
	value := engine.pass.TypesInfo.Types[expression].Value
	if value == nil {
		value = engine.eval(expression).constant
	}
	if value == nil || value.Kind() != constant.Int {
		return 0, false
	}
	exact, ok := constant.Int64Val(value)
	return exact, ok
}

func (engine *ps6101Engine) applyCompositeLength(result *ps6101Value, literal *ast.CompositeLit) {
	if result == nil || literal == nil {
		return
	}
	typ := types.Unalias(engine.pass.TypesInfo.TypeOf(literal))
	if named, ok := typ.(*types.Named); ok {
		typ = named.Underlying()
	}
	switch value := typ.Underlying().(type) {
	case *types.Array:
		result.length, result.capacity = value.Len(), value.Len()
		result.lengthOK, result.capacityOK = true, true
		result.offsetOK = true
		result.nonempty = value.Len() > 0
	case *types.Slice:
		length := int64(0)
		next := int64(0)
		for _, element := range literal.Elts {
			index := next
			if keyed, ok := element.(*ast.KeyValueExpr); ok {
				var exact bool
				index, exact = engine.exactInteger(keyed.Key)
				if !exact || index < 0 {
					return
				}
			}
			length = max(length, index+1)
			next = index + 1
		}
		result.length, result.capacity = length, length
		result.lengthOK, result.capacityOK = true, true
		result.offsetOK = true
		result.nonempty = length > 0
	case *types.Map:
		result.length, result.lengthOK = int64(len(literal.Elts)), true
		result.nonempty = len(literal.Elts) > 0
	}
}

func (engine *ps6101Engine) applySliceBounds(result *ps6101Value, expression *ast.SliceExpr) {
	if result == nil || expression == nil {
		return
	}
	low, lowOK := int64(0), true
	if expression.Low != nil {
		low, lowOK = engine.exactInteger(expression.Low)
	}
	high, highOK := result.length, result.lengthOK
	if expression.High != nil {
		high, highOK = engine.exactInteger(expression.High)
	}
	maximum, maximumOK := result.capacity, result.capacityOK
	if expression.Max != nil {
		maximum, maximumOK = engine.exactInteger(expression.Max)
	}
	baseOffset, baseOffsetOK := result.offset, result.offsetOK
	result.offsetOK = baseOffsetOK && lowOK
	if result.offsetOK {
		result.offset = baseOffset + low
	}
	result.lengthOK = lowOK && highOK && high >= low
	if result.lengthOK {
		result.length = high - low
		result.nonempty = result.length > 0
	}
	result.capacityOK = lowOK && maximumOK && maximum >= low
	if result.capacityOK {
		result.capacity = maximum - low
	}
}

func ps6101AppendCount(call *ast.CallExpr, values []ps6101Value) (int64, bool) {
	if call == nil || len(values) < 2 {
		return 0, true
	}
	if call.Ellipsis.IsValid() {
		prefix := int64(len(values) - 2)
		last := values[len(values)-1]
		if !last.lengthOK {
			return 0, false
		}
		return prefix + last.length, true
	}
	return int64(len(values) - 1), true
}

func (engine *ps6101Engine) invalidateAppendBacking(expression ast.Expr, destination ps6101Value, added int64, addedOK bool) {
	baseExpression := expression
	start := destination.offset + destination.length
	startOK := destination.offsetOK && destination.lengthOK
	if sliced, ok := ps6101Unparen(expression).(*ast.SliceExpr); ok {
		baseExpression = sliced.X
	}
	base, ok := engine.location(baseExpression, true)
	if !ok {
		engine.invalidateCapturedState()
		return
	}
	if startOK && addedOK && added >= 0 && added <= 256 {
		for offset := int64(0); offset < added; offset++ {
			location := base
			location.path += "[" + strconv.FormatInt(start+offset, 10) + "]"
			engine.invalidatePrefix(location)
		}
		engine.recomputeIndexedValue(base)
		return
	}
	engine.invalidateLocationAndReferences(base)
}

func (engine *ps6101Engine) resolveCall(call *ast.CallExpr) (*ast.FuncDecl, ast.Expr) {
	if call != nil {
		if base, ok := engine.instantiationBase(call.Fun); ok {
			adjusted := *call
			adjusted.Fun = base
			return engine.resolveCall(&adjusted)
		}
	}
	switch function := call.Fun.(type) {
	case *ast.Ident:
		return engine.functions[identObject(engine.pass, function)], nil
	case *ast.SelectorExpr:
		if selection := engine.pass.TypesInfo.Selections[function]; selection != nil {
			return engine.functions[selection.Obj()], function.X
		}
		return engine.functions[identObject(engine.pass, function.Sel)], nil
	}
	return nil, nil
}

func ps6101BoundedRandomValue(position token.Pos, bound float64) ps6101Value {
	result := ps6101Value{kind: ps6101Symmetric, sources: map[token.Pos]bool{position: true}}
	analysis := result.mutableAnalysis()
	analysis.lower, analysis.upper = constant.MakeFloat64(-bound), constant.MakeFloat64(bound)
	return result
}

func ps6101ConstantValue(value constant.Value) ps6101Value {
	result := ps6101Value{constant: value}
	if value.Kind() == constant.String {
		result.nonempty = constant.StringVal(value) != ""
	}
	if value.Kind() != constant.Int && value.Kind() != constant.Float {
		return result
	}
	zero := constant.MakeInt64(0)
	switch {
	case constant.Compare(value, token.EQL, zero):
		result.kind = ps6101Zero
	case constant.Compare(value, token.GTR, zero):
		result.kind = ps6101Positive
	}
	analysis := result.mutableAnalysis()
	analysis.lower, analysis.upper = value, value
	return result
}

func ps6101Add(left, right ps6101Value) ps6101Value {
	if ps6101NumericZero(right.constant) {
		return ps6101CloneValue(left)
	}
	if ps6101NumericZero(left.constant) {
		return ps6101CloneValue(right)
	}
	result := ps6101JoinedValue(left, right)
	switch {
	case left.kind == ps6101Zero:
		result.kind = right.kind
	case right.kind == ps6101Zero:
		result.kind = left.kind
	case left.kind == ps6101Symmetric && right.kind == ps6101Symmetric:
		result.kind = ps6101Symmetric
	case left.kind == ps6101Symmetric && right.kind == ps6101Positive:
		result.kind = ps6101Symmetric
	case right.kind == ps6101Symmetric && left.kind == ps6101Positive:
		result.kind = ps6101Symmetric
	case left.kind == ps6101Positive && (right.kind == ps6101Nonnegative || right.kind == ps6101Positive):
		result.kind = ps6101Positive
	case right.kind == ps6101Positive && (left.kind == ps6101Nonnegative || left.kind == ps6101Positive):
		result.kind = ps6101Positive
	case (left.kind == ps6101Nonnegative || left.kind == ps6101Positive) && (right.kind == ps6101Nonnegative || right.kind == ps6101Positive):
		result.kind = ps6101Nonnegative
	}
	leftBounds, rightBounds := left.analysisValue(), right.analysisValue()
	analysis := result.mutableAnalysis()
	analysis.lower = ps6101BoundOperation(leftBounds.lower, token.ADD, rightBounds.lower)
	analysis.upper = ps6101BoundOperation(leftBounds.upper, token.ADD, rightBounds.upper)
	ps6101ClassifyBounds(&result)
	return result
}

func ps6101Subtract(left, right ps6101Value) ps6101Value {
	if ps6101NumericZero(right.constant) {
		return ps6101CloneValue(left)
	}
	result := ps6101JoinedValue(left, right)
	if ps6101SameFiniteSignedNumericIdentity(left, right) {
		result.kind = ps6101Zero
		analysis := result.mutableAnalysis()
		analysis.lower, analysis.upper = constant.MakeInt64(0), constant.MakeInt64(0)
		return result
	}
	if left.kind == ps6101Symmetric && (right.kind == ps6101Symmetric || right.kind == ps6101Zero) {
		result.kind = ps6101Symmetric
	}
	leftBounds, rightBounds := left.analysisValue(), right.analysisValue()
	analysis := result.mutableAnalysis()
	analysis.lower = ps6101BoundOperation(leftBounds.lower, token.SUB, rightBounds.upper)
	analysis.upper = ps6101BoundOperation(leftBounds.upper, token.SUB, rightBounds.lower)
	ps6101ClassifyBounds(&result)
	return result
}

func ps6101NumericZero(value constant.Value) bool {
	return value != nil && (value.Kind() == constant.Int || value.Kind() == constant.Float) &&
		constant.Compare(value, token.EQL, constant.MakeInt64(0))
}

func ps6101Multiply(left, right ps6101Value) ps6101Value {
	result := ps6101JoinedValue(left, right)
	if ps6101SameSignedNumericIdentity(left, right) {
		result.kind = ps6101Nonnegative
		analysis := result.mutableAnalysis()
		analysis.lower = constant.MakeInt64(0)
		analysis.upper = ps6101MultiplyUpperBound(left, right)
		return result
	}
	if left.kind == ps6101Zero || right.kind == ps6101Zero {
		result.kind = ps6101Zero
	} else if left.constant != nil && right.kind == ps6101Symmetric || right.constant != nil && left.kind == ps6101Symmetric {
		result.kind = ps6101Symmetric
	} else if (left.kind == ps6101Nonnegative || left.kind == ps6101Positive) && (right.kind == ps6101Nonnegative || right.kind == ps6101Positive) {
		result.kind = ps6101Nonnegative
	}
	analysis := result.mutableAnalysis()
	analysis.lower, analysis.upper = ps6101ProductBounds(left, right)
	ps6101ClassifyBounds(&result)
	return result
}

func ps6101SameSignedNumericIdentity(left, right ps6101Value) bool {
	leftAnalysis, rightAnalysis := left.analysisValue(), right.analysisValue()
	return leftAnalysis.squareID != 0 && leftAnalysis.squareID == rightAnalysis.squareID && leftAnalysis.squareSign != 0 &&
		leftAnalysis.squareSign == rightAnalysis.squareSign && leftAnalysis.squareSig == rightAnalysis.squareSig && len(left.sources) > 0 &&
		ps6101SourceSignature(left.sources) == ps6101SourceSignature(right.sources)
}

func ps6101SameFiniteSignedNumericIdentity(left, right ps6101Value) bool {
	if !ps6101SameSignedNumericIdentity(left, right) {
		return false
	}
	maximum := constant.MakeFloat64(math.MaxFloat64)
	minimum := constant.UnaryOp(token.SUB, maximum, 0)
	leftAnalysis, rightAnalysis := left.analysisValue(), right.analysisValue()
	return ps6101BoundsWithin(leftAnalysis.lower, leftAnalysis.upper, minimum, maximum) &&
		ps6101BoundsWithin(rightAnalysis.lower, rightAnalysis.upper, minimum, maximum)
}

func ps6101Divide(left, right ps6101Value) ps6101Value {
	result := ps6101JoinedValue(left, right)
	if right.constant == nil || right.constant.Kind() != constant.Int && right.constant.Kind() != constant.Float ||
		ps6101NumericZero(right.constant) {
		return result
	}
	zero := constant.MakeInt64(0)
	positive := constant.Compare(right.constant, token.GTR, zero)
	switch {
	case left.kind == ps6101Zero:
		result.kind = ps6101Zero
	case left.kind == ps6101Symmetric:
		result.kind = ps6101Symmetric
	case positive && left.kind == ps6101Positive:
		result.kind = ps6101Positive
	case positive && left.kind == ps6101Nonnegative:
		result.kind = ps6101Nonnegative
	}
	leftAnalysis := left.analysisValue()
	analysis := result.mutableAnalysis()
	if positive {
		analysis.lower = ps6101BoundOperation(leftAnalysis.lower, token.QUO, right.constant)
		analysis.upper = ps6101BoundOperation(leftAnalysis.upper, token.QUO, right.constant)
	} else {
		analysis.lower = ps6101BoundOperation(leftAnalysis.upper, token.QUO, right.constant)
		analysis.upper = ps6101BoundOperation(leftAnalysis.lower, token.QUO, right.constant)
	}
	ps6101ClassifyBounds(&result)
	return result
}

func ps6101CollectionMerge(left, right ps6101Value) ps6101Value {
	if len(left.sources) == 0 {
		result := ps6101CloneValue(right)
		result.reference, result.callable = nil, nil
		if result.analysis != nil {
			result.analysis.dynamic = nil
		}
		result.nonempty = result.nonempty || left.nonempty
		ps6101CarryCollectionShape(&result, left, right)
		return result
	}
	if len(right.sources) == 0 {
		result := ps6101CloneValue(left)
		result.reference, result.callable = nil, nil
		if result.analysis != nil {
			result.analysis.dynamic = nil
		}
		result.nonempty = result.nonempty || right.nonempty
		ps6101CarryCollectionShape(&result, left, right)
		return result
	}
	result := ps6101JoinedValue(left, right)
	if left.kind == right.kind {
		result.kind = left.kind
	}
	result.nonempty = left.nonempty || right.nonempty
	leftBounds, rightBounds := left.analysisValue(), right.analysisValue()
	analysis := result.mutableAnalysis()
	analysis.lower = ps6101MinBound(leftBounds.lower, rightBounds.lower)
	analysis.upper = ps6101MaxBound(leftBounds.upper, rightBounds.upper)
	ps6101CarryCollectionShape(&result, left, right)
	return result
}

func ps6101CarryCollectionShape(result *ps6101Value, left, right ps6101Value) {
	if result == nil {
		return
	}
	if left.lengthOK {
		result.length, result.lengthOK = left.length, true
	} else if right.lengthOK {
		result.length, result.lengthOK = right.length, true
	}
	if left.capacityOK {
		result.capacity, result.capacityOK = left.capacity, true
	} else if right.capacityOK {
		result.capacity, result.capacityOK = right.capacity, true
	}
	if left.offsetOK {
		result.offset, result.offsetOK = left.offset, true
	} else if right.offsetOK {
		result.offset, result.offsetOK = right.offset, true
	}
}

func ps6101JoinedValue(left, right ps6101Value) ps6101Value {
	return ps6101Value{
		sources: ps6101JoinSources(left.sources, right.sources), eligible: left.eligible || right.eligible,
		aggregate: left.aggregate || right.aggregate, threshold: left.threshold || right.threshold,
		testing: left.testing || right.testing,
	}
}

func ps6101BoundOperation(left constant.Value, operation token.Token, right constant.Value) constant.Value {
	if left == nil || right == nil {
		return nil
	}
	return constant.BinaryOp(constant.ToFloat(left), operation, constant.ToFloat(right))
}

func ps6101ClassifyBounds(value *ps6101Value) {
	if value == nil {
		return
	}
	bounds := value.analysisValue()
	if bounds.lower == nil || bounds.upper == nil {
		return
	}
	zero := constant.MakeInt64(0)
	if constant.Compare(bounds.lower, token.GEQ, zero) {
		if constant.Compare(bounds.lower, token.GTR, zero) {
			value.kind = ps6101Positive
		} else {
			value.kind = ps6101Nonnegative
		}
		return
	}
	if constant.Compare(bounds.upper, token.LEQ, zero) {
		value.kind = ps6101Unknown
		return
	}
	if len(value.sources) > 0 {
		value.kind = ps6101Symmetric
	}
}

func ps6101ProductBounds(left, right ps6101Value) (constant.Value, constant.Value) {
	leftBounds, rightBounds := left.analysisValue(), right.analysisValue()
	if leftBounds.lower == nil || leftBounds.upper == nil || rightBounds.lower == nil || rightBounds.upper == nil {
		return nil, nil
	}
	products := []constant.Value{
		ps6101BoundOperation(leftBounds.lower, token.MUL, rightBounds.lower),
		ps6101BoundOperation(leftBounds.lower, token.MUL, rightBounds.upper),
		ps6101BoundOperation(leftBounds.upper, token.MUL, rightBounds.lower),
		ps6101BoundOperation(leftBounds.upper, token.MUL, rightBounds.upper),
	}
	lower, upper := products[0], products[0]
	for _, product := range products[1:] {
		lower = ps6101MinBound(lower, product)
		upper = ps6101MaxBound(upper, product)
	}
	return lower, upper
}

func ps6101MultiplyUpperBound(left, right ps6101Value) constant.Value {
	_, upper := ps6101ProductBounds(left, right)
	return ps6101AbsoluteBound(constant.MakeInt64(0), upper)
}

func ps6101AbsoluteBound(lower, upper constant.Value) constant.Value {
	if lower == nil || upper == nil {
		return nil
	}
	negativeLower := constant.UnaryOp(token.SUB, constant.ToFloat(lower), 0)
	return ps6101MaxBound(negativeLower, upper)
}

func ps6101MinBound(left, right constant.Value) constant.Value {
	if left == nil || right == nil {
		return nil
	}
	if constant.Compare(left, token.LEQ, right) {
		return left
	}
	return right
}

func ps6101MaxBound(left, right constant.Value) constant.Value {
	if left == nil || right == nil {
		return nil
	}
	if constant.Compare(left, token.GEQ, right) {
		return left
	}
	return right
}

func ps6101JoinSources(left, right map[token.Pos]bool) map[token.Pos]bool {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	joined := make(map[token.Pos]bool, len(left)+len(right))
	for source := range left {
		joined[source] = true
	}
	for source := range right {
		joined[source] = true
	}
	return joined
}

func ps6101CloneValue(value ps6101Value) ps6101Value {
	value.sources = ps6101JoinSources(value.sources, nil)
	if value.analysis != nil {
		analysis := *value.analysis
		analysis.excluded = maps.Clone(value.analysis.excluded)
		if len(value.analysis.fields) > 0 {
			analysis.fields = make(map[string]ps6101Value, len(value.analysis.fields))
			for path, field := range value.analysis.fields {
				analysis.fields[path] = ps6101CloneValue(field)
			}
		}
		value.analysis = &analysis
	}
	if value.reference != nil {
		value.reference = ps6101Reference(*value.reference)
	}
	if len(value.elements) > 0 {
		elements := value.elements
		value.elements = make(map[int64]ps6101Value, len(elements))
		for index, element := range elements {
			value.elements[index] = ps6101CloneValue(element)
		}
	}
	return value
}

func (value ps6101Value) analysisValue() ps6101ValueAnalysis {
	if value.analysis == nil {
		return ps6101ValueAnalysis{}
	}
	return *value.analysis
}

func (value ps6101Value) fieldValues() map[string]ps6101Value {
	return value.analysisValue().fields
}

func (value *ps6101Value) takeFields() map[string]ps6101Value {
	fields := value.analysisValue().fields
	if len(fields) > 0 {
		value.mutableAnalysis().fields = nil
	}
	return fields
}

func (value *ps6101Value) setFields(fields map[string]ps6101Value) {
	if len(fields) > 0 {
		value.mutableAnalysis().fields = fields
	}
}

// mutableAnalysis is copy-on-write because ps6101Value itself is deliberately
// passed by value throughout the abstract interpreter.
func (value *ps6101Value) mutableAnalysis() *ps6101ValueAnalysis {
	analysis := new(ps6101ValueAnalysis)
	if value.analysis != nil {
		*analysis = *value.analysis
	}
	value.analysis = analysis
	return analysis
}

func (engine *ps6101Engine) gateComparisons(expression ast.Expr) []ps6101GateKey {
	return engine.gateComparisonsWithPolarity(expression, false)
}

func (engine *ps6101Engine) gateComparisonsWithPolarity(expression ast.Expr, negated bool) []ps6101GateKey {
	unparenthesized := ps6101Unparen(expression)
	if unary, ok := unparenthesized.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
		return engine.gateComparisonsWithPolarity(unary.X, !negated)
	}
	binary, ok := unparenthesized.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	if binary.Op == token.LAND || binary.Op == token.LOR {
		operation := binary.Op
		if negated {
			if operation == token.LAND {
				operation = token.LOR
			} else {
				operation = token.LAND
			}
		}
		leftConstant := engine.boolConstantPolarity(binary.X, negated)
		rightConstant := engine.boolConstantPolarity(binary.Y, negated)
		if operation == token.LAND {
			if leftConstant < 0 || rightConstant < 0 {
				return nil
			}
			if leftConstant > 0 {
				return engine.gateComparisonsWithPolarity(binary.Y, negated)
			}
			if rightConstant > 0 {
				return engine.gateComparisonsWithPolarity(binary.X, negated)
			}
			return append(engine.gateComparisonsWithPolarity(binary.X, negated), engine.gateComparisonsWithPolarity(binary.Y, negated)...)
		}
		if leftConstant > 0 || rightConstant > 0 {
			return nil
		}
		if leftConstant < 0 {
			return engine.gateComparisonsWithPolarity(binary.Y, negated)
		}
		if rightConstant < 0 {
			return engine.gateComparisonsWithPolarity(binary.X, negated)
		}
		// Neither operand alone controls an OR body. Treating either one as a
		// hot-path gate would report even when the other operand admits it.
		return nil
	}
	if comparison, ok := engine.comparison(binary); ok {
		if negated {
			comparison.op = ps6101NegatedComparison(comparison.op)
		}
		return []ps6101GateKey{comparison}
	}
	return nil
}

func (engine *ps6101Engine) boolConstantPolarity(expression ast.Expr, negated bool) int {
	result := engine.boolConstant(expression)
	if negated {
		return -result
	}
	return result
}

func (engine *ps6101Engine) singleComparison(expression ast.Expr) (ps6101GateKey, bool) {
	return engine.singleComparisonWithPolarity(expression, false)
}

func (engine *ps6101Engine) singleComparisonWithPolarity(expression ast.Expr, negated bool) (ps6101GateKey, bool) {
	unparenthesized := ps6101Unparen(expression)
	if unary, ok := unparenthesized.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
		return engine.singleComparisonWithPolarity(unary.X, !negated)
	}
	binary, ok := unparenthesized.(*ast.BinaryExpr)
	if !ok {
		return ps6101GateKey{}, false
	}
	if binary.Op == token.LAND || binary.Op == token.LOR {
		operation := binary.Op
		if negated {
			if operation == token.LAND {
				operation = token.LOR
			} else {
				operation = token.LAND
			}
		}
		leftConstant := engine.boolConstantPolarity(binary.X, negated)
		rightConstant := engine.boolConstantPolarity(binary.Y, negated)
		if operation == token.LAND {
			if leftConstant > 0 {
				return engine.singleComparisonWithPolarity(binary.Y, negated)
			}
			if rightConstant > 0 {
				return engine.singleComparisonWithPolarity(binary.X, negated)
			}
		} else {
			if leftConstant < 0 {
				return engine.singleComparisonWithPolarity(binary.Y, negated)
			}
			if rightConstant < 0 {
				return engine.singleComparisonWithPolarity(binary.X, negated)
			}
		}
		return ps6101GateKey{}, false
	}
	comparison, valid := engine.comparison(binary)
	if valid && negated {
		comparison.op = ps6101NegatedComparison(comparison.op)
	}
	return comparison, valid
}

func (engine *ps6101Engine) comparison(binary *ast.BinaryExpr) (ps6101GateKey, bool) {
	switch binary.Op {
	case token.GTR, token.GEQ, token.LSS, token.LEQ, token.EQL, token.NEQ:
	default:
		return ps6101GateKey{}, false
	}
	if comparison, ok := engine.comparisonSides(binary.X, binary.Y, binary.Op); ok {
		return comparison, true
	}
	return engine.comparisonSides(binary.Y, binary.X, ps6101ReverseComparison(binary.Op))
}

func (engine *ps6101Engine) comparisonSides(aggregateExpression, thresholdExpression ast.Expr, operation token.Token) (ps6101GateKey, bool) {
	value := engine.eval(aggregateExpression)
	if value.kind != ps6101Symmetric || !value.eligible || !value.aggregate || len(value.sources) == 0 {
		return ps6101GateKey{}, false
	}
	threshold, ok := engine.threshold(thresholdExpression)
	if !ok {
		return ps6101GateKey{}, false
	}
	return ps6101GateKey{sources: ps6101SourceSignature(value.sources), op: operation, threshold: threshold, revision: value.identity}, true
}

func (engine *ps6101Engine) threshold(expression ast.Expr) (string, bool) {
	if value := engine.pass.TypesInfo.Types[expression].Value; value != nil && (value.Kind() == constant.Int || value.Kind() == constant.Float) {
		return "const:" + value.ExactString(), true
	}
	location, ok := engine.location(expression, true)
	if !ok {
		return "", false
	}
	evaluated := engine.state[location]
	if evaluated.constant != nil && (evaluated.constant.Kind() == constant.Int || evaluated.constant.Kind() == constant.Float) {
		return "const:" + evaluated.constant.ExactString(), true
	}
	if !evaluated.threshold && !ps6101ThresholdName(ps6101ExpressionName(expression)) {
		return "", false
	}
	if evaluated.identity == 0 {
		evaluated = engine.materializeThreshold(location)
	}
	return "identity:" + strconv.FormatUint(evaluated.identity, 10), true
}

func (engine *ps6101Engine) materializeThreshold(location ps6101Location) ps6101Value {
	evaluated := engine.state[location]
	if evaluated.identity != 0 {
		return ps6101CloneValue(evaluated)
	}
	engine.nextRevision++
	evaluated.identity = engine.nextRevision
	evaluated.revision = engine.nextRevision
	evaluated.threshold = true
	engine.state[location] = evaluated
	return ps6101CloneValue(evaluated)
}

func ps6101ReverseComparison(operation token.Token) token.Token {
	switch operation {
	case token.GTR:
		return token.LSS
	case token.GEQ:
		return token.LEQ
	case token.LSS:
		return token.GTR
	case token.LEQ:
		return token.GEQ
	}
	return operation
}

func ps6101NegatedComparison(operation token.Token) token.Token {
	switch operation {
	case token.GTR:
		return token.LEQ
	case token.GEQ:
		return token.LSS
	case token.LSS:
		return token.GEQ
	case token.LEQ:
		return token.GTR
	case token.EQL:
		return token.NEQ
	case token.NEQ:
		return token.EQL
	}
	return token.ILLEGAL
}

type ps6101ProofFlow uint8

const (
	ps6101ProofContinues ps6101ProofFlow = 1 << iota
	ps6101ProofFails
	ps6101ProofEscapes
)

func (engine *ps6101Engine) blockDirectlyFails(block *ast.BlockStmt) bool {
	return engine.inspectProof(func() bool {
		flow, _ := engine.proofBlockFlow(block, engine.recoverable)
		return flow == ps6101ProofFails
	})
}

func (engine *ps6101Engine) statementDirectlyFails(statement ast.Stmt) bool {
	return engine.inspectProof(func() bool {
		flow, _ := engine.proofStatementFlow(statement, engine.recoverable)
		return flow == ps6101ProofFails
	})

}

// inspectProof classifies a branch without executing it. Proof flow sometimes
// needs value information from a nested condition; evaluating a helper in that
// condition must not apply its side effects before normal control-flow analysis
// reaches the branch.
func (engine *ps6101Engine) inspectProof(inspect func() bool) bool {
	snapshot := engine.currentExitState()
	proofs := make(map[ps6101GateKey]bool, len(engine.proofs))
	for proof, present := range engine.proofs {
		proofs[proof] = present
	}
	subGates := slices.Clone(engine.subGates)
	activeGates := slices.Clone(engine.activeGates)
	nextRevision := engine.nextRevision
	result := inspect()
	engine.state, engine.aliases = snapshot.state, snapshot.aliases
	engine.gates = snapshot.gates
	engine.counterGates, engine.counterRevs = snapshot.counterGates, snapshot.counterRevs
	engine.timer, engine.recoverable = snapshot.timer, snapshot.recoverable
	engine.proofs, engine.subGates, engine.activeGates = proofs, subGates, activeGates
	engine.nextRevision = nextRevision
	return result
}

func (engine *ps6101Engine) proofBlockFlow(block *ast.BlockStmt, recoverable int) (ps6101ProofFlow, int) {
	if block == nil {
		return ps6101ProofContinues, recoverable
	}
	flow := ps6101ProofContinues
	for _, statement := range block.List {
		if flow&ps6101ProofContinues == 0 {
			break
		}
		statementFlow, nextRecoverable := engine.proofStatementFlow(statement, recoverable)
		flow = flow&^ps6101ProofContinues | statementFlow
		recoverable = max(recoverable, nextRecoverable)
	}
	return flow, recoverable
}

func (engine *ps6101Engine) proofStatementFlow(statement ast.Stmt, recoverable int) (ps6101ProofFlow, int) {
	switch value := statement.(type) {
	case nil:
		return ps6101ProofContinues, recoverable
	case *ast.BlockStmt:
		return engine.proofBlockFlow(value, recoverable)
	case *ast.ExprStmt:
		call, ok := ps6101Unparen(value.X).(*ast.CallExpr)
		if ok && engine.testingFailureCall(call, recoverable) {
			return ps6101ProofFails, recoverable
		}
	case *ast.ReturnStmt, *ast.BranchStmt:
		return ps6101ProofEscapes, recoverable
	case *ast.DeferStmt:
		if engine.deferredCallCanRecover(value.Call) {
			recoverable++
		}
		return ps6101ProofContinues, recoverable
	case *ast.IfStmt:
		if value.Init != nil {
			initFlow, nextRecoverable := engine.proofStatementFlow(value.Init, recoverable)
			if initFlow&ps6101ProofContinues == 0 {
				return initFlow, nextRecoverable
			}
			recoverable = nextRecoverable
		}
		switch engine.boolConstant(value.Cond) {
		case 1:
			return engine.proofBlockFlow(value.Body, recoverable)
		case -1:
			return engine.proofStatementFlow(value.Else, recoverable)
		default:
			bodyFlow, bodyRecoverable := engine.proofBlockFlow(value.Body, recoverable)
			elseFlow, elseRecoverable := engine.proofStatementFlow(value.Else, recoverable)
			return bodyFlow | elseFlow, max(bodyRecoverable, elseRecoverable)
		}
	}
	return ps6101ProofContinues, recoverable
}

func (engine *ps6101Engine) testingFailureCall(call *ast.CallExpr, recoverable int) bool {
	if function, ok := ps6101Unparen(call.Fun).(*ast.Ident); ok {
		return recoverable == 0 && function.Name == "panic" && identObject(engine.pass, function) == types.Universe.Lookup("panic")
	}
	switch engine.testingMethod(call) {
	case "Fatal", "Fatalf", "FailNow", "Fail", "Error", "Errorf":
		return true
	}
	return false
}

func (engine *ps6101Engine) terminalFailureCall(call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	if identifier, ok := ps6101Unparen(call.Fun).(*ast.Ident); ok {
		return engine.recoverable == 0 && identifier.Name == "panic" && identObject(engine.pass, identifier) == types.Universe.Lookup("panic")
	}
	switch engine.testingMethod(call) {
	case "Fatal", "Fatalf", "FailNow":
		return true
	}
	return false
}

func (engine *ps6101Engine) deferredCallCanRecover(call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	var root ast.Node
	switch function := ps6101Unparen(call.Fun).(type) {
	case *ast.FuncLit:
		root = function.Body
	default:
		declaration, _ := engine.resolveCall(call)
		if declaration != nil {
			root = declaration.Body
		}
	}
	if root == nil {
		return false
	}
	found := false
	ast.Inspect(root, func(node ast.Node) bool {
		if found {
			return false
		}
		called, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		identifier, ok := ps6101Unparen(called.Fun).(*ast.Ident)
		if ok && identifier.Name == "recover" && identObject(engine.pass, identifier) == types.Universe.Lookup("recover") {
			found = true
			return false
		}
		return true
	})
	return found
}

func (engine *ps6101Engine) counterProofs(expression ast.Expr) []ps6101GateKey {
	binary, ok := ps6101Unparen(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.EQL && binary.Op != token.LEQ {
		return nil
	}
	var counter ast.Expr
	if ps6101ZeroConstant(engine.pass, binary.Y) {
		counter = binary.X
	} else if binary.Op == token.EQL && ps6101ZeroConstant(engine.pass, binary.X) {
		counter = binary.Y
	} else {
		return nil
	}
	location, ok := engine.counterLocation(counter)
	if !ok {
		return nil
	}
	evidence, ok := engine.counterGates[location]
	if !ok || !evidence.incremented || evidence.revision != engine.counterRevs[location] {
		return nil
	}
	proofs := make([]ps6101GateKey, 0, len(evidence.gates))
	for gate := range evidence.gates {
		proofs = append(proofs, gate)
	}
	return proofs
}

func ps6101StrictPositiveConstant(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[expression].Value
	return value != nil && (value.Kind() == constant.Int || value.Kind() == constant.Float) && constant.Compare(value, token.GTR, constant.MakeInt64(0))
}

func ps6101ZeroConstant(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[expression].Value
	return value != nil && (value.Kind() == constant.Int || value.Kind() == constant.Float) && constant.Compare(value, token.EQL, constant.MakeInt64(0))
}

func (engine *ps6101Engine) boolConstant(expression ast.Expr) int {
	if expression == nil {
		return 1
	}
	expression = ps6101Unparen(expression)
	if cached, ok := engine.boolCache[expression]; ok {
		return cached
	}
	result := engine.boolConstantUncached(expression)
	if engine.boolCache != nil {
		engine.boolCache[expression] = result
	}
	return result
}

func (engine *ps6101Engine) boolConstantUncached(expression ast.Expr) int {
	if expression == nil {
		return 1
	}
	if value := engine.pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Bool {
		if constant.BoolVal(value) {
			return 1
		}
		return -1
	}
	switch value := expression.(type) {
	case *ast.Ident, *ast.SelectorExpr:
		if location, ok := engine.location(value, true); ok {
			stored := engine.state[location].constant
			if stored != nil && stored.Kind() == constant.Bool {
				if constant.BoolVal(stored) {
					return 1
				}
				return -1
			}
		}
	case *ast.CallExpr:
		stored := engine.eval(value).constant
		if stored != nil && stored.Kind() == constant.Bool {
			if constant.BoolVal(stored) {
				return 1
			}
			return -1
		}
	case *ast.UnaryExpr:
		if value.Op == token.NOT {
			return -engine.boolConstant(value.X)
		}
	case *ast.BinaryExpr:
		leftBool := engine.boolConstant(value.X)
		switch value.Op {
		case token.LAND:
			if leftBool < 0 {
				return -1
			}
			shortCircuit := engine.currentExitState()
			rightBool := engine.boolConstant(value.Y)
			if leftBool == 0 {
				engine.mergeMayExitStates([]ps6101ExitState{shortCircuit, engine.currentExitState()})
			}
			if rightBool < 0 {
				return -1
			}
			if leftBool > 0 && rightBool > 0 {
				return 1
			}
		case token.LOR:
			if leftBool > 0 {
				return 1
			}
			shortCircuit := engine.currentExitState()
			rightBool := engine.boolConstant(value.Y)
			if leftBool == 0 {
				engine.mergeMayExitStates([]ps6101ExitState{shortCircuit, engine.currentExitState()})
			}
			if rightBool > 0 {
				return 1
			}
			if leftBool < 0 && rightBool < 0 {
				return -1
			}
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			leftValue, rightValue := engine.eval(value.X), engine.eval(value.Y)
			left, right := leftValue.constant, rightValue.constant
			if left != nil && right != nil && constant.Compare(left, value.Op, right) {
				return 1
			}
			if left != nil && right != nil {
				return -1
			}
			if result := ps6101CompareBounds(leftValue, value.Op, rightValue); result != 0 {
				return result
			}
		}
	default:
		stored := engine.eval(expression).constant
		if stored != nil && stored.Kind() == constant.Bool {
			if constant.BoolVal(stored) {
				return 1
			}
			return -1
		}
	}
	return 0
}

func ps6101CompareBounds(left ps6101Value, operation token.Token, right ps6101Value) int {
	leftBounds, rightBounds := left.analysisValue(), right.analysisValue()
	if operation == token.EQL || operation == token.NEQ {
		excluded := right.constant != nil && leftBounds.excluded[right.constant.ExactString()] ||
			left.constant != nil && rightBounds.excluded[left.constant.ExactString()]
		if excluded {
			if operation == token.EQL {
				return -1
			}
			return 1
		}
	}
	if leftBounds.lower == nil || leftBounds.upper == nil || rightBounds.lower == nil || rightBounds.upper == nil {
		return 0
	}
	definitely := func(op token.Token, first, second constant.Value) bool {
		return constant.Compare(first, op, second)
	}
	switch operation {
	case token.LSS:
		if definitely(token.LSS, leftBounds.upper, rightBounds.lower) {
			return 1
		}
		if definitely(token.GEQ, leftBounds.lower, rightBounds.upper) {
			return -1
		}
	case token.LEQ:
		if definitely(token.LEQ, leftBounds.upper, rightBounds.lower) {
			return 1
		}
		if definitely(token.GTR, leftBounds.lower, rightBounds.upper) {
			return -1
		}
	case token.GTR:
		return ps6101CompareBounds(right, token.LSS, left)
	case token.GEQ:
		return ps6101CompareBounds(right, token.LEQ, left)
	case token.EQL:
		if definitely(token.LSS, leftBounds.upper, rightBounds.lower) || definitely(token.GTR, leftBounds.lower, rightBounds.upper) {
			return -1
		}
		if definitely(token.EQL, leftBounds.lower, leftBounds.upper) && definitely(token.EQL, rightBounds.lower, rightBounds.upper) &&
			definitely(token.EQL, leftBounds.lower, rightBounds.lower) {
			return 1
		}
	case token.NEQ:
		return -ps6101CompareBounds(left, token.EQL, right)
	}
	return 0
}

func ps6101SymmetricRandomCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	return ok && ps6101RandFunction(pass, selector, "NormFloat64", "NormFloat32")
}

func ps6101CenteredRandomBound(pass *analysis.Pass, expression *ast.BinaryExpr) (float64, bool) {
	if expression == nil || expression.Op != token.SUB {
		return 0, false
	}
	leftRandom := ps6101UniformRandomCall(pass, expression.X)
	rightRandom := ps6101UniformRandomCall(pass, expression.Y)
	if leftRandom && rightRandom {
		return 1, true
	}
	if leftRandom && ps6101HalfConstant(pass, expression.Y) || rightRandom && ps6101HalfConstant(pass, expression.X) {
		return 0.5, true
	}
	return 0, false
}

func ps6101HalfConstant(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[expression].Value
	return value != nil && constant.Compare(value, token.EQL, constant.MakeFloat64(0.5))
}

func ps6101UniformRandomCall(pass *analysis.Pass, expression ast.Expr) bool {
	call, ok := ps6101Unparen(expression).(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	return ok && ps6101RandFunction(pass, selector, "Float64", "Float32")
}

func ps6101RandFunction(pass *analysis.Pass, selector *ast.SelectorExpr, names ...string) bool {
	if pass == nil || pass.TypesInfo == nil || selector == nil {
		return false
	}
	var function *types.Func
	if selection := pass.TypesInfo.Selections[selector]; selection != nil {
		function, _ = selection.Obj().(*types.Func)
	} else {
		function, _ = pass.TypesInfo.Uses[selector.Sel].(*types.Func)
	}
	if function == nil || function.Pkg() == nil {
		return false
	}
	path := function.Pkg().Path()
	if path != "math/rand" && path != "math/rand/v2" {
		return false
	}
	for _, name := range names {
		if function.Name() == name {
			return true
		}
	}
	return false
}

func ps6101AbsCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Abs" || len(call.Args) != 1 {
		return false
	}
	function, ok := pass.TypesInfo.Uses[selector.Sel].(*types.Func)
	return ok && function.Pkg() != nil && function.Pkg().Path() == "math" && function.Name() == "Abs"
}

func ps6101BenchmarkInputName(name string) bool {
	lower := strings.ToLower(name)
	for _, keyword := range ps6101BenchmarkInputKeywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

func ps6101AggregateName(name string) bool {
	lower := strings.ToLower(name)
	for _, keyword := range ps6101AggregateKeywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

func ps6101ThresholdName(name string) bool {
	return strings.Contains(strings.ToLower(name), "threshold")
}

func ps6101BranchCounterName(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "hot") || strings.Contains(lower, "branch") || strings.Contains(lower, "entered")
}

func ps6101ExpressionName(expression ast.Expr) string {
	switch value := ps6101Unparen(expression).(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	case *ast.IndexExpr:
		return ps6101ExpressionName(value.X)
	case *ast.SliceExpr:
		return ps6101ExpressionName(value.X)
	case *ast.StarExpr:
		return ps6101ExpressionName(value.X)
	case *ast.TypeAssertExpr:
		return ps6101ExpressionName(value.X)
	}
	return ""
}

func ps6101Unparen(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.X
	}
}

func (engine *ps6101Engine) location(expression ast.Expr, followAlias bool) (ps6101Location, bool) {
	expression = ps6101Unparen(expression)
	switch value := expression.(type) {
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return engine.location(value.X, false)
		}
	case *ast.StarExpr:
		return engine.location(value.X, true)
	case *ast.Ident:
		object := identObject(engine.pass, value)
		if object == nil {
			return ps6101Location{}, false
		}
		if followAlias {
			if alias, ok := engine.aliases[object]; ok {
				return alias, true
			}
		}
		location := ps6101Location{root: object}
		if followAlias {
			return engine.followReference(location), true
		}
		return location, true
	case *ast.IndexExpr:
		base, ok := engine.location(value.X, true)
		if !ok {
			return ps6101Location{}, false
		}
		index := engine.indexConstant(value.Index)
		if index == nil {
			return base, true
		}
		if cached, present := engine.evalCache[ps6101Unparen(value.X)]; present && cached.offsetOK {
			if exact, ok := constant.Int64Val(index); ok {
				index = constant.MakeInt64(exact + cached.offset)
			}
		}
		base.path += "[" + index.ExactString() + "]"
		if followAlias {
			base = engine.followReference(base)
		}
		return base, true
	case *ast.SliceExpr:
		return engine.location(value.X, followAlias)
	case *ast.TypeAssertExpr:
		if cached, present := engine.evalCache[expression]; present && cached.reference != nil {
			location := *cached.reference
			if followAlias {
				location = engine.followReference(location)
			}
			return location, true
		}
		raw, ok := engine.location(value.X, false)
		if !ok {
			return ps6101Location{}, false
		}
		stored := engine.state[raw]
		if !engine.assertionCompatible(stored.analysisValue().dynamic, engine.typeAssertionTarget(value)) || stored.reference == nil {
			return ps6101Location{}, false
		}
		if followAlias {
			return engine.followReference(raw), true
		}
		return raw, true
	case *ast.CallExpr:
		if cached, present := engine.evalCache[expression]; present && cached.reference != nil {
			location := *cached.reference
			if followAlias {
				location = engine.followReference(location)
			}
			return location, true
		}
		if len(value.Args) == 1 {
			typed := engine.pass.TypesInfo.Types[value.Fun]
			if typed.IsType() && ps6101ContainsReference(typed.Type) &&
				ps6101ContainsReference(engine.pass.TypesInfo.TypeOf(value.Args[0])) {
				return engine.location(value.Args[0], followAlias)
			}
		}
	case *ast.SelectorExpr:
		base, ok := engine.location(value.X, true)
		if !ok {
			if cached, present := engine.evalCache[expression]; present && cached.reference != nil {
				location := *cached.reference
				if followAlias {
					location = engine.followReference(location)
				}
				return location, true
			}
			return ps6101Location{}, false
		}
		fields := []string{value.Sel.Name}
		if selection := engine.pass.TypesInfo.Selections[value]; selection != nil {
			if path := ps6101SelectionFieldPath(selection); len(path) > 0 {
				fields = path
			} else if selection.Obj().Pkg() != nil {
				fields[0] = selection.Obj().Pkg().Path() + "." + selection.Obj().Name()
			}
		}
		for _, field := range fields {
			if base.path == "" {
				base.path = field
			} else {
				base.path += "." + field
			}
		}
		if followAlias {
			base = engine.followReference(base)
		}
		return base, true
	}
	return ps6101Location{}, false
}

func ps6101SelectionFieldPath(selection *types.Selection) []string {
	if selection == nil || selection.Kind() != types.FieldVal {
		return nil
	}
	typ := selection.Recv()
	path := make([]string, 0, len(selection.Index()))
	for _, index := range selection.Index() {
		for {
			typ = types.Unalias(typ)
			switch value := typ.(type) {
			case *types.Pointer:
				typ = value.Elem()
				continue
			case *types.Named:
				typ = value.Underlying()
				continue
			}
			break
		}
		structure, ok := typ.Underlying().(*types.Struct)
		if !ok || index < 0 || index >= structure.NumFields() {
			return nil
		}
		field := structure.Field(index)
		path = append(path, ps6101FieldName(field))
		typ = field.Type()
	}
	return path
}

func ps6101FieldName(field *types.Var) string {
	if field == nil {
		return ""
	}
	name := field.Name()
	if field.Pkg() != nil {
		name = field.Pkg().Path() + "." + name
	}
	return name
}

func (engine *ps6101Engine) indexConstant(expression ast.Expr) constant.Value {
	if expression == nil {
		return nil
	}
	if value := engine.pass.TypesInfo.Types[expression].Value; value != nil {
		return value
	}
	if location, ok := engine.location(expression, true); ok {
		return engine.state[location].constant
	}
	return nil
}

func (engine *ps6101Engine) followReference(location ps6101Location) ps6101Location {
	seen := make(map[ps6101Location]bool)
	for location.root != nil && !seen[location] {
		seen[location] = true
		reference := engine.state[location].reference
		if reference == nil {
			break
		}
		location = *reference
	}
	return location
}

func (engine *ps6101Engine) killPrefix(prefix ps6101Location) {
	for location := range engine.state {
		if ps6101HasLocationPrefix(location, prefix) {
			delete(engine.state, location)
		}
	}
}

func (engine *ps6101Engine) copyPrefix(source, destination ps6101Location) {
	type entry struct {
		location ps6101Location
		value    ps6101Value
	}
	var copied []entry
	parent := engine.state[destination]
	for location, value := range engine.state {
		if !ps6101HasLocationPrefix(location, source) {
			continue
		}
		suffix := strings.TrimPrefix(location.path, source.path)
		suffix = strings.TrimPrefix(suffix, ".")
		if suffix == "" {
			// The direct destination value was already assigned with its own
			// eligibility/aggregate classification. Only copy nested fields.
			continue
		}
		path := suffix
		if destination.path != "" {
			separator := "."
			if strings.HasPrefix(suffix, "[") {
				separator = ""
			}
			path = destination.path + separator + suffix
		}
		value = ps6101CloneValue(value)
		value.eligible = value.eligible || parent.eligible
		value.aggregate = value.aggregate || parent.aggregate
		copied = append(copied, entry{location: ps6101Location{root: destination.root, path: path}, value: value})
	}
	for index := range copied {
		item := &copied[index]
		engine.nextRevision++
		item.value.revision = engine.nextRevision
		engine.state[item.location] = item.value
	}
}

func ps6101HasLocationPrefix(location, prefix ps6101Location) bool {
	if location.root != prefix.root {
		return false
	}
	if prefix.path == "" {
		return true
	}
	if location.path == prefix.path || !strings.HasPrefix(location.path, prefix.path) {
		return location.path == prefix.path
	}
	remainder := strings.TrimPrefix(location.path, prefix.path)
	return strings.HasPrefix(remainder, ".") || strings.HasPrefix(remainder, "[")
}

func ps6101SourceSignature(sources map[token.Pos]bool) string {
	positions := make([]int, 0, len(sources))
	for source := range sources {
		positions = append(positions, int(source))
	}
	slices.Sort(positions)
	var builder strings.Builder
	builder.Grow(len(positions) * 12)
	for _, position := range positions {
		builder.WriteString(strconv.Itoa(position))
		builder.WriteByte(',')
	}
	return builder.String()
}

func (engine *ps6101Engine) cloneState() map[ps6101Location]ps6101Value {
	return engine.copyState(engine.state)
}

func (engine *ps6101Engine) copyState(source map[ps6101Location]ps6101Value) map[ps6101Location]ps6101Value {
	copy := make(map[ps6101Location]ps6101Value, len(source))
	for location, value := range source {
		copy[location] = ps6101CloneValue(value)
	}
	return copy
}

func (engine *ps6101Engine) cloneAliases() map[types.Object]ps6101Location {
	return engine.copyAliases(engine.aliases)
}

func (engine *ps6101Engine) copyAliases(source map[types.Object]ps6101Location) map[types.Object]ps6101Location {
	copy := make(map[types.Object]ps6101Location, len(source))
	for object, location := range source {
		copy[object] = location
	}
	return copy
}

func ps6101MergeStates(left, right map[ps6101Location]ps6101Value) map[ps6101Location]ps6101Value {
	merged := make(map[ps6101Location]ps6101Value)
	for location, leftValue := range left {
		rightValue, ok := right[location]
		if ok && ps6101SameValue(leftValue, rightValue) {
			merged[location] = ps6101CloneValue(leftValue)
		}
	}
	return merged
}

func ps6101MergeAliases(left, right map[types.Object]ps6101Location) map[types.Object]ps6101Location {
	merged := make(map[types.Object]ps6101Location)
	for object, location := range left {
		if right[object] == location {
			merged[object] = location
		}
	}
	return merged
}

func ps6101SameValue(left, right ps6101Value) bool {
	leftAnalysis, rightAnalysis := left.analysisValue(), right.analysisValue()
	return left.kind == right.kind && left.eligible == right.eligible && left.aggregate == right.aggregate &&
		left.nonempty == right.nonempty && left.threshold == right.threshold && left.testing == right.testing &&
		left.revision == right.revision && left.identity == right.identity && leftAnalysis.squareID == rightAnalysis.squareID &&
		leftAnalysis.squareSig == rightAnalysis.squareSig && leftAnalysis.squareSign == rightAnalysis.squareSign && ps6101SameReference(left.reference, right.reference) &&
		left.callable == right.callable && ps6101SameDynamicType(leftAnalysis.dynamic, rightAnalysis.dynamic) && left.length == right.length && left.capacity == right.capacity && left.offset == right.offset &&
		left.lengthOK == right.lengthOK && left.capacityOK == right.capacityOK && left.offsetOK == right.offsetOK &&
		ps6101SameElements(left.elements, right.elements) && maps.EqualFunc(leftAnalysis.fields, rightAnalysis.fields, ps6101SameValue) &&
		ps6101SourceSignature(left.sources) == ps6101SourceSignature(right.sources) &&
		ps6101SameConstant(left.constant, right.constant) && ps6101SameConstant(leftAnalysis.lower, rightAnalysis.lower) &&
		ps6101SameConstant(leftAnalysis.upper, rightAnalysis.upper) && maps.Equal(leftAnalysis.excluded, rightAnalysis.excluded)
}

func ps6101SameDynamicType(left, right types.Type) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return types.Identical(left, right)
}

func ps6101SameElements(left, right map[int64]ps6101Value) bool {
	return maps.EqualFunc(left, right, ps6101SameValue)
}

func ps6101Reference(location ps6101Location) *ps6101Location {
	if location.root == nil {
		return nil
	}
	return &location
}

func ps6101SameReference(left, right *ps6101Location) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func ps6101SameConstant(left, right constant.Value) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.ExactString() == right.ExactString()
}

func (engine *ps6101Engine) mergeReturns(returns [][]ps6101Value) []ps6101Value {
	if len(returns) == 0 {
		return nil
	}
	width := len(returns[0])
	for _, values := range returns[1:] {
		if len(values) != width {
			return make([]ps6101Value, width)
		}
	}
	result := make([]ps6101Value, width)
	for index := 0; index < width; index++ {
		result[index] = ps6101CloneValue(returns[0][index])
		for _, values := range returns[1:] {
			if !ps6101SameValue(result[index], values[index]) {
				result[index] = ps6101Value{}
				break
			}
		}
	}
	return result
}

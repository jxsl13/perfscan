package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6106 implements owner issue #886. It reports a bounded producer followed
// by a wider elementwise scratch consumer and only bounded later observers.
var PS6106 = register(&lint.Check{
	ID:       "PS6106",
	Category: "verify",
	Slug:     "capacity-wide-consumer-after-bounded-scratch-write",
	Level:    lint.LevelAggressive,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "elementwise scratch consumer traverses beyond the active prefix",
		Text: `Scratch storage is often sized for a maximum workload while one hot
invocation writes and later consumes only a smaller active prefix. A following
elementwise operation over the complete logical slice length or a configured
device capacity can perform substantially more work than the dataflow needs.

PS6106 deliberately accepts two narrow proof sources. For ordinary Go slices,
it requires a fresh private local or uniquely assignment-initialized unexported field, a
fixed positive make length, direct package-local float32/float64 helpers with
one canonical same-index range loop, full-cap-clamped producer and observer
slices, and a raw-slice elementwise consumer. The consumer extent is the Go
slice length, NOT its backing capacity. For opaque device buffers, an explicit
boundedScratchFlowContracts entry must bind exact typed methods, argument
roles, one concrete provider, ownership, active-prefix, full-capacity,
non-retention, synchronous-execution, observer, and inactive-tail promises.
Function names never establish those semantics.

The documented After example uses a separate extent-aware consumer. The
retained microbenchmark instead isolates the other documented remedy, a
bounded fused producer/consumer epilogue; its evidence is labeled accordingly.

Every use of the scratch object must be classified. Aliases, address exposure,
interface dispatch, cap-sensitive operations, tail observations, alternate
field initialization, unstable extents, different receiver instances,
unreachable or non-straight-line calls, and unknown uses suppress the finding.
Source ratios use exact constants. Dynamic opaque extents may use a paired
configured workload value, but producer and observer calls must still carry
the same stable runtime extent expression. Static enclosing repetitions and
configured model repetitions are labeled separately; neither is a runtime
invocation count inferred from a name.

There is NO automatic fix. Prefer an extent-aware consumer or a bounded fused
epilogue only after validating active-prefix bits, untouched tail, provider and
fallback behavior, errors, synchronization, dispatch structure, and the real
workload crossover. A configured replacement is named only when all of those
contract promises are explicit, and remains advisory.`,
		Before: `scratch := make([]float32, 1024)
write(scratch[:16:16])
activate(scratch)       // ranges over all 1024 elements
observe(scratch[:16:16])`,
		After: `scratch := make([]float32, 1024)
write(scratch[:16:16])
activateN(scratch, 16) // project-validated bounded path
observe(scratch[:16:16])`,
		MeasuredWin: `Owner issue #886 replaced a bounded bias operation plus a
capacity-wide GELU dispatch with a bounded fused epilogue. On Apple M2 Pro the
owner reported 21/21 paired single-token wins, a 1.115699x paired median, exact
active-prefix parity, unchanged inactive tail, and unchanged allocation counts.
The owner workload's configured work ratio was 3,145,728/3,072 = 1024x across
12 decoder blocks. Treat those as attributed project evidence, not a general
timing guarantee.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6106",
		Doc:  "bounded scratch production is followed by a wider elementwise consumer and only bounded observers",
		Run:  runPS6106,
	},
})

type ps6106Storage struct {
	root   types.Object
	fields []*types.Var
	typeOf types.Type
}

type ps6106Sequence struct {
	producer  *ast.CallExpr
	consumer  *ast.CallExpr
	observers []*ast.CallExpr
	buffer    ps6106Storage
	provider  ps6106Storage
	active    ast.Expr
	allowed   []ast.Expr
	contract  *config.BoundedScratchFlowContract
}

type ps6106Initializer struct {
	lhs        ast.Expr
	value      ast.Expr
	fullExtent ast.Expr
	provider   ps6106Storage
	function   *ast.FuncDecl
	root       types.Object
}

type ps6106RoleKey struct {
	function *types.Func
	buffer   *types.Var
	role     ps6106Role
}

type ps6106FieldInitKey struct {
	field    *types.Var
	contract *config.BoundedScratchFlowContract
	function *ast.FuncDecl
}

type ps6106FieldInitResult struct {
	initializer ps6106Initializer
	ok          bool
}

type ps6106Analysis struct {
	pass           *analysis.Pass
	locals         map[*types.Func]*ast.FuncDecl
	objectUses     map[types.Object][]*ast.Ident
	roleProofs     map[ps6106RoleKey]bool
	roleKnown      map[ps6106RoleKey]bool
	fieldInitProof map[ps6106FieldInitKey]ps6106FieldInitResult
}

func runPS6106(pass *analysis.Pass) (any, error) {
	return runPS6106WithContracts(pass, config.Current().BoundedScratchFlowContracts)
}

func runPS6106WithContracts(pass *analysis.Pass, contracts []config.BoundedScratchFlowContract) (any, error) {
	state := &ps6106Analysis{
		pass:           pass,
		locals:         ps6106LocalFunctions(pass),
		objectUses:     ps6106ObjectUses(pass),
		roleProofs:     make(map[ps6106RoleKey]bool),
		roleKnown:      make(map[ps6106RoleKey]bool),
		fieldInitProof: make(map[ps6106FieldInitKey]ps6106FieldInitResult),
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			parents := ps6087Parents(fn.Body)
			reachable := ps6099ReachableNodesInBlock(pass, fn.Body, parents)
			ps6032Blocks(fn.Body, func(block *ast.BlockStmt) {
				for index := 0; index+2 < len(block.List); index++ {
					if sequence, ok := ps6106LocalSequence(state, block, index, reachable); ok {
						ps6106CheckAndReport(state, fn, &sequence, nil, parents, reachable)
						continue
					}
					for contractIndex := range contracts {
						contract := &contracts[contractIndex]
						if !contract.Valid() {
							continue
						}
						if sequence, ok := ps6106ContractSequence(pass, block, index, contract, reachable); ok {
							ps6106CheckAndReport(state, fn, &sequence, contract, parents, reachable)
							break
						}
					}
				}
			})
		}
	}
	return nil, nil
}

func ps6106ObjectUses(pass *analysis.Pass) map[types.Object][]*ast.Ident {
	result := make(map[types.Object][]*ast.Ident)
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			object := pass.TypesInfo.Uses[identifier]
			if object == nil {
				object = pass.TypesInfo.Defs[identifier]
			}
			if object != nil {
				result[object] = append(result[object], identifier)
			}
			return true
		})
	}
	return result
}

func ps6106LocalFunctions(pass *analysis.Pass) map[*types.Func]*ast.FuncDecl {
	result := make(map[*types.Func]*ast.FuncDecl)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			object, _ := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			if object != nil {
				result[object.Origin()] = fn
			}
		}
	}
	return result
}

func ps6106StatementCall(statement ast.Stmt) *ast.CallExpr {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return nil
	}
	call, _ := ps2110Unparen(expression.X).(*ast.CallExpr)
	return call
}

func ps6106LocalSequence(state *ps6106Analysis, block *ast.BlockStmt, index int, reachable map[ast.Node]bool) (ps6106Sequence, bool) {
	pass := state.pass
	producer := ps6106StatementCall(block.List[index])
	consumer := ps6106StatementCall(block.List[index+1])
	if producer == nil || consumer == nil || !reachable[producer] || !reachable[consumer] {
		return ps6106Sequence{}, false
	}
	producerArg, producerStorage, active, ok := ps6106BoundedArgument(pass, producer)
	if !ok || !state.localRole(producer, producerArg, ps6106ProducerRole) {
		return ps6106Sequence{}, false
	}
	consumerArg, consumerStorage, ok := ps6106UniqueRawStorageArgument(pass, consumer)
	if !ok || !ps6106SameStorage(producerStorage, consumerStorage) ||
		!state.localRole(consumer, consumerArg, ps6106ConsumerRole) {
		return ps6106Sequence{}, false
	}
	sequence := ps6106Sequence{
		producer: producer,
		consumer: consumer,
		buffer:   producerStorage,
		active:   active,
		allowed:  []ast.Expr{producerArg, consumerArg},
	}
	for observerIndex := index + 2; observerIndex < len(block.List); observerIndex++ {
		observer := ps6106StatementCall(block.List[observerIndex])
		if observer == nil || !reachable[observer] {
			break
		}
		observerArg, storage, observerActive, bounded := ps6106BoundedArgument(pass, observer)
		if !bounded || !ps6106SameStorage(sequence.buffer, storage) ||
			!ps6106SameExtent(pass, sequence.active, observerActive) ||
			!state.localRole(observer, observerArg, ps6106ObserverRole) {
			break
		}
		sequence.observers = append(sequence.observers, observer)
		sequence.allowed = append(sequence.allowed, observerArg)
	}
	return sequence, len(sequence.observers) > 0
}

func ps6106ContractSequence(pass *analysis.Pass, block *ast.BlockStmt, index int, contract *config.BoundedScratchFlowContract, reachable map[ast.Node]bool) (ps6106Sequence, bool) {
	producer := ps6106StatementCall(block.List[index])
	consumer := ps6106StatementCall(block.List[index+1])
	if producer == nil || consumer == nil || !reachable[producer] || !reachable[consumer] ||
		ps6087FunctionID(pass, producer) != contract.Producer ||
		ps6087FunctionID(pass, consumer) != contract.Consumer {
		return ps6106Sequence{}, false
	}
	producerBuffer, producerBufferExpr, producerActive, provider, ok := ps6106ContractCall(pass, producer, contract.ProducerBufferArg, contract.ProducerActiveArg)
	if !ok {
		return ps6106Sequence{}, false
	}
	consumerBuffer, consumerBufferExpr, _, consumerProvider, ok := ps6106ContractCall(pass, consumer, contract.ConsumerBufferArg, 0)
	if !ok || !ps6106SameStorage(producerBuffer, consumerBuffer) || !ps6106SameStorage(provider, consumerProvider) {
		return ps6106Sequence{}, false
	}
	sequence := ps6106Sequence{
		producer: producer,
		consumer: consumer,
		buffer:   producerBuffer,
		provider: provider,
		active:   producerActive,
		allowed:  []ast.Expr{producerBufferExpr, consumerBufferExpr},
		contract: contract,
	}
	for observerIndex := index + 2; observerIndex < len(block.List); observerIndex++ {
		observer := ps6106StatementCall(block.List[observerIndex])
		if observer == nil || !reachable[observer] {
			break
		}
		observerContract := ps6106ObserverContract(pass, observer, contract.Observers)
		if observerContract == nil {
			break
		}
		buffer, bufferExpr, active, observerProvider, valid := ps6106ContractCall(pass, observer, observerContract.BufferArg, observerContract.ActiveArg)
		if !valid || !ps6106SameStorage(sequence.buffer, buffer) || !ps6106SameStorage(sequence.provider, observerProvider) ||
			!ps6106SameExtent(pass, sequence.active, active) {
			break
		}
		sequence.observers = append(sequence.observers, observer)
		sequence.allowed = append(sequence.allowed, bufferExpr)
	}
	return sequence, len(sequence.observers) > 0
}

func ps6106ObserverContract(pass *analysis.Pass, call *ast.CallExpr, observers []config.BoundedScratchObserverContract) *config.BoundedScratchObserverContract {
	id := ps6087FunctionID(pass, call)
	for index := range observers {
		if observers[index].Function == id {
			return &observers[index]
		}
	}
	return nil
}

func ps6106StorageExpression(pass *analysis.Pass, expression ast.Expr) (ps6106Storage, bool) {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := pass.TypesInfo.Uses[value]
		if object == nil {
			object = pass.TypesInfo.Defs[value]
		}
		if _, ok := object.(*types.Var); !ok {
			return ps6106Storage{}, false
		}
		return ps6106Storage{root: object, typeOf: pass.TypesInfo.TypeOf(value)}, true
	case *ast.SelectorExpr:
		selection := pass.TypesInfo.Selections[value]
		if selection == nil || selection.Kind() != types.FieldVal || len(selection.Index()) != 1 {
			return ps6106Storage{}, false
		}
		field, _ := selection.Obj().(*types.Var)
		base, ok := ps6106StorageExpression(pass, value.X)
		if !ok || field == nil {
			return ps6106Storage{}, false
		}
		base.fields = append(base.fields, field)
		base.typeOf = pass.TypesInfo.TypeOf(value)
		return base, true
	default:
		return ps6106Storage{}, false
	}
}

func ps6106SameStorage(left, right ps6106Storage) bool {
	if left.root == nil || left.root != right.root || len(left.fields) != len(right.fields) ||
		!types.Identical(left.typeOf, right.typeOf) {
		return false
	}
	for index := range left.fields {
		if left.fields[index] != right.fields[index] {
			return false
		}
	}
	return true
}

func ps6106SameRelativePath(left, right ps6106Storage) bool {
	if len(left.fields) != len(right.fields) || !types.Identical(left.typeOf, right.typeOf) {
		return false
	}
	for index := range left.fields {
		if left.fields[index] != right.fields[index] {
			return false
		}
	}
	return true
}

func ps6106BoundedArgument(pass *analysis.Pass, call *ast.CallExpr) (ast.Expr, ps6106Storage, ast.Expr, bool) {
	var found ast.Expr
	var storage ps6106Storage
	var active ast.Expr
	for _, argument := range call.Args {
		slice, ok := ps2110Unparen(argument).(*ast.SliceExpr)
		if !ok || slice.Low != nil || slice.High == nil || slice.Max == nil ||
			!ps6106SameExtent(pass, slice.High, slice.Max) {
			continue
		}
		candidate, ok := ps6106StorageExpression(pass, slice.X)
		if !ok || found != nil {
			return nil, ps6106Storage{}, nil, false
		}
		found, storage, active = argument, candidate, slice.High
	}
	return found, storage, active, found != nil
}

func ps6106UniqueRawStorageArgument(pass *analysis.Pass, call *ast.CallExpr) (ast.Expr, ps6106Storage, bool) {
	var found ast.Expr
	var storage ps6106Storage
	for _, argument := range call.Args {
		candidate, ok := ps6106StorageExpression(pass, argument)
		if !ok || !ps6106FloatSlice(candidate.typeOf) {
			continue
		}
		if found != nil {
			return nil, ps6106Storage{}, false
		}
		found, storage = argument, candidate
	}
	return found, storage, found != nil
}

func ps6106ContractCall(pass *analysis.Pass, call *ast.CallExpr, bufferPosition, activePosition int) (ps6106Storage, ast.Expr, ast.Expr, ps6106Storage, bool) {
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function == nil || signature == nil || signature.Recv() == nil || signature.Variadic() ||
		signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 {
		return ps6106Storage{}, nil, nil, ps6106Storage{}, false
	}
	selector := ps6099CallSelector(call.Fun)
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.MethodVal {
		return ps6106Storage{}, nil, nil, ps6106Storage{}, false
	}
	provider, ok := ps6106StorageExpression(pass, selector.X)
	if !ok || ps6106Interface(provider.typeOf) || ps6087Named(provider.typeOf) == nil {
		return ps6106Storage{}, nil, nil, ps6106Storage{}, false
	}
	bufferIndex := bufferPosition - 1
	if bufferIndex < 0 || bufferIndex >= len(call.Args) || bufferIndex >= signature.Params().Len() {
		return ps6106Storage{}, nil, nil, ps6106Storage{}, false
	}
	bufferExpression := call.Args[bufferIndex]
	buffer, ok := ps6106StorageExpression(pass, bufferExpression)
	parameterType := signature.Params().At(bufferIndex).Type()
	if !ok || ps6106Interface(buffer.typeOf) || ps6106Interface(parameterType) ||
		!types.Identical(buffer.typeOf, parameterType) {
		return ps6106Storage{}, nil, nil, ps6106Storage{}, false
	}
	if activePosition == 0 {
		return buffer, bufferExpression, nil, provider, true
	}
	activeIndex := activePosition - 1
	if activeIndex < 0 || activeIndex >= len(call.Args) || activeIndex >= signature.Params().Len() || activeIndex == bufferIndex {
		return ps6106Storage{}, nil, nil, ps6106Storage{}, false
	}
	active := call.Args[activeIndex]
	if !ps6106Integer(pass.TypesInfo.TypeOf(active)) || !ps6106Integer(signature.Params().At(activeIndex).Type()) ||
		!types.AssignableTo(pass.TypesInfo.TypeOf(active), signature.Params().At(activeIndex).Type()) {
		return ps6106Storage{}, nil, nil, ps6106Storage{}, false
	}
	return buffer, bufferExpression, active, provider, true
}

func ps6106FloatSlice(value types.Type) bool {
	slice, ok := types.Unalias(value).Underlying().(*types.Slice)
	if !ok {
		return false
	}
	basic, ok := types.Unalias(slice.Elem()).Underlying().(*types.Basic)
	return ok && (basic.Kind() == types.Float32 || basic.Kind() == types.Float64)
}

func ps6106Integer(value types.Type) bool {
	if value == nil {
		return false
	}
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsInteger != 0
}

func ps6106Interface(value types.Type) bool {
	if value == nil {
		return false
	}
	_, ok := types.Unalias(value).Underlying().(*types.Interface)
	return ok
}

type ps6106Role uint8

const (
	ps6106ProducerRole ps6106Role = iota
	ps6106ConsumerRole
	ps6106ObserverRole
)

func (state *ps6106Analysis) localRole(call *ast.CallExpr, bufferArgument ast.Expr, role ps6106Role) bool {
	pass := state.pass
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function.Pkg() != pass.Pkg || signature.Variadic() ||
		signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 ||
		!ps6099ConcreteLeafCallee(pass, call, function, signature) {
		return false
	}
	offset, ok := ps6099CallSignatureOffset(pass, call, signature)
	if !ok {
		return false
	}
	argumentIndex := -1
	for index, argument := range call.Args {
		if argument == bufferArgument {
			argumentIndex = index
			break
		}
	}
	parameterIndex := argumentIndex - offset
	if parameterIndex < 0 || parameterIndex >= signature.Params().Len() {
		return false
	}
	buffer := signature.Params().At(parameterIndex)
	if !ps6106FloatSlice(buffer.Type()) || ps6106Interface(buffer.Type()) ||
		!types.AssignableTo(pass.TypesInfo.TypeOf(bufferArgument), buffer.Type()) {
		return false
	}
	key := ps6106RoleKey{function: function.Origin(), buffer: buffer, role: role}
	if state.roleKnown[key] {
		return state.roleProofs[key]
	}
	declaration := state.locals[function.Origin()]
	proof := declaration != nil && ps6106RoleBody(pass, declaration, buffer, role)
	state.roleKnown[key], state.roleProofs[key] = true, proof
	return proof
}

func ps6106RoleBody(pass *analysis.Pass, declaration *ast.FuncDecl, buffer *types.Var, role ps6106Role) bool {
	if declaration.Body == nil || len(declaration.Body.List) != 1 {
		return false
	}
	loop, ok := declaration.Body.List[0].(*ast.RangeStmt)
	if !ok || loop.Value != nil || loop.Tok != token.DEFINE || len(loop.Body.List) != 1 ||
		!ps6106ObjectExpression(pass, loop.X, buffer) {
		return false
	}
	indexID, ok := ps2110Unparen(loop.Key).(*ast.Ident)
	if !ok {
		return false
	}
	index := pass.TypesInfo.Defs[indexID]
	assignment, ok := loop.Body.List[0].(*ast.AssignStmt)
	if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return false
	}
	parents := ps6087Parents(declaration.Body)
	if !ps6106AllBufferUsesIndexed(pass, declaration.Body, buffer, index, loop.X, parents) {
		return false
	}
	lhsIsBuffer := ps6106BufferIndex(pass, assignment.Lhs[0], buffer, index)
	stableScalars := ps6106StableScalarParameters(pass, declaration, buffer)
	rhsUsesBuffer := ps6106Arithmetic(pass, assignment.Rhs[0], buffer, index, stableScalars)
	if !rhsUsesBuffer.valid {
		return false
	}
	switch role {
	case ps6106ProducerRole:
		return assignment.Tok == token.ASSIGN && lhsIsBuffer
	case ps6106ConsumerRole:
		if !lhsIsBuffer {
			return false
		}
		switch assignment.Tok {
		case token.ADD_ASSIGN, token.SUB_ASSIGN, token.MUL_ASSIGN:
			return true
		case token.ASSIGN:
			return rhsUsesBuffer.bufferRead
		default:
			return false
		}
	case ps6106ObserverRole:
		blank, blankOK := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		return assignment.Tok == token.ASSIGN && blankOK && blank.Name == "_" && rhsUsesBuffer.bufferRead
	default:
		return false
	}
}

func ps6106StableScalarParameters(pass *analysis.Pass, declaration *ast.FuncDecl, buffer *types.Var) map[types.Object]bool {
	result := make(map[types.Object]bool)
	function, _ := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	if function == nil {
		return result
	}
	signature, _ := function.Type().(*types.Signature)
	for index := 0; signature != nil && index < signature.Params().Len(); index++ {
		parameter := signature.Params().At(index)
		if parameter != buffer && ps6106FloatScalar(parameter.Type()) {
			result[parameter] = true
		}
	}
	return result
}

type ps6106ArithmeticResult struct {
	valid      bool
	bufferRead bool
}

func ps6106Arithmetic(pass *analysis.Pass, expression ast.Expr, buffer, index types.Object, stableScalars map[types.Object]bool) ps6106ArithmeticResult {
	expression = ps2110Unparen(expression)
	if ps6106BufferIndex(pass, expression, buffer, index) {
		return ps6106ArithmeticResult{valid: true, bufferRead: true}
	}
	switch value := expression.(type) {
	case *ast.BasicLit:
		return ps6106ArithmeticResult{valid: value.Kind == token.INT || value.Kind == token.FLOAT}
	case *ast.Ident:
		object := pass.TypesInfo.Uses[value]
		if object == nil {
			return ps6106ArithmeticResult{}
		}
		if _, ok := object.(*types.Const); ok {
			return ps6106ArithmeticResult{valid: true}
		}
		return ps6106ArithmeticResult{valid: stableScalars[object]}
	case *ast.UnaryExpr:
		if value.Op != token.ADD && value.Op != token.SUB {
			return ps6106ArithmeticResult{}
		}
		return ps6106Arithmetic(pass, value.X, buffer, index, stableScalars)
	case *ast.BinaryExpr:
		if value.Op != token.ADD && value.Op != token.SUB && value.Op != token.MUL {
			return ps6106ArithmeticResult{}
		}
		left := ps6106Arithmetic(pass, value.X, buffer, index, stableScalars)
		right := ps6106Arithmetic(pass, value.Y, buffer, index, stableScalars)
		return ps6106ArithmeticResult{valid: left.valid && right.valid, bufferRead: left.bufferRead || right.bufferRead}
	default:
		return ps6106ArithmeticResult{}
	}
}

func ps6106FloatScalar(value types.Type) bool {
	if value == nil {
		return false
	}
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && (basic.Kind() == types.Float32 || basic.Kind() == types.Float64)
}

func ps6106AllBufferUsesIndexed(pass *analysis.Pass, body *ast.BlockStmt, buffer, index types.Object, rangeExpression ast.Expr, parents map[ast.Node]ast.Node) bool {
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[identifier] != buffer {
			return true
		}
		if identifier == ps2110Unparen(rangeExpression) {
			return true
		}
		parent := parents[identifier]
		for {
			_, ok := parent.(*ast.ParenExpr)
			if !ok {
				break
			}
			parent = parents[parent]
		}
		indexed, ok := parent.(*ast.IndexExpr)
		if !ok || !ps6106BufferIndex(pass, indexed, buffer, index) {
			valid = false
		}
		return true
	})
	return valid
}

func ps6106BufferIndex(pass *analysis.Pass, expression ast.Expr, buffer, index types.Object) bool {
	indexed, ok := ps2110Unparen(expression).(*ast.IndexExpr)
	return ok && ps6106ObjectExpression(pass, indexed.X, buffer) && ps6106ObjectExpression(pass, indexed.Index, index)
}

func ps6106ObjectExpression(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && pass.TypesInfo.Uses[identifier] == object
}

func ps6106CheckAndReport(state *ps6106Analysis, fn *ast.FuncDecl, sequence *ps6106Sequence, contract *config.BoundedScratchFlowContract, parents map[ast.Node]ast.Node, reachable map[ast.Node]bool) {
	pass := state.pass
	if !ps6106ExtentStable(pass, fn.Body, sequence.active) {
		return
	}
	initializer, ok := ps6106FindInitializer(state, fn, sequence, contract, reachable)
	if !ok || !ps6106CompleteUseClosure(state, fn, sequence, &initializer) ||
		len(sequence.buffer.fields) > 0 && (!ps6106SequenceRootClosed(state, fn, sequence) ||
			!ps6106WholeOwnerClosed(state, fn, sequence, &initializer)) {
		return
	}
	full, fullSource := ps6106PositiveConstant(pass, initializer.fullExtent)
	active, activeSource := ps6106PositiveConstant(pass, sequence.active)
	configured := false
	if contract != nil {
		if fullSource && contract.ConfiguredCapacityElements > 0 && full != contract.ConfiguredCapacityElements ||
			activeSource && contract.ConfiguredActiveElements > 0 && active != contract.ConfiguredActiveElements {
			return
		}
		if !fullSource || !activeSource {
			if contract.ConfiguredCapacityElements <= 0 || contract.ConfiguredActiveElements <= 0 {
				return
			}
			full, active = contract.ConfiguredCapacityElements, contract.ConfiguredActiveElements
			configured = true
		}
	}
	if !fullSource && contract == nil || !activeSource && contract == nil || full <= active || full/active < 2 {
		return
	}
	if contract != nil && !ps6106ContractProviderProof(state, fn, sequence, &initializer, contract) {
		return
	}
	ratio := ps6106Ratio(full, active)
	repetition := ps6106StaticRepetitions(pass, sequence.consumer, parents)
	repeatText := "one static source site; runtime invocation count unknown"
	if repetition > 1 {
		repeatText = fmt.Sprintf("%d source-proven static enclosing repetitions; runtime invocation count unknown", repetition)
	}
	if contract != nil && contract.ConfiguredRepeatCount > 0 {
		repeatText += fmt.Sprintf("; %d configured model repetitions", contract.ConfiguredRepeatCount)
	}
	extentKind := "Go slice logical length"
	if contract != nil {
		extentKind = "configured opaque full-capacity extent"
		if !configured {
			extentKind = "source-constant opaque full-capacity extent"
		}
	}
	remedy := "an extent-aware consumer or bounded fused epilogue"
	if contract != nil && contract.Replacement != "" && ps6106ReplacementOnProvider(pass, sequence.provider.typeOf, contract.Replacement) {
		remedy = contract.Replacement + " (configured equivalent replacement; advisory only)"
	}
	label := "source-proven"
	if configured {
		label = "configured workload"
	}
	pass.Reportf(sequence.consumer.Pos(), "%s: bounded scratch writes and later observers use %d active elements, but the %s consumer traverses %d elements (%s, %s ratio); %s — evaluate %s after validating prefix bits, inactive tail, provider/fallback behavior, errors, synchronization, dispatch structure, and workload crossover (advisory, no automatic fix)",
		ps6106ContractName(contract), active, extentKind, full, label, ratio, repeatText, remedy)
}

func ps6106ContractName(contract *config.BoundedScratchFlowContract) string {
	if contract == nil {
		return "local-float-scratch"
	}
	return contract.Name
}

func ps6106Ratio(full, active int64) string {
	if full%active == 0 {
		return strconv.FormatInt(full/active, 10) + "x"
	}
	return strconv.FormatFloat(float64(full)/float64(active), 'f', 3, 64) + "x"
}

func ps6106PositiveConstant(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	if expression == nil {
		return 0, false
	}
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	if value == nil || value.Kind() != constant.Int {
		return 0, false
	}
	integer, exact := constant.Int64Val(value)
	if !exact || integer <= 0 {
		return 0, false
	}
	intBytes := pass.TypesSizes.Sizeof(types.Typ[types.Int])
	if intBytes <= 0 || intBytes > 8 {
		return 0, false
	}
	maxInt := int64(math.MaxInt64)
	if intBytes < 8 {
		maxInt = int64(1)<<(uint(intBytes)*8-1) - 1
	}
	return integer, integer <= maxInt
}

func ps6106SameExtent(pass *analysis.Pass, left, right ast.Expr) bool {
	left, right = ps2110Unparen(left), ps2110Unparen(right)
	switch leftValue := left.(type) {
	case *ast.BasicLit:
		rightValue, ok := right.(*ast.BasicLit)
		return ok && leftValue.Kind == rightValue.Kind && leftValue.Value == rightValue.Value
	case *ast.Ident:
		rightValue, ok := right.(*ast.Ident)
		if !ok {
			return false
		}
		leftObject := pass.TypesInfo.Uses[leftValue]
		if leftObject == nil {
			leftObject = pass.TypesInfo.Defs[leftValue]
		}
		rightObject := pass.TypesInfo.Uses[rightValue]
		if rightObject == nil {
			rightObject = pass.TypesInfo.Defs[rightValue]
		}
		return leftObject != nil && leftObject == rightObject
	case *ast.UnaryExpr:
		rightValue, ok := right.(*ast.UnaryExpr)
		return ok && leftValue.Op == rightValue.Op && (leftValue.Op == token.ADD || leftValue.Op == token.SUB) &&
			ps6106SameExtent(pass, leftValue.X, rightValue.X)
	case *ast.BinaryExpr:
		rightValue, ok := right.(*ast.BinaryExpr)
		return ok && leftValue.Op == rightValue.Op &&
			(leftValue.Op == token.ADD || leftValue.Op == token.SUB || leftValue.Op == token.MUL) &&
			ps6106SameExtent(pass, leftValue.X, rightValue.X) && ps6106SameExtent(pass, leftValue.Y, rightValue.Y)
	default:
		return false
	}
}

func ps6106ExtentStable(pass *analysis.Pass, body *ast.BlockStmt, expression ast.Expr) bool {
	objects := make(map[types.Object]bool)
	valid := true
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := pass.TypesInfo.Uses[identifier]
		if object == nil {
			object = pass.TypesInfo.Defs[identifier]
		}
		switch object.(type) {
		case *types.Const:
		case *types.Var:
			if object.Parent() == pass.Pkg.Scope() {
				valid = false
				return false
			}
			objects[object] = true
		default:
			valid = false
		}
		return valid
	})
	if !valid {
		return false
	}
	parents := ps6087Parents(body)
	ast.Inspect(body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || !objects[pass.TypesInfo.Uses[identifier]] && !objects[pass.TypesInfo.Defs[identifier]] {
			return true
		}
		for parent := parents[identifier]; parent != nil; parent = parents[parent] {
			switch value := parent.(type) {
			case *ast.AssignStmt:
				for _, lhs := range value.Lhs {
					if ps6099NodeWithin(identifier, lhs) {
						// One local definition initializes an immutable extent. Any
						// later assignment is a mutation and invalidates the proof.
						if pass.TypesInfo.Defs[identifier] != pass.TypesInfo.ObjectOf(identifier) {
							valid = false
						}
					}
				}
				return false
			case *ast.IncDecStmt:
				if ps6099NodeWithin(identifier, value.X) {
					valid = false
				}
				return false
			case *ast.UnaryExpr:
				if value.Op == token.AND {
					valid = false
				}
				return false
			case *ast.CallExpr:
				return false
			}
		}
		return valid
	})
	return valid
}

func ps6106StaticRepetitions(pass *analysis.Pass, node ast.Node, parents map[ast.Node]ast.Node) uint64 {
	repetitions := uint64(1)
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		var count uint64
		var known bool
		switch loop := parent.(type) {
		case *ast.ForStmt:
			count, known = ps6099ForExactIterations(pass, loop)
		case *ast.RangeStmt:
			count, known = ps6099RangeExactIterations(pass, loop.X)
		}
		if !known || count == 0 {
			continue
		}
		if repetitions > math.MaxUint64/count {
			return 1
		}
		repetitions *= count
	}
	return repetitions
}

func ps6106FindInitializer(state *ps6106Analysis, fn *ast.FuncDecl, sequence *ps6106Sequence, contract *config.BoundedScratchFlowContract, reachable map[ast.Node]bool) (ps6106Initializer, bool) {
	if len(sequence.buffer.fields) == 0 {
		return ps6106FindLocalInitializer(state.pass, fn, sequence, contract, reachable)
	}
	field := sequence.buffer.fields[len(sequence.buffer.fields)-1]
	key := ps6106FieldInitKey{field: field, contract: contract, function: fn}
	if result, ok := state.fieldInitProof[key]; ok {
		return result.initializer, result.ok
	}
	initializer, ok := ps6106FindFieldInitializer(state.pass, fn, sequence, contract)
	state.fieldInitProof[key] = ps6106FieldInitResult{initializer: initializer, ok: ok}
	return initializer, ok
}

func ps6106FindLocalInitializer(pass *analysis.Pass, fn *ast.FuncDecl, sequence *ps6106Sequence, contract *config.BoundedScratchFlowContract, reachable map[ast.Node]bool) (ps6106Initializer, bool) {
	var result ps6106Initializer
	count := 0
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if node == nil || node.Pos() >= sequence.producer.Pos() {
			return node != nil
		}
		var lhs ast.Expr
		var rhs ast.Expr
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) == 1 && len(value.Rhs) == 1 {
				lhs, rhs = value.Lhs[0], value.Rhs[0]
			}
		case *ast.ValueSpec:
			if len(value.Names) == 1 && len(value.Values) == 1 {
				lhs, rhs = value.Names[0], value.Values[0]
			}
		}
		if lhs == nil || !ps6106StorageRoot(pass, lhs, sequence.buffer.root) || !reachable[rhs] {
			return true
		}
		initializer, ok := ps6106ClassifyInitializer(pass, lhs, rhs, sequence.buffer, contract)
		if ok {
			result, count = initializer, count+1
		}
		return true
	})
	return result, count == 1
}

func ps6106FindFieldInitializer(pass *analysis.Pass, fn *ast.FuncDecl, sequence *ps6106Sequence, contract *config.BoundedScratchFlowContract) (ps6106Initializer, bool) {
	if len(sequence.buffer.fields) != 1 || !ps6106MethodReceiverRoot(pass, fn, sequence.buffer.root) {
		return ps6106Initializer{}, false
	}
	field := sequence.buffer.fields[0]
	if field.Exported() || field.Pkg() != pass.Pkg {
		return ps6106Initializer{}, false
	}
	owner := ps6087Named(fieldOwnerType(pass, field))
	if owner == nil || owner.Obj().Pkg() != pass.Pkg || owner.TypeParams().Len() != 0 {
		return ps6106Initializer{}, false
	}
	var result ps6106Initializer
	count := 0
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			constructor, ok := declaration.(*ast.FuncDecl)
			if !ok || constructor.Body == nil {
				continue
			}
			parents := ps6087Parents(constructor.Body)
			reachable := ps6099ReachableNodesInBlock(pass, constructor.Body, parents)
			ast.Inspect(constructor.Body, func(node ast.Node) bool {
				assignment, ok := node.(*ast.AssignStmt)
				if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || !reachable[assignment.Rhs[0]] {
					return true
				}
				lhsStorage, ok := ps6106StorageExpression(pass, assignment.Lhs[0])
				if !ok || len(lhsStorage.fields) != 1 || lhsStorage.fields[0] != field ||
					!ps6106ConstructorReturnsRoot(pass, constructor, lhsStorage.root, owner) {
					return true
				}
				initializer, valid := ps6106ClassifyInitializer(pass, assignment.Lhs[0], assignment.Rhs[0], lhsStorage, contract)
				valid = valid && ps6106ConstructorRootClosed(pass, constructor, lhsStorage.root, assignment.Lhs[0], assignment.Rhs[0])
				if valid {
					result, count = initializer, count+1
					result.function = constructor
					result.root = lhsStorage.root
				}
				return true
			})
		}
	}
	return result, count == 1
}

func ps6106ConstructorRootClosed(pass *analysis.Pass, fn *ast.FuncDecl, root types.Object, lhs, rhs ast.Expr) bool {
	allowed := []ast.Expr{lhs, rhs}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		statement, ok := node.(*ast.ReturnStmt)
		if ok && len(statement.Results) == 1 && ps6106StorageRoot(pass, statement.Results[0], root) {
			allowed = append(allowed, statement.Results[0])
		}
		return true
	})
	valid := true
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[identifier] != root && pass.TypesInfo.Defs[identifier] != root {
			return true
		}
		if pass.TypesInfo.Defs[identifier] == root {
			return true
		}
		for _, expression := range allowed {
			if ps6099NodeWithin(identifier, expression) {
				return true
			}
		}
		valid = false
		return false
	})
	return valid
}

func ps6106SequenceRootClosed(state *ps6106Analysis, fn *ast.FuncDecl, sequence *ps6106Sequence) bool {
	allowed := slicesForPS6106(sequence.allowed)
	if sequence.contract != nil {
		allowed = append(allowed, ps6106CallReceivers(state.pass, sequence)...)
	}
	for _, identifier := range state.objectUses[sequence.buffer.root] {
		if identifier.Pos() < fn.Body.Pos() || identifier.End() > fn.Body.End() || state.pass.TypesInfo.Defs[identifier] == sequence.buffer.root {
			continue
		}
		classified := false
		for _, expression := range allowed {
			if ps6099NodeWithin(identifier, expression) {
				classified = true
				break
			}
		}
		if !classified {
			return false
		}
	}
	return true
}

func slicesForPS6106(values []ast.Expr) []ast.Expr {
	return slices.Clone(values)
}

func ps6106WholeOwnerClosed(state *ps6106Analysis, fn *ast.FuncDecl, sequence *ps6106Sequence, initializer *ps6106Initializer) bool {
	pass := state.pass
	field := sequence.buffer.fields[len(sequence.buffer.fields)-1]
	owner := ps6087Named(fieldOwnerType(pass, field))
	if owner == nil || initializer.function == nil {
		return false
	}
	construction := ps6106OwnerConstruction(pass, initializer.function, initializer.root, owner)
	if construction == nil {
		return false
	}
	candidate, _ := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if candidate == nil {
		return false
	}
	allowed := []ast.Expr{initializer.lhs, initializer.value}
	allowed = append(allowed, sequence.allowed...)
	if sequence.contract != nil {
		allowed = append(allowed, ps6106CallReceivers(state.pass, sequence)...)
	}
	// Collect only complete, already-proven owner construction sites. A typed
	// leaf nested inside any other aggregate remains visible to the second pass.
	for _, file := range pass.Files {
		parents := ps6087Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.CallExpr:
				callee, _, ok := typedCallee(pass, value.Fun)
				if !ok || callee.Origin() != candidate.Origin() {
					return true
				}
				switch parents[value].(type) {
				case *ast.GoStmt, *ast.DeferStmt:
					return true
				}
				if receiver := ps6099CallReceiverExpression(pass, value); receiver != nil {
					allowed = append(allowed, receiver)
				}
			case *ast.AssignStmt:
				if ps6106AllowedOwnerConstructionAssignment(pass, value, initializer.root, construction) {
					allowed = append(allowed, value.Lhs[0], value.Rhs[0])
					return true
				}
				if len(value.Lhs) == 1 && len(value.Rhs) == 1 &&
					ps6106InitializerCall(pass, value.Rhs[0], initializer.function) {
					allowed = append(allowed, value.Lhs[0], value.Rhs[0])
					return true
				}
			case *ast.ValueSpec:
				if len(value.Names) == 1 && len(value.Values) == 1 &&
					ps6106InitializerCall(pass, value.Values[0], initializer.function) {
					allowed = append(allowed, value.Names[0], value.Values[0])
				}
			case *ast.ReturnStmt:
				if initializer.function.Body.Pos() <= value.Pos() && value.End() <= initializer.function.Body.End() {
					for _, expression := range value.Results {
						if ps6106StorageRoot(pass, expression, initializer.root) {
							allowed = append(allowed, expression)
						}
					}
				}
			}
			return true
		})
	}
	for _, file := range pass.Files {
		closed := true
		ast.Inspect(file, func(node ast.Node) bool {
			if !closed {
				return false
			}
			expression, ok := node.(ast.Expr)
			if !ok || !ps6106RuntimeOwnerValue(pass, expression, owner) {
				return true
			}
			for _, permitted := range allowed {
				if ps6099NodeWithin(expression, permitted) {
					return true
				}
			}
			closed = false
			return false
		})
		if !closed {
			return false
		}
	}
	return ps6106MethodReceiverRoot(pass, fn, sequence.buffer.root)
}

func ps6106RuntimeOwnerValue(pass *analysis.Pass, expression ast.Expr, owner *types.Named) bool {
	if !ps6106OwnerValue(pass.TypesInfo.TypeOf(expression), owner) {
		return false
	}
	if typeAndValue, ok := pass.TypesInfo.Types[expression]; ok && typeAndValue.IsType() {
		return false
	}
	identifier, ok := expression.(*ast.Ident)
	return !ok || pass.TypesInfo.Defs[identifier] == nil
}

func ps6106InitializerCall(pass *analysis.Pass, expression ast.Expr, initializer *ast.FuncDecl) bool {
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || initializer == nil {
		return false
	}
	callee, _, ok := typedCallee(pass, call.Fun)
	want, _ := pass.TypesInfo.Defs[initializer.Name].(*types.Func)
	return ok && want != nil && callee.Origin() == want.Origin()
}

func ps6106OwnerConstruction(pass *analysis.Pass, fn *ast.FuncDecl, root types.Object, owner *types.Named) *ast.CompositeLit {
	var result *ast.CompositeLit
	count := 0
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 ||
			!ps6106StorageRoot(pass, assignment.Lhs[0], root) {
			return true
		}
		rhs := ps2110Unparen(assignment.Rhs[0])
		if address, ok := rhs.(*ast.UnaryExpr); ok && address.Op == token.AND {
			rhs = ps2110Unparen(address.X)
		}
		literal, ok := rhs.(*ast.CompositeLit)
		if ok && ps6106ExactOwner(pass.TypesInfo.TypeOf(literal), owner) {
			result, count = literal, count+1
		}
		return true
	})
	if count != 1 {
		return nil
	}
	return result
}

func ps6106AllowedOwnerConstructionAssignment(pass *analysis.Pass, assignment *ast.AssignStmt, root types.Object, construction *ast.CompositeLit) bool {
	return len(assignment.Lhs) == 1 && len(assignment.Rhs) == 1 &&
		ps6106StorageRoot(pass, assignment.Lhs[0], root) && ps6099NodeWithin(construction, assignment.Rhs[0])
}

func ps6106ExactOwner(value types.Type, owner *types.Named) bool {
	return value != nil && types.Identical(types.Unalias(value), owner)
}

func ps6106OwnerValue(value types.Type, owner *types.Named) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if types.Identical(value, owner) {
		return true
	}
	pointer, ok := value.(*types.Pointer)
	return ok && types.Identical(types.Unalias(pointer.Elem()), owner)
}

// fieldOwnerType returns the named struct that declares field. Field objects
// do not retain their owner, so find the exact declaration by object identity.
func fieldOwnerType(pass *analysis.Pass, field *types.Var) types.Type {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, specification := range generic.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok {
					continue
				}
				object := pass.TypesInfo.Defs[typeSpec.Name]
				if object == nil {
					continue
				}
				named, _ := object.Type().(*types.Named)
				if named == nil {
					continue
				}
				structure, _ := named.Underlying().(*types.Struct)
				for index := 0; structure != nil && index < structure.NumFields(); index++ {
					if structure.Field(index) == field {
						return named
					}
				}
			}
		}
	}
	return nil
}

func ps6106MethodReceiverRoot(pass *analysis.Pass, fn *ast.FuncDecl, object types.Object) bool {
	if fn.Recv == nil || len(fn.Recv.List) != 1 || len(fn.Recv.List[0].Names) != 1 {
		return false
	}
	return pass.TypesInfo.Defs[fn.Recv.List[0].Names[0]] == object
}

func ps6106ConstructorReturnsRoot(pass *analysis.Pass, fn *ast.FuncDecl, root types.Object, owner *types.Named) bool {
	function, _ := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if function == nil {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Results().Len() != 1 || !types.Identical(signature.Results().At(0).Type(), types.NewPointer(owner)) {
		return false
	}
	returns := 0
	valid := true
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		statement, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		returns++
		if len(statement.Results) != 1 || !ps6106StorageRoot(pass, statement.Results[0], root) {
			valid = false
		}
		return valid
	})
	return valid && returns == 1
}

func ps6106StorageRoot(pass *analysis.Pass, expression ast.Expr, root types.Object) bool {
	storage, ok := ps6106StorageExpression(pass, expression)
	return ok && len(storage.fields) == 0 && storage.root == root
}

func ps6106ClassifyInitializer(pass *analysis.Pass, lhs, rhs ast.Expr, storage ps6106Storage, contract *config.BoundedScratchFlowContract) (ps6106Initializer, bool) {
	if contract == nil {
		call, ok := ps2110Unparen(rhs).(*ast.CallExpr)
		if !ok || !typedBuiltinName(pass, call.Fun, "make") || len(call.Args) < 2 || len(call.Args) > 3 ||
			!types.Identical(pass.TypesInfo.TypeOf(call), storage.typeOf) || !ps6106FloatSlice(storage.typeOf) {
			return ps6106Initializer{}, false
		}
		// A Go range consumer traverses len(slice), not cap(slice).
		return ps6106Initializer{lhs: lhs, value: rhs, fullExtent: call.Args[1]}, true
	}
	call, ok := ps2110Unparen(rhs).(*ast.CallExpr)
	if !ok || ps6087FunctionID(pass, call) != contract.Allocator {
		return ps6106Initializer{}, false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function == nil || signature.Recv() == nil || signature.Results().Len() != 1 ||
		!types.Identical(signature.Results().At(0).Type(), storage.typeOf) || ps6106Interface(storage.typeOf) {
		return ps6106Initializer{}, false
	}
	_, _, _, provider, ok := ps6106ContractCallForAllocator(pass, call, contract.AllocatorCapacityArg)
	if !ok {
		return ps6106Initializer{}, false
	}
	capacity := call.Args[contract.AllocatorCapacityArg-1]
	return ps6106Initializer{lhs: lhs, value: rhs, fullExtent: capacity, provider: provider}, true
}

func ps6106ContractCallForAllocator(pass *analysis.Pass, call *ast.CallExpr, capacityPosition int) (ast.Expr, types.Type, ast.Expr, ps6106Storage, bool) {
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function == nil || signature.Recv() == nil || signature.Variadic() || signature.Results().Len() != 1 ||
		signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 {
		return nil, nil, nil, ps6106Storage{}, false
	}
	selector := ps6099CallSelector(call.Fun)
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.MethodVal {
		return nil, nil, nil, ps6106Storage{}, false
	}
	provider, ok := ps6106StorageExpression(pass, selector.X)
	index := capacityPosition - 1
	if !ok || ps6106Interface(provider.typeOf) || ps6087Named(provider.typeOf) == nil ||
		index < 0 || index >= len(call.Args) || index >= signature.Params().Len() ||
		!ps6106Integer(pass.TypesInfo.TypeOf(call.Args[index])) || !ps6106Integer(signature.Params().At(index).Type()) ||
		!types.AssignableTo(pass.TypesInfo.TypeOf(call.Args[index]), signature.Params().At(index).Type()) {
		return nil, nil, nil, ps6106Storage{}, false
	}
	return selector.X, signature.Results().At(0).Type(), call.Args[index], provider, true
}

func ps6106CompleteUseClosure(state *ps6106Analysis, fn *ast.FuncDecl, sequence *ps6106Sequence, initializer *ps6106Initializer) bool {
	pass := state.pass
	allowed := append([]ast.Expr{initializer.lhs}, sequence.allowed...)
	field := (*types.Var)(nil)
	if len(sequence.buffer.fields) > 0 {
		field = sequence.buffer.fields[len(sequence.buffer.fields)-1]
	}
	object := sequence.buffer.root
	if field != nil {
		object = field
	}
	for _, identifier := range state.objectUses[object] {
		if pass.TypesInfo.Defs[identifier] == object {
			continue
		}
		classified := false
		for _, expression := range allowed {
			if ps6099NodeWithin(identifier, expression) {
				classified = true
				break
			}
		}
		if !classified {
			return false
		}
	}
	if field == nil {
		return ps6106RootUsesClassified(pass, fn.Body, sequence, initializer)
	}
	return true
}

func ps6106RootUsesClassified(pass *analysis.Pass, body *ast.BlockStmt, sequence *ps6106Sequence, initializer *ps6106Initializer) bool {
	allowed := append([]ast.Expr{initializer.lhs}, sequence.allowed...)
	if sequence.contract != nil {
		allowed = append(allowed, ps6106CallReceivers(pass, sequence)...)
	}
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[identifier] != sequence.buffer.root && pass.TypesInfo.Defs[identifier] != sequence.buffer.root {
			return true
		}
		for _, expression := range allowed {
			if ps6099NodeWithin(identifier, expression) {
				return true
			}
		}
		valid = false
		return false
	})
	return valid
}

func ps6106CallReceivers(pass *analysis.Pass, sequence *ps6106Sequence) []ast.Expr {
	calls := []*ast.CallExpr{sequence.producer, sequence.consumer}
	calls = append(calls, sequence.observers...)
	result := make([]ast.Expr, 0, len(calls))
	for _, call := range calls {
		if receiver := ps6099CallReceiverExpression(pass, call); receiver != nil {
			result = append(result, receiver)
		}
	}
	return result
}

func ps6106ContractProviderProof(state *ps6106Analysis, fn *ast.FuncDecl, sequence *ps6106Sequence, initializer *ps6106Initializer, contract *config.BoundedScratchFlowContract) bool {
	pass := state.pass
	if !ps6106SameContractReceiver(contract) || !ps6106ExtentArgumentIdentity(pass, sequence, contract) ||
		!ps6106ProviderUsesClassified(state, fn.Body, sequence, initializer) {
		return false
	}
	if len(sequence.buffer.fields) == 0 {
		return ps6106SameStorage(initializer.provider, sequence.provider) &&
			ps6106ExtentStable(pass, fn.Body, initializer.fullExtent)
	}
	// Field initialization and method execution use different lexical receiver
	// objects. Require the exact same relative provider selector path from the
	// constructor root and the method receiver root.
	return initializer.root != nil && initializer.root == initializer.provider.root &&
		ps6106SameRelativePath(initializer.provider, sequence.provider) &&
		ps6106MethodReceiverRoot(pass, fn, sequence.provider.root) &&
		ps6106ExtentStable(pass, initializer.function.Body, initializer.fullExtent)
}

func ps6106SameContractReceiver(contract *config.BoundedScratchFlowContract) bool {
	owner := ps6106MethodIDOwner(contract.Allocator)
	if owner == "" || ps6106MethodIDOwner(contract.Producer) != owner ||
		ps6106MethodIDOwner(contract.Consumer) != owner {
		return false
	}
	for _, observer := range contract.Observers {
		if ps6106MethodIDOwner(observer.Function) != owner {
			return false
		}
	}
	return contract.Replacement == "" || ps6106MethodIDOwner(contract.Replacement) == owner
}

func ps6106MethodIDOwner(id string) string {
	methodSeparator := strings.LastIndexByte(id, '.')
	if methodSeparator < 0 {
		return ""
	}
	receiverSeparator := strings.LastIndexByte(id[:methodSeparator], '.')
	if receiverSeparator < 0 {
		return ""
	}
	return id[:methodSeparator]
}

func ps6106ExtentArgumentIdentity(pass *analysis.Pass, sequence *ps6106Sequence, contract *config.BoundedScratchFlowContract) bool {
	producerIndex := contract.ProducerActiveArg - 1
	if producerIndex < 0 || producerIndex >= len(sequence.producer.Args) ||
		!ps6106SameExtent(pass, sequence.active, sequence.producer.Args[producerIndex]) {
		return false
	}
	for _, observer := range sequence.observers {
		observerContract := ps6106ObserverContract(pass, observer, contract.Observers)
		if observerContract == nil {
			return false
		}
		index := observerContract.ActiveArg - 1
		if index < 0 || index >= len(observer.Args) || !ps6106SameExtent(pass, sequence.active, observer.Args[index]) {
			return false
		}
	}
	return true
}

func ps6106ProviderUsesClassified(state *ps6106Analysis, body *ast.BlockStmt, sequence *ps6106Sequence, initializer *ps6106Initializer) bool {
	pass := state.pass
	allowed := ps6106CallReceivers(pass, sequence)
	if sequence.provider.root == sequence.buffer.root {
		allowed = append(allowed, sequence.allowed...)
	}
	if initializer.value != nil {
		if call, ok := ps2110Unparen(initializer.value).(*ast.CallExpr); ok {
			if receiver := ps6099CallReceiverExpression(pass, call); receiver != nil {
				allowed = append(allowed, receiver)
			}
		}
	}
	field := (*types.Var)(nil)
	if len(sequence.provider.fields) > 0 {
		field = sequence.provider.fields[len(sequence.provider.fields)-1]
	}
	if field != nil {
		if key := ps6106ProviderConstructorKey(pass, initializer, field); key != nil {
			allowed = append(allowed, key)
		}
	}
	object := sequence.provider.root
	if field != nil {
		object = field
	}
	for _, identifier := range state.objectUses[object] {
		if pass.TypesInfo.Defs[identifier] == object {
			continue
		}
		classified := false
		for _, expression := range allowed {
			if ps6099NodeWithin(identifier, expression) {
				classified = true
				break
			}
		}
		if !classified {
			return false
		}
	}
	return true
}

func ps6106ProviderConstructorKey(pass *analysis.Pass, initializer *ps6106Initializer, field *types.Var) ast.Expr {
	if initializer.function == nil {
		return nil
	}
	var result ast.Expr
	count := 0
	parents := ps6087Parents(initializer.function.Body)
	ast.Inspect(initializer.function.Body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[identifier] != field {
			return true
		}
		keyValue, ok := parents[identifier].(*ast.KeyValueExpr)
		if !ok || keyValue.Key != identifier {
			return true
		}
		count++
		result = identifier
		return true
	})
	if count != 1 {
		return nil
	}
	return result
}

func ps6106ReplacementOnProvider(pass *analysis.Pass, providerType types.Type, id string) bool {
	named := ps6087Named(providerType)
	if named == nil || named.Obj().Pkg() == nil || ps6106MethodIDOwner(id) != named.Obj().Pkg().Path()+"."+named.Obj().Name() {
		return false
	}
	separator := strings.LastIndexByte(id, '.')
	object, _, _ := types.LookupFieldOrMethod(providerType, true, pass.Pkg, id[separator+1:])
	method, ok := object.(*types.Func)
	if !ok || method.Pkg() == nil || method.Pkg().Path() != named.Obj().Pkg().Path() {
		return false
	}
	signature, _ := method.Type().(*types.Signature)
	return signature != nil && ps6087Named(signature.Recv().Type()) == named
}

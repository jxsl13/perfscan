package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6111 implements owner issue #895. It is the dynamic-loop call-site
// companion to PS2140 and the contract-gated counterpart to PS6103's fixed
// loop source proof. Method names alone never establish wrapper/Into parity.
var PS6111 = register(&lint.Check{
	ID:          "PS6111",
	Category:    "alloc",
	Slug:        "allocating-wrapper-in-reusable-result-loop",
	Level:       lint.LevelAggressive,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"reusableResultLoopContracts"},
	Doc: lint.Documentation{
		Title: "a reusable-result loop calls an allocating compatibility wrapper",
		Text: `A generation, decode, or selection loop can repeatedly call an
allocation-returning compatibility wrapper even though the same concrete
receiver exposes a caller-owned-output operation. When one numeric-slice
result is fully traversed before the next call and its identity never escapes,
one destination allocated before the loop can serve every later iteration.

PS6111 does not infer this relationship from Into, To, Append, Step, or TopK
spelling. A reusableResultLoopContracts entry must bind the exact concrete
wrapper and destination methods, identify the result and destination roles and
every shape-dependent wrapper argument, and affirm fresh ownership, stable
shape, complete overwrite before read, synchronous non-retention, and exact
state, error, and panic parity. The analyzer independently checks concrete
typed method identity, direct receiver ownership, non-generic non-variadic
signatures, one scalar numeric slice result, destination placement, argument
mapping, residual error results, and stable receiver/shape roots.

The initial call-site grammar is intentionally small. It accepts a dynamic
for/range loop with one top-level wrapper assignment, either directly into a
loop-carried slice or through one checked temporary immediately transferred to
that slice. Before the call, exactly one unconditional full range traverses the
carried slice, either by a used value binding or by an index used only as the
same slice's element subscript. All uses of both slice objects are classified
across the enclosing function. The loop must have a feasible back edge after
the call; source-known counts and unconditional exits after the call stay
silent. The receiver is one stable concrete local or parameter, configured
shape arguments are immutable local integers or constants across the body and
loop header, and a returned error is checked by an immediate terminal guard.
Constant fixed-count loops remain PS6103's scope.

Fresh-identity observation, nil/address/reflection checks, aliases, stores,
returns, sends, append, calls receiving the slice, callbacks, closures,
goroutines, defer, partial or conditional traversal, post-call reads,
receiver rebinding/address exposure, unstable shape roots, interface or
promoted dispatch, generic/variadic methods, multiple reusable results,
unreachable calls, and incomplete or duplicate contracts stay silent. The
multi-slice TopKN/TopKNInto evidence in issue #895 therefore motivates a future
coupled-result contract; PS6111 does not pretend that two destinations plus
sorting/remap scratch are one slice.

The retained repository work pair allocates the reusable destination once per
measured generation operation, digests the final produced logits, validates
owner-visible decoder state, and pins eight wrapper allocations versus one
destination allocation. It supports allocation and equivalence validation;
no local timing result is claimed.

There is NO automatic fix. Hoist a correctly sized, distinct destination,
preserve the original allocating wrapper as fallback where required, and
benchmark the exact loop. Validate fresh-identity unobservability, destination
length/capacity, overlap rules, all success and failure outputs, state/cache
advancement, callbacks, races, and crossover. Allocation removal alone is not
an unconditional timing win.`,
		Before: `var logits []float32
// initialize logits before the loop
for range maxNew {
	consumeAll(logits)
	next, err := decoder.Step(token, position)
	if err != nil { return err }
	logits = next
}`,
		After: `var logits []float32
// initialize logits before the loop
destination := make([]float32, exactStableResultLength)
for range maxNew {
	consumeAll(logits)
	if err := decoder.StepInto(token, position, destination); err != nil { return err }
	logits = destination // it is fully consumed before the next overwrite
}`,
		MeasuredWin: `Owner issue #895 reports three related GoAI campaigns on
Apple M2 Pro. GPT-2-small at eight generated tokens moved from 2,253,184 B/op
and 12 allocs/op to 614,784 B/op and 4 allocs/op: exactly 1,638,400 fewer bytes
and eight fewer allocations, with seven order-reversed throughput ratios
0.990x-1.007x (median 1.004x). The shared decoder at Vocab=32,000/maxNew=8
likewise removed eight 128,000-byte slices (eight 131,072-byte allocator size
classes); observed bytes moved from 1,442,176 to 393,661 B/op and allocations
from 12 to 4, with seven ratios 0.974x-1.053x (median 1.010x).

The later multi-result TopKNInto primitive moved from 448 B/op and 2 allocs/op
to 0 B/op and 0 allocs/op while retaining 0.981x median primitive throughput.
Its resident first-token integration removed 408,927 B/op and two allocations
and measured a 1.066x median across seven paired campaigns. That coupled
two-slice/remap shape is evidence for the optimization family but lies outside
PS6111's one-result grammar. GoAI pull requests 1214, 1215, and 1216 passed all
reported current-head jobs and retained exact token plus continuation-logit or
device/host Top-K parity. Revalidate the configured backend and workload.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6111",
		Doc:  "allocating compatibility wrapper in a reusable-result loop",
		Run:  runPS6111,
	},
})

type ps6111Candidate struct {
	call          *ast.CallExpr
	assignment    *ast.AssignStmt
	callIndex     int
	transfer      *ast.AssignStmt
	transferIndex int
	carrier       types.Object
	result        types.Object
	consumer      *ast.RangeStmt
	contract      config.ReusableResultLoopContract
	resultType    types.Type
	elementBytes  int64
}

func runPS6111(pass *analysis.Pass) (any, error) {
	return runPS6111WithContracts(pass, config.Current().ReusableResultLoopContracts)
}

func runPS6111WithContracts(pass *analysis.Pass, configured []config.ReusableResultLoopContract) (any, error) {
	contracts := ps6111Contracts(configured)
	if len(contracts) == 0 {
		return nil, nil
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ps6111Function(pass, function, contracts)
		}
	}
	return nil, nil
}

// ps6111Contracts rejects ambiguous configuration as a set, independent of
// input order. One wrapper cannot carry two different semantic promises.
func ps6111Contracts(configured []config.ReusableResultLoopContract) map[string]config.ReusableResultLoopContract {
	nameCount := make(map[string]int)
	wrapperCount := make(map[string]int)
	for _, contract := range configured {
		if contract.Valid() {
			nameCount[contract.Name]++
			wrapperCount[contract.Wrapper]++
		}
	}
	result := make(map[string]config.ReusableResultLoopContract)
	for _, contract := range configured {
		if contract.Valid() && nameCount[contract.Name] == 1 && wrapperCount[contract.Wrapper] == 1 {
			result[contract.Wrapper] = contract
		}
	}
	return result
}

func ps6111Function(pass *analysis.Pass, function *ast.FuncDecl, contracts map[string]config.ReusableResultLoopContract) {
	parents := ps6087Parents(function.Body)
	exposed := ps6103ExposureFacts(pass, function.Body, parents)
	unreachable := ps2144Unreachable(pass, function.Body)
	reported := make(map[*ast.CallExpr]bool)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		body := ps6111LoopBody(node)
		if body == nil || !ps6111LoopMayRepeat(pass, node) || ps6103FixedLoop(pass, node) != nil ||
			ps6111AsyncOrDeferred(body) {
			return true
		}
		for index, statement := range body.List {
			assignment, ok := statement.(*ast.AssignStmt)
			if !ok || len(assignment.Rhs) != 1 {
				continue
			}
			call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
			if !ok || call.Ellipsis.IsValid() || reported[call] || ps2144PositionIn(call.Pos(), unreachable) {
				continue
			}
			contract, ok := contracts[ps6087FunctionID(pass, call)]
			if !ok {
				continue
			}
			candidate, ok := ps6111At(pass, function, node, body, index, assignment, call, contract, parents, exposed)
			if !ok {
				continue
			}
			reported[call] = true
			pass.Report(analysis.Diagnostic{Pos: call.Pos(), End: call.End(), Message: ps6111Message(pass, &candidate)})
		}
		return true
	})
}

func ps6111LoopBody(node ast.Node) *ast.BlockStmt {
	switch loop := node.(type) {
	case *ast.ForStmt:
		return loop.Body
	case *ast.RangeStmt:
		return loop.Body
	default:
		return nil
	}
}

func ps6111LoopMayRepeat(pass *analysis.Pass, node ast.Node) bool {
	switch loop := node.(type) {
	case *ast.RangeStmt:
		// A source-known range count belongs to fixed-count analysis, not this
		// dynamic-loop rule. Composite literals need a separate check because a
		// slice literal's type does not encode its length.
		if ps6111RangeCountKnown(pass, loop.X) {
			return false
		}
		return true
	case *ast.ForStmt:
		if loop.Cond == nil {
			return true
		}
		if value := pass.TypesInfo.Types[loop.Cond].Value; value != nil {
			return value.Kind() == constant.Bool && constant.BoolVal(value)
		}
		// If the canonical initializer, comparison, and post statement prove
		// the first two condition evaluations, the loop is statically counted:
		// zero/one trips cannot amortize a destination, while longer fixed loops
		// remain PS6103's scope.
		if ps6111StaticForCountKnown(pass, loop) {
			return false
		}
		return true
	default:
		return false
	}
}

func ps6111RangeCountKnown(pass *analysis.Pass, expression ast.Expr) bool {
	if pass.TypesInfo.Types[expression].Value != nil {
		return true
	}
	typ := pass.TypesInfo.TypeOf(expression)
	if typ != nil {
		if _, ok := types.Unalias(typ).Underlying().(*types.Array); ok {
			return true
		}
	}
	literal, ok := ps2110Unparen(expression).(*ast.CompositeLit)
	if !ok {
		return false
	}
	literalType := pass.TypesInfo.TypeOf(literal)
	if literalType == nil {
		return false
	}
	_, slice := types.Unalias(literalType).Underlying().(*types.Slice)
	return slice
}

// ps6111StaticForCountKnown recognizes only a local induction variable with a
// constant initializer and constant comparison bound. The post/body need not
// be interpreted: suppressing all such fixed-header loops is conservative and
// leaves dynamic-bound loops as the intentionally supported PS6111 shape.
func ps6111StaticForCountKnown(pass *analysis.Pass, loop *ast.ForStmt) bool {
	initializer, ok := loop.Init.(*ast.AssignStmt)
	if !ok || len(initializer.Lhs) != 1 || len(initializer.Rhs) != 1 {
		return false
	}
	identifier, ok := ps2110Unparen(initializer.Lhs[0]).(*ast.Ident)
	if !ok {
		return false
	}
	object := ps6103AssignedObject(pass, identifier, initializer.Tok)
	start := pass.TypesInfo.Types[initializer.Rhs[0]].Value
	condition, conditionOK := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
	if object == nil || start == nil || start.Kind() != constant.Int || !conditionOK ||
		!ps6103Object(pass, condition.X, object) || !ps6111Comparison(condition.Op) {
		return false
	}
	bound := pass.TypesInfo.Types[condition.Y].Value
	return bound != nil && bound.Kind() == constant.Int
}

func ps6111Comparison(operation token.Token) bool {
	switch operation {
	case token.LSS, token.LEQ, token.GTR, token.GEQ, token.EQL, token.NEQ:
		return true
	default:
		return false
	}
}

func ps6111AsyncOrDeferred(body *ast.BlockStmt) bool {
	unsafe := false
	ast.Inspect(body, func(node ast.Node) bool {
		if unsafe {
			return false
		}
		switch node.(type) {
		case *ast.FuncLit, *ast.GoStmt, *ast.DeferStmt:
			unsafe = true
			return false
		}
		return true
	})
	return unsafe
}

func ps6111At(pass *analysis.Pass, function *ast.FuncDecl, loop ast.Node, body *ast.BlockStmt, index int, assignment *ast.AssignStmt, call *ast.CallExpr, contract config.ReusableResultLoopContract, parents map[ast.Node]ast.Node, exposed map[types.Object]bool) (ps6111Candidate, bool) {
	resultIndex, resultType, elementBytes, errorIndex, ok := ps6111TypedPair(pass, call, contract)
	if !ok || len(assignment.Lhs) != ps6111CallResultCount(pass, call) || resultIndex >= len(assignment.Lhs) {
		return ps6111Candidate{}, false
	}
	receiver, receiverObject, ok := ps6111StableReceiver(pass, function.Body, loop, call, parents, exposed)
	if !ok || receiver == nil || receiverObject == nil {
		return ps6111Candidate{}, false
	}
	for _, position := range contract.ShapeArgumentPositions {
		if position > len(call.Args) || !ps6111StableShapeExpression(pass, call.Args[position-1], loop, body, parents, exposed) {
			return ps6111Candidate{}, false
		}
	}
	resultName, ok := ps2110Unparen(assignment.Lhs[resultIndex]).(*ast.Ident)
	if !ok || resultName.Name == "_" {
		return ps6111Candidate{}, false
	}
	resultObject := ps6111LHSObject(pass, resultName)
	if resultObject == nil || resultObject == receiverObject || ps6111ExpressionHasObject(pass, call, resultObject) {
		return ps6111Candidate{}, false
	}
	next := index + 1
	if errorIndex >= 0 {
		if errorIndex >= len(assignment.Lhs) || !ps6111ImmediateErrorReturn(pass, body, next, assignment.Lhs[errorIndex]) {
			return ps6111Candidate{}, false
		}
		next++
	}
	candidate := ps6111Candidate{call: call, assignment: assignment, callIndex: index, transferIndex: index, carrier: resultObject, result: resultObject, contract: contract, resultType: resultType, elementBytes: elementBytes}
	if resultObject.Pos() >= loop.Pos() && resultObject.Pos() <= loop.End() {
		if next >= len(body.List) {
			return ps6111Candidate{}, false
		}
		transfer, carrier, ok := ps6111Transfer(pass, body.List[next], resultObject, loop)
		if !ok || !ps6111TemporaryClosed(pass, function.Body, resultObject, resultName, transfer, parents) {
			return ps6111Candidate{}, false
		}
		candidate.transfer, candidate.transferIndex, candidate.carrier = transfer, next, carrier
	}
	consumer, ok := ps6111CarrierClosed(pass, function.Body, loop, body, &candidate, parents)
	if !ok || !ps6111HasFeasibleBackEdge(pass, body, candidate.transferIndex) {
		return ps6111Candidate{}, false
	}
	candidate.consumer = consumer
	return candidate, true
}

func ps6111TypedPair(pass *analysis.Pass, call *ast.CallExpr, contract config.ReusableResultLoopContract) (int, types.Type, int64, int, bool) {
	callee, wrapper, ok := typedCallee(pass, call.Fun)
	if !ok || callee.Pkg() == nil || wrapper == nil || wrapper.Recv() == nil || wrapper.Variadic() ||
		wrapper.TypeParams().Len() != 0 || wrapper.RecvTypeParams().Len() != 0 || ps6090FunctionID(callee) != contract.Wrapper {
		return 0, nil, 0, 0, false
	}
	if named := ps6087Named(wrapper.Recv().Type()); named == nil {
		return 0, nil, 0, 0, false
	} else if _, dynamic := named.Underlying().(*types.Interface); dynamic {
		return 0, nil, 0, 0, false
	}
	resultIndex := contract.ResultPosition - 1
	if resultIndex < 0 || resultIndex >= wrapper.Results().Len() || wrapper.Results().Len() < 1 || wrapper.Results().Len() > 2 {
		return 0, nil, 0, 0, false
	}
	resultType := wrapper.Results().At(resultIndex).Type()
	slice, ok := types.Unalias(resultType).Underlying().(*types.Slice)
	if !ok || !ps6107Scalar(slice.Elem()) {
		return 0, nil, 0, 0, false
	}
	errorIndex := -1
	for position := 0; position < wrapper.Results().Len(); position++ {
		if position == resultIndex {
			continue
		}
		if errorIndex >= 0 || !ps6103ErrorType(wrapper.Results().At(position).Type()) {
			return 0, nil, 0, 0, false
		}
		errorIndex = position
	}
	selector := ps6103CallSelector(call.Fun)
	selection := pass.TypesInfo.Selections[selector]
	if selector == nil || selection == nil || selection.Kind() != types.MethodVal || len(selection.Index()) != 1 {
		return 0, nil, 0, 0, false
	}
	intoName := contract.Into[strings.LastIndexByte(contract.Into, '.')+1:]
	method := types.NewMethodSet(pass.TypesInfo.TypeOf(selector.X)).Lookup(callee.Pkg(), intoName)
	if method == nil {
		return 0, nil, 0, 0, false
	}
	intoObject, ok := method.Obj().(*types.Func)
	into, okSignature := method.Type().(*types.Signature)
	if !ok || !okSignature || ps6090FunctionID(intoObject) != contract.Into || into.Recv() == nil || into.Variadic() ||
		into.TypeParams().Len() != 0 || into.RecvTypeParams().Len() != 0 ||
		ps6103ReceiverObject(wrapper.Recv().Type()) != ps6103ReceiverObject(intoObject.Type().(*types.Signature).Recv().Type()) {
		return 0, nil, 0, 0, false
	}
	destination := contract.DestinationArgument - 1
	if destination < 0 || into.Params().Len() != wrapper.Params().Len()+1 || destination >= into.Params().Len() ||
		!types.Identical(resultType, into.Params().At(destination).Type()) || !ps6103ResidualResults(wrapper.Results(), resultIndex, into.Results()) {
		return 0, nil, 0, 0, false
	}
	wrapperArgument := 0
	for position := 0; position < into.Params().Len(); position++ {
		if position == destination {
			continue
		}
		if wrapperArgument >= wrapper.Params().Len() || !types.Identical(wrapper.Params().At(wrapperArgument).Type(), into.Params().At(position).Type()) {
			return 0, nil, 0, 0, false
		}
		wrapperArgument++
	}
	for _, position := range contract.ShapeArgumentPositions {
		if position > wrapper.Params().Len() {
			return 0, nil, 0, 0, false
		}
		if !ps6111StableShapeType(wrapper.Params().At(position - 1).Type()) {
			return 0, nil, 0, 0, false
		}
	}
	elementBytes := pass.TypesSizes.Sizeof(slice.Elem())
	return resultIndex, resultType, elementBytes, errorIndex, elementBytes > 0
}

func ps6111StableShapeType(value types.Type) bool {
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsInteger != 0
}

func ps6111StableShapeExpression(pass *analysis.Pass, expression ast.Expr, loop ast.Node, body *ast.BlockStmt, parents map[ast.Node]ast.Node, exposed map[types.Object]bool) bool {
	expression = ps2110Unparen(expression)
	if value := pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Int {
		return true
	}
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return false
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	variable, ok := object.(*types.Var)
	if !ok || variable.IsField() || object.Parent() == pass.Pkg.Scope() || object.Pos() >= loop.Pos() ||
		!ps6103StableExpression(pass, expression, &ps6103Loop{node: loop, body: body}, parents, exposed) {
		return false
	}
	switch statement := loop.(type) {
	case *ast.ForStmt:
		return statement.Post == nil || !ps6103ObjectMutable(pass, statement.Post, object, parents, exposed)
	case *ast.RangeStmt:
		return !ps6103Object(pass, statement.Key, object) && !ps6103Object(pass, statement.Value, object)
	default:
		return false
	}
}

func ps6111HasFeasibleBackEdge(pass *analysis.Pass, body *ast.BlockStmt, after int) bool {
	if body == nil || after < 0 || after >= len(body.List) {
		return false
	}
	for _, statement := range body.List[after+1:] {
		if ps6111DefinitelyTerminatesIteration(pass, statement) {
			return false
		}
	}
	return true
}

func ps6111DefinitelyTerminatesIteration(pass *analysis.Pass, statement ast.Stmt) bool {
	switch value := statement.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BranchStmt:
		return value.Tok == token.BREAK || value.Tok == token.GOTO
	case *ast.BlockStmt:
		return len(value.List) > 0 && ps6111DefinitelyTerminatesIteration(pass, value.List[len(value.List)-1])
	case *ast.IfStmt:
		if condition := pass.TypesInfo.Types[value.Cond].Value; condition != nil && condition.Kind() == constant.Bool {
			if constant.BoolVal(condition) {
				return len(value.Body.List) > 0 && ps6111DefinitelyTerminatesIteration(pass, value.Body.List[len(value.Body.List)-1])
			}
			return value.Else != nil && ps6111DefinitelyTerminatesElse(pass, value.Else)
		}
		return len(value.Body.List) > 0 && ps6111DefinitelyTerminatesIteration(pass, value.Body.List[len(value.Body.List)-1]) &&
			value.Else != nil && ps6111DefinitelyTerminatesElse(pass, value.Else)
	default:
		return false
	}
}

func ps6111DefinitelyTerminatesElse(pass *analysis.Pass, statement ast.Stmt) bool {
	if alternate, ok := statement.(*ast.IfStmt); ok {
		return ps6111DefinitelyTerminatesIteration(pass, alternate)
	}
	block, ok := statement.(*ast.BlockStmt)
	return ok && len(block.List) > 0 && ps6111DefinitelyTerminatesIteration(pass, block.List[len(block.List)-1])
}

func ps6111CallResultCount(pass *analysis.Pass, call *ast.CallExpr) int {
	if tuple, ok := pass.TypesInfo.TypeOf(call).(*types.Tuple); ok {
		return tuple.Len()
	}
	return 1
}

func ps6111StableReceiver(pass *analysis.Pass, function *ast.BlockStmt, loop ast.Node, call *ast.CallExpr, parents map[ast.Node]ast.Node, exposed map[types.Object]bool) (ast.Expr, types.Object, bool) {
	selector := ps6103CallSelector(call.Fun)
	if selector == nil {
		return nil, nil, false
	}
	receiver, ok := ps2110Unparen(selector.X).(*ast.Ident)
	if !ok {
		return nil, nil, false
	}
	object := pass.TypesInfo.ObjectOf(receiver)
	if object == nil || object.Pos() >= loop.Pos() && object.Pos() <= loop.End() || exposed[object] ||
		ps6111ReceiverRebound(pass, function, object, parents) {
		return nil, nil, false
	}
	return selector.X, object, true
}

func ps6111ReceiverRebound(pass *analysis.Pass, function *ast.BlockStmt, object types.Object, parents map[ast.Node]ast.Node) bool {
	rebound := false
	ast.Inspect(function, func(node ast.Node) bool {
		if rebound {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(identifier) != object || pass.TypesInfo.Defs[identifier] == object {
			return true
		}
		if ps6103ObjectWrite(identifier, parents) {
			rebound = true
			return false
		}
		return true
	})
	return rebound
}

func ps6111LHSObject(pass *analysis.Pass, identifier *ast.Ident) types.Object {
	if identifier == nil {
		return nil
	}
	if object := pass.TypesInfo.Defs[identifier]; object != nil {
		return object
	}
	return pass.TypesInfo.Uses[identifier]
}

func ps6111ImmediateErrorReturn(pass *analysis.Pass, body *ast.BlockStmt, index int, expression ast.Expr) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok || identifier.Name == "_" || index >= len(body.List) {
		return false
	}
	object := ps6111LHSObject(pass, identifier)
	guard, ok := body.List[index].(*ast.IfStmt)
	condition, conditionOK := ps2110Unparen(guardCondition(guard)).(*ast.BinaryExpr)
	if !ok || object == nil || guard.Init != nil || guard.Else != nil || !conditionOK || condition.Op != token.NEQ ||
		!ps6103Object(pass, condition.X, object) || !ps6109Nil(pass, condition.Y) || len(guard.Body.List) != 1 {
		return false
	}
	returned, ok := guard.Body.List[0].(*ast.ReturnStmt)
	if !ok {
		return false
	}
	for _, result := range returned.Results {
		if ps6103Object(pass, result, object) {
			return true
		}
	}
	return false
}

func guardCondition(guard *ast.IfStmt) ast.Expr {
	if guard == nil {
		return nil
	}
	return guard.Cond
}

func ps6111Transfer(pass *analysis.Pass, statement ast.Stmt, result types.Object, loop ast.Node) (*ast.AssignStmt, types.Object, bool) {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 ||
		!ps6103Object(pass, assignment.Rhs[0], result) {
		return nil, nil, false
	}
	identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	if !ok || identifier.Name == "_" {
		return nil, nil, false
	}
	carrier := pass.TypesInfo.Uses[identifier]
	if carrier == nil || carrier.Pos() >= loop.Pos() && carrier.Pos() <= loop.End() {
		return nil, nil, false
	}
	return assignment, carrier, true
}

func ps6111TemporaryClosed(pass *analysis.Pass, function *ast.BlockStmt, object types.Object, definition *ast.Ident, transfer *ast.AssignStmt, parents map[ast.Node]ast.Node) bool {
	uses := 0
	valid := true
	ast.Inspect(function, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(identifier) != object {
			return true
		}
		if identifier == definition || pass.TypesInfo.Defs[identifier] == object {
			return true
		}
		if assignment, ok := parents[identifier].(*ast.AssignStmt); ok && assignment == transfer && len(assignment.Rhs) == 1 && ps2110Unparen(assignment.Rhs[0]) == identifier {
			uses++
			return true
		}
		valid = false
		return false
	})
	return valid && uses == 1
}

func ps6111CarrierClosed(pass *analysis.Pass, function *ast.BlockStmt, loop ast.Node, body *ast.BlockStmt, candidate *ps6111Candidate, parents map[ast.Node]ast.Node) (*ast.RangeStmt, bool) {
	consumer := ps6111PriorConsumer(pass, body, candidate.call.Pos(), candidate.carrier, parents)
	if consumer == nil {
		return nil, false
	}
	valid := true
	ast.Inspect(function, func(node ast.Node) bool {
		if !valid {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(identifier) != candidate.carrier {
			return true
		}
		if pass.TypesInfo.Defs[identifier] == candidate.carrier {
			return true
		}
		if candidate.result == candidate.carrier && ps6111InLHS(identifier, candidate.assignment) {
			return true
		}
		if candidate.transfer != nil && ps6111InLHS(identifier, candidate.transfer) {
			return true
		}
		if identifier.Pos() < loop.Pos() && ps6111AnyAssignmentLHS(identifier, parents) {
			return true
		}
		if ps6111RangeUse(identifier, parents) == consumer || ps6111IndexRangeCarrierUse(pass, identifier, consumer, parents) {
			return true
		}
		valid = false
		return false
	})
	return consumer, valid && consumer != nil
}

func ps6111PriorConsumer(pass *analysis.Pass, body *ast.BlockStmt, before token.Pos, carrier types.Object, parents map[ast.Node]ast.Node) *ast.RangeStmt {
	var consumer *ast.RangeStmt
	for _, statement := range body.List {
		if statement.Pos() >= before {
			break
		}
		ranged, ok := statement.(*ast.RangeStmt)
		if !ok || !ps6103Object(pass, ranged.X, carrier) {
			continue
		}
		if consumer != nil || !ps6111FullRangeConsumer(pass, ranged, parents) {
			return nil
		}
		consumer = ranged
	}
	return consumer
}

func ps6111InLHS(identifier *ast.Ident, assignment *ast.AssignStmt) bool {
	if assignment == nil {
		return false
	}
	for _, left := range assignment.Lhs {
		if ps2110Unparen(left) == identifier {
			return true
		}
	}
	return false
}

func ps6111AnyAssignmentLHS(identifier *ast.Ident, parents map[ast.Node]ast.Node) bool {
	node := ast.Node(identifier)
	for {
		parent := parents[node]
		if paren, ok := parent.(*ast.ParenExpr); ok {
			node = paren
			continue
		}
		assignment, ok := parent.(*ast.AssignStmt)
		if !ok {
			return false
		}
		for _, left := range assignment.Lhs {
			if ps2110Unparen(left) == node {
				return true
			}
		}
		return false
	}
}

func ps6111RangeUse(identifier *ast.Ident, parents map[ast.Node]ast.Node) *ast.RangeStmt {
	node := ast.Node(identifier)
	for {
		parent := parents[node]
		if paren, ok := parent.(*ast.ParenExpr); ok {
			node = paren
			continue
		}
		ranged, ok := parent.(*ast.RangeStmt)
		if ok && ps2110Unparen(ranged.X) == node {
			return ranged
		}
		return nil
	}
}

func ps6111FullRangeConsumer(pass *analysis.Pass, ranged *ast.RangeStmt, parents map[ast.Node]ast.Node) bool {
	if ranged == nil || ranged.Body == nil {
		return false
	}
	if value, ok := ps2110Unparen(ranged.Value).(*ast.Ident); ok && value.Name != "_" {
		return ps6111ValueRangeConsumer(pass, ranged, value)
	}
	return ps6111IndexRangeConsumer(pass, ranged, parents)
}

func ps6111ValueRangeConsumer(pass *analysis.Pass, ranged *ast.RangeStmt, value *ast.Ident) bool {
	valueObject := ps6103AssignedObject(pass, value, ranged.Tok)
	if valueObject == nil {
		return false
	}
	valueUses := 0
	safe := true
	ast.Inspect(ranged.Body, func(node ast.Node) bool {
		if !safe {
			return false
		}
		switch node.(type) {
		case *ast.BranchStmt, *ast.ReturnStmt, *ast.FuncLit, *ast.GoStmt, *ast.DeferStmt:
			safe = false
			return false
		}
		if identifier, ok := node.(*ast.Ident); ok && pass.TypesInfo.Uses[identifier] == valueObject {
			valueUses++
		}
		return true
	})
	return safe && valueUses > 0
}

func ps6111IndexRangeConsumer(pass *analysis.Pass, ranged *ast.RangeStmt, parents map[ast.Node]ast.Node) bool {
	if ranged.Value != nil || ranged.Tok != token.DEFINE {
		return false
	}
	index, ok := ps2110Unparen(ranged.Key).(*ast.Ident)
	if !ok || index.Name == "_" {
		return false
	}
	indexObject := pass.TypesInfo.Defs[index]
	carrierObject := ps6111ExpressionObject(pass, ranged.X)
	if indexObject == nil || carrierObject == nil {
		return false
	}
	uses := 0
	safe := true
	ast.Inspect(ranged.Body, func(node ast.Node) bool {
		if !safe {
			return false
		}
		switch node.(type) {
		case *ast.BranchStmt, *ast.ReturnStmt, *ast.FuncLit, *ast.GoStmt, *ast.DeferStmt:
			safe = false
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := pass.TypesInfo.Uses[identifier]
		if object != indexObject && object != carrierObject {
			return true
		}
		indexed := ps6111ParentIndex(identifier, parents)
		if indexed == nil || !ps6103Object(pass, indexed.X, carrierObject) || !ps6103Object(pass, indexed.Index, indexObject) ||
			ps6111IndexAddressed(indexed, parents) {
			safe = false
			return false
		}
		uses++
		return true
	})
	return safe && uses > 0
}

func ps6111ExpressionObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	identifier, _ := ps2110Unparen(expression).(*ast.Ident)
	if identifier == nil {
		return nil
	}
	return pass.TypesInfo.ObjectOf(identifier)
}

func ps6111ParentIndex(identifier *ast.Ident, parents map[ast.Node]ast.Node) *ast.IndexExpr {
	var node ast.Node = identifier
	for {
		parent := parents[node]
		if parenthesis, ok := parent.(*ast.ParenExpr); ok {
			node = parenthesis
			continue
		}
		indexed, _ := parent.(*ast.IndexExpr)
		return indexed
	}
}

func ps6111IndexAddressed(indexed *ast.IndexExpr, parents map[ast.Node]ast.Node) bool {
	var node ast.Node = indexed
	for {
		parent := parents[node]
		if parenthesis, ok := parent.(*ast.ParenExpr); ok {
			node = parenthesis
			continue
		}
		unary, ok := parent.(*ast.UnaryExpr)
		return ok && unary.Op == token.AND && unary.X == node
	}
}

func ps6111IndexRangeCarrierUse(pass *analysis.Pass, identifier *ast.Ident, ranged *ast.RangeStmt, parents map[ast.Node]ast.Node) bool {
	if ranged == nil || ranged.Value != nil {
		return false
	}
	index, ok := ps2110Unparen(ranged.Key).(*ast.Ident)
	if !ok {
		return false
	}
	indexed := ps6111ParentIndex(identifier, parents)
	return indexed != nil && ps6103Object(pass, indexed.Index, pass.TypesInfo.Defs[index]) && !ps6111IndexAddressed(indexed, parents)
}

func ps6111ExpressionHasObject(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.ObjectOf(identifier) == object {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps6111Message(pass *analysis.Pass, candidate *ps6111Candidate) string {
	var message strings.Builder
	message.Grow(768)
	message.WriteString(exprTextRendered(candidate.call.Fun))
	message.WriteString(" returns a fresh ")
	message.WriteString(types.TypeString(candidate.resultType, func(pkg *types.Package) string { return pkg.Name() }))
	message.WriteString(" inside a reusable-result loop after the prior result is fully traversed; exact configured ")
	message.WriteString(candidate.contract.Into[strings.LastIndexByte(candidate.contract.Into, '.')+1:])
	message.WriteString(" can write one caller-owned destination because this contract affirms stable shape, complete overwrite, identity/capacity independence, synchronous non-retention, and state/error/panic parity")
	if elements, iterations := candidate.contract.ConfiguredResultElements, candidate.contract.ConfiguredLoopIterations; elements > 0 && iterations > 0 && candidate.elementBytes > 0 && elements <= math.MaxInt64/candidate.elementBytes && elements*candidate.elementBytes <= math.MaxInt64/iterations {
		payload := elements * candidate.elementBytes * iterations
		message.WriteString("; configured workload model ")
		message.WriteString(strconv.FormatInt(iterations, 10))
		message.WriteString(" calls × ")
		message.WriteString(strconv.FormatInt(elements, 10))
		message.WriteString(" elements estimates ")
		message.WriteString(strconv.FormatInt(payload, 10))
		message.WriteString(" payload bytes avoided (configuration, not source proof)")
	}
	message.WriteString("; hoist an exact distinct destination, preserve fallback, and validate identity, aliasing, lengths/capacity, all error paths, state/cache advancement, races, exact outputs, and benchmark crossover (advisory, no automatic fix)")
	return message.String()
}

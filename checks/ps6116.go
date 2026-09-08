package checks

import (
	"go/ast"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6116 implements owner issue #876 as a configured, source-proved advisory.
var PS6116 = register(&lint.Check{
	ID:          "PS6116",
	Category:    "verify",
	Slug:        "forward-loss-backward-graph-objective",
	Level:       lint.LevelAggressive,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"forwardLossBackwardGraphContracts"},
	Doc: lint.Documentation{
		Title: "an eager forward-loss-backward objective may merit one cached graph submission",
		Text: `A model objective can cross an accelerator boundary for forward,
scalar loss, and reverse mode separately. Some graph backends then export
intermediates, synchronize, or recreate forward work while constructing the
backward graph. Per-operator checks cannot establish whether the complete
objective is a useful ownership boundary.

PS6116 is deliberately config-gated and fail-closed. A
forwardLossBackwardGraphContracts entry binds one exact non-generic objective,
private-recorder factory and binding, concrete forward, scalar-loss, backward,
parameter-order, and gradient callables. The source must contain exactly one
straight-line chain: a private recorder is created and bound; the forward
result flows directly and exclusively into the loss; that loss flows into
backward and the final result; and one canonical full range over the exact
parameter-order result stores gradient(parameter) at the same index. The
recorder factory's direct backend field and recorder-binding receiver must
come from the same context object. Only terminal error guards may separate
forward, loss, and backward.

Calls and optional reduction constants are checked by typed identity. Methods
must be direct concrete method values. Interfaces, promoted methods, method
expressions, function values, wrappers, closures, generics, variadic ellipsis
expansion, go/defer, source-proven unreachable calls, duplicate or partial
chains, aliases, extra
forward/loss/recorder uses, wrapped forward inputs, backend/context capability
changes, non-canonical gradient collection, gradient-loop side effects, and
ambiguous contracts stay silent. Concrete variadic APIs are eligible only
through direct fixed-argument calls with no variadic options. The contract
must separately cite owner-reviewed evidence and affirm stable geometry and
scalar reduction, complete gradient
order, private-recorder isolation, absent custom hooks and mutation, exact
dtype/layout/backend eligibility, forward, loss, all-gradient, immutability,
error and panic parity, absence of per-operation/layer routes on the candidate,
preservation of configured routes by the private tape, backend-selection parity,
and preservation of the portable fallback.

The candidate described by the contract is one graph submission with a
positive bounded cache keyed by complete geometry and dtype/layout/objective
semantics. A configured current path must have at least two submissions and
must recreate forward work. Existing reviewed whole-objective capabilities and
intentionally retained multi-submission routes are silent.

There is NO automatic fix. The diagnostic requests a narrow optional backend
capability while keeping the portable private-tape path authoritative. Require
paired numerical proof for scalar loss and every ordered gradient, input and
parameter immutability, fallback isolation, and paired end-to-end campaigns.
The finding does not claim graph fusion, one submission, or an application
speedup will win on another model, shape, dtype, backend, or machine.`,
		Before: `tape := newTape(backend)
recording := ctx.WithRecorder(tape)
logits, err := model.Forward(recording, inputs)
loss, err := CrossEntropy(recording, logits, targets)
if err := tape.Backward(loss); err != nil { return nil, nil, err }
params := model.Params()
grads := make([]*Tensor, len(params))
for i, parameter := range params { grads[i] = tape.Grad(parameter) }`,
		After: `// Candidate only: add a narrowly selected whole-objective backend
// capability backed by a complete-key bounded cache and one submission.
// Keep the exact portable private-tape sequence as the fallback and retain the
// candidate only after paired numerical and end-to-end validation.`,
		MeasuredWin: `GoAI PR #1199 at exact head
d99bc4b5f02a80e03db7c29684efd61207143e41 passed all 15 CI checks and was
squash-merged as 686f27f0a16d8f56526849ac87bda6d97fea029e. On Apple M2 Pro with
F32 B=8, sequence 65, width 128, four heads, FFN 512, depth 4, and ten
classes, its cached complete MPSGraph objective used one command-buffer
submission. Three alternating-order paired campaigns had medians 1.454x,
1.450x, and 1.499x; the overall 21-sample median was 1.456x and the minimum
was 1.321x. Correctness covered scalar loss, all 56 parameter gradients,
input and parameter immutability, recorder isolation, and the portable
fallback. This exact owner evidence motivates a screen; it is not a generic
fusion or application-win claim. Two unresolved reviews on that exact head
(comments 3840658100 and 3840658103) found that the candidate could bypass
per-operation/layer routes and the portable private tape could discard those
routes during backward. Those findings are explicit contract and validation
gates here, not accepted behavior.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6116",
		Doc:  "configured eager forward-loss-backward objectives that merit a one-submission graph screen",
		Run:  runPS6116,
	},
})

type ps6116Chain struct {
	forward, loss, backward, gradient *ast.CallExpr
}

func runPS6116(pass *analysis.Pass) (any, error) {
	return runPS6116WithContracts(pass, config.Current().ForwardLossBackwardGraphContracts)
}

func runPS6116WithContracts(pass *analysis.Pass, configured []config.ForwardLossBackwardGraphContract) (any, error) {
	declarations := ps6115Declarations(pass)
	for _, contract := range ps6116Contracts(configured) {
		if contract.ExistingWholeObjectiveCapability || contract.IntentionalMultiSubmissionRoute {
			continue
		}
		declaration := declarations[contract.ObjectiveSite]
		if declaration == nil || ps6113GenericFunction(pass, declaration) || !ps6116CallablesResolve(pass, contract) {
			continue
		}
		chain, ok := ps6116Match(pass, declaration, contract)
		if !ok {
			continue
		}
		pass.Report(analysis.Diagnostic{
			Pos: chain.forward.Pos(), End: chain.forward.End(),
			Message: contract.Name + ": configured " + contract.ConfiguredGeometry + " eager objective crosses " +
				strconv.Itoa(contract.ConfiguredCurrentSubmissionCount) + " submissions and recreates forward work; screen only a narrow optional whole-objective capability with one submission and a complete-key cache bounded to " +
				strconv.Itoa(contract.MaxCacheEntries) + " entries while preserving the portable private-tape fallback and configured operation/layer routes with backend-selection parity, then require paired numerical proof for the scalar loss and all " +
				strconv.Itoa(contract.ConfiguredGradientCount) + " ordered gradients plus paired end-to-end validation—this is not a fusion or application-win claim (PS6116 advisory, no automatic fix)",
			Related: []analysis.RelatedInformation{
				{Pos: chain.loss.Pos(), End: chain.loss.End(), Message: "exact scalar-loss stage in the configured objective"},
				{Pos: chain.backward.Pos(), End: chain.backward.End(), Message: "exact backward stage that consumes the scalar loss"},
				{Pos: chain.gradient.Pos(), End: chain.gradient.End(), Message: "exact parameter-ordered gradient extraction"},
			},
		})
	}
	return nil, nil
}

func ps6116Contracts(configured []config.ForwardLossBackwardGraphContract) []*config.ForwardLossBackwardGraphContract {
	nameCount := make(map[string]int)
	siteCount := make(map[string]int)
	for index := range configured {
		contract := &configured[index]
		if contract.Valid() {
			nameCount[contract.Name]++
			siteCount[contract.ObjectiveSite]++
		}
	}
	var result []*config.ForwardLossBackwardGraphContract
	for index := range configured {
		contract := &configured[index]
		if contract.Valid() && nameCount[contract.Name] == 1 && siteCount[contract.ObjectiveSite] == 1 {
			result = append(result, contract)
		}
	}
	slices.SortFunc(result, func(left, right *config.ForwardLossBackwardGraphContract) int {
		return strings.Compare(left.Name, right.Name)
	})
	return result
}

func ps6116CallablesResolve(pass *analysis.Pass, contract *config.ForwardLossBackwardGraphContract) bool {
	for _, callable := range []string{
		contract.RecorderFactoryCallable, contract.RecorderBindingCallable, contract.ForwardCallable,
		contract.LossCallable, contract.BackwardCallable, contract.ParameterOrderCallable, contract.GradientCallable,
	} {
		if !ps6116CallableResolves(pass, callable) {
			return false
		}
	}
	return true
}

func ps6116CallableResolves(pass *analysis.Pass, id string) bool {
	packages := []*types.Package{pass.Pkg}
	seen := map[string]bool{pass.Pkg.Path(): true}
	for index := 0; index < len(packages); index++ {
		for _, imported := range packages[index].Imports() {
			if !seen[imported.Path()] {
				seen[imported.Path()] = true
				packages = append(packages, imported)
			}
		}
	}
	for _, pkg := range packages {
		prefix := pkg.Path() + "."
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(id, prefix), ".")
		if len(parts) == 1 {
			function, ok := pkg.Scope().Lookup(parts[0]).(*types.Func)
			return ok && ps6116ConcreteSignature(function)
		}
		if len(parts) != 2 {
			return false
		}
		typeName, ok := pkg.Scope().Lookup(parts[0]).(*types.TypeName)
		if !ok {
			return false
		}
		named, ok := types.Unalias(typeName.Type()).(*types.Named)
		if !ok {
			return false
		}
		for methodIndex := 0; methodIndex < named.NumMethods(); methodIndex++ {
			method := named.Method(methodIndex)
			if method.Name() == parts[1] {
				return ps6116ConcreteSignature(method)
			}
		}
		return false
	}
	return false
}

func ps6116ConcreteSignature(function *types.Func) bool {
	if function == nil || function.Pkg() == nil {
		return false
	}
	signature, ok := function.Type().(*types.Signature)
	if !ok || signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 {
		return false
	}
	if signature.Recv() == nil {
		return true
	}
	_, dynamic := types.Unalias(signature.Recv().Type()).Underlying().(*types.Interface)
	return !dynamic
}

func ps6116DirectCall(pass *analysis.Pass, file *ast.File, call *ast.CallExpr, id string) bool {
	if call == nil || call.Ellipsis.IsValid() {
		return false
	}
	if strings.HasPrefix(id, "C.") {
		name, ok := ps6110CgoName(pass, file, call)
		return ok && id == "C."+name && !ps6115ExplicitInstantiation(call.Fun)
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function == nil || signature == nil || signature.TypeParams().Len() != 0 ||
		signature.RecvTypeParams().Len() != 0 || ps6115ExplicitInstantiation(call.Fun) ||
		ps6087FunctionID(pass, call) != id {
		return false
	}
	if signature.Recv() == nil {
		switch expression := ps2110Unparen(call.Fun).(type) {
		case *ast.Ident:
			return pass.TypesInfo.Uses[expression] == function
		case *ast.SelectorExpr:
			return pass.TypesInfo.Selections[expression] == nil && pass.TypesInfo.Uses[expression.Sel] == function
		default:
			return false
		}
	}
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || ps6091MethodExpression(pass, call.Fun) {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.MethodVal || len(selection.Index()) != 1 || selection.Obj() != function {
		return false
	}
	_, dynamic := types.Unalias(selection.Recv()).Underlying().(*types.Interface)
	return !dynamic
}

func ps6116Match(pass *analysis.Pass, declaration *ast.FuncDecl, contract *config.ForwardLossBackwardGraphContract) (ps6116Chain, bool) {
	file := ps6115FileContaining(pass, declaration)
	if file == nil {
		return ps6116Chain{}, false
	}
	parents := ps6087Parents(declaration.Body)
	unreachable := ps2144Unreachable(pass, declaration.Body)
	ids := []string{
		contract.RecorderFactoryCallable, contract.RecorderBindingCallable, contract.ForwardCallable,
		contract.LossCallable, contract.BackwardCallable, contract.ParameterOrderCallable, contract.GradientCallable,
	}
	matches := make(map[string][]*ast.CallExpr, len(ids))
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || ps2144PositionIn(call.Pos(), unreachable) {
			return true
		}
		for _, id := range ids {
			if ps6116DirectCall(pass, file, call, id) && ps6116SafeVariadicArity(pass, call) {
				matches[id] = append(matches[id], call)
			}
		}
		return true
	})
	for _, id := range ids {
		if len(matches[id]) != 1 || ps6115AsyncCall(matches[id][0], parents) {
			return ps6116Chain{}, false
		}
	}
	factory := matches[contract.RecorderFactoryCallable][0]
	binding := matches[contract.RecorderBindingCallable][0]
	forward := matches[contract.ForwardCallable][0]
	loss := matches[contract.LossCallable][0]
	backward := matches[contract.BackwardCallable][0]
	parameters := matches[contract.ParameterOrderCallable][0]
	gradient := matches[contract.GradientCallable][0]
	if !(factory.Pos() < binding.Pos() && binding.Pos() < forward.Pos() && forward.Pos() < loss.Pos() &&
		loss.Pos() < backward.Pos() && backward.Pos() < parameters.Pos() && parameters.Pos() < gradient.Pos()) {
		return ps6116Chain{}, false
	}

	tape, factoryOK := ps6116AssignedObject(pass, factory, 0, parents)
	recording, bindingOK := ps6116AssignedObject(pass, binding, 0, parents)
	logits, forwardOK := ps6116AssignedObject(pass, forward, 0, parents)
	lossValue, lossOK := ps6116AssignedObject(pass, loss, 0, parents)
	parameterSlice, parametersOK := ps6116AssignedObject(pass, parameters, 0, parents)
	if !factoryOK || !bindingOK || !forwardOK || !lossOK || !parametersOK ||
		!ps6116RecorderContext(pass, factory, binding, contract) ||
		!ps6116ArgumentIs(pass, binding, contract.RecorderBindingArgument, tape) ||
		!ps6116ArgumentIs(pass, forward, contract.ForwardRecorderArgument, recording) ||
		!ps6116ArgumentIs(pass, loss, contract.LossRecorderArgument, recording) ||
		!ps6116ArgumentIs(pass, loss, contract.LossForwardArgument, logits) ||
		!ps6116ArgumentIs(pass, backward, contract.BackwardLossArgument, lossValue) ||
		ps6116ReceiverObject(pass, backward) != tape || ps6116ReceiverObject(pass, gradient) != tape ||
		!ps6116Reduction(pass, loss, contract) {
		return ps6116Chain{}, false
	}

	factoryStatement := ps6116TopLevelStatement(factory, declaration.Body, parents)
	bindingStatement := ps6116TopLevelStatement(binding, declaration.Body, parents)
	forwardStatement := ps6116TopLevelStatement(forward, declaration.Body, parents)
	lossStatement := ps6116TopLevelStatement(loss, declaration.Body, parents)
	backwardStatement := ps6116TopLevelStatement(backward, declaration.Body, parents)
	parameterStatement := ps6116TopLevelStatement(parameters, declaration.Body, parents)
	if factoryStatement == nil || bindingStatement == nil || forwardStatement == nil || lossStatement == nil ||
		backwardStatement == nil || parameterStatement == nil ||
		!ps6116OnlyCalls(factoryStatement, factory) || !ps6116OnlyCalls(bindingStatement, binding) ||
		!ps6116OnlyCalls(forwardStatement, forward) || !ps6116ForwardInputs(pass, forward, contract) ||
		!ps6116OnlyCalls(lossStatement, loss) || !ps6116OnlyCalls(parameterStatement, parameters) ||
		!ps6116OnlyTerminalBetween(declaration.Body, forwardStatement, lossStatement) ||
		!ps6116OnlyTerminalBetween(declaration.Body, lossStatement, backwardStatement) ||
		!ps6116BackwardGuard(pass, backward, backwardStatement, parents) {
		return ps6116Chain{}, false
	}

	finalReturn, lossReturnUse := ps6116FinalReturn(pass, declaration.Body, lossValue)
	if finalReturn == nil || !ps6116OnlyUses(pass, declaration, logits, loss.Args[contract.LossForwardArgument-1]) ||
		!ps6116OnlyUses(pass, declaration, lossValue, backward.Args[contract.BackwardLossArgument-1], lossReturnUse) ||
		!ps6116OnlyUses(pass, declaration, recording,
			forward.Args[contract.ForwardRecorderArgument-1], loss.Args[contract.LossRecorderArgument-1]) ||
		!ps6116OnlyUses(pass, declaration, tape,
			binding.Args[contract.RecorderBindingArgument-1], backward.Fun, gradient.Fun) {
		return ps6116Chain{}, false
	}

	rangeStatement, key, parameter, gradientSlice, lenUse, ok := ps6116GradientOrder(pass, declaration, gradient, parameterSlice, parents)
	gradientAllocation := ps6116TopLevelStatement(lenUse, declaration.Body, parents)
	if !ok || rangeStatement.Pos() <= parameterStatement.Pos() ||
		gradientAllocation == nil ||
		!ps6116ArgumentIs(pass, gradient, contract.GradientParameterArgument, parameter) ||
		!ps6116OnlyUses(pass, declaration, parameter, gradient.Args[contract.GradientParameterArgument-1]) ||
		!ps6116OnlyUses(pass, declaration, parameterSlice, rangeStatement.X, lenUse) ||
		!ps6116GradientSliceUses(pass, declaration, gradientSlice, key, rangeStatement,
			ps6116GradientStore(gradient, parents), finalReturn, parents) ||
		key == nil || !ps6116ExactBody(pass, declaration.Body, parents, factoryStatement, bindingStatement,
		forwardStatement, forward, lossStatement, loss, backwardStatement, parameterStatement,
		gradientAllocation, rangeStatement, finalReturn) {
		return ps6116Chain{}, false
	}
	return ps6116Chain{forward: forward, loss: loss, backward: backward, gradient: gradient}, true
}

func ps6116RecorderContext(pass *analysis.Pass, factory, binding *ast.CallExpr,
	contract *config.ForwardLossBackwardGraphContract) bool {
	index := contract.RecorderFactoryBackendArgument - 1
	if index < 0 || index >= len(factory.Args) {
		return false
	}
	backend, ok := ps2110Unparen(factory.Args[index]).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	selection := pass.TypesInfo.Selections[backend]
	if selection == nil || selection.Kind() != types.FieldVal || len(selection.Index()) != 1 {
		return false
	}
	context := ps6114ExprObject(pass, backend.X)
	return context != nil && ps6116ReceiverObject(pass, binding) == context
}

func ps6116SafeVariadicArity(pass *analysis.Pass, call *ast.CallExpr) bool {
	_, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature == nil || call.Ellipsis.IsValid() {
		return false
	}
	if !signature.Variadic() {
		return true
	}
	// Variadic stage APIs are accepted only in their fixed-argument form. This
	// covers GoAI's NewTapeOn(backend) and CrossEntropy(ctx, logits, targets)
	// defaults without accepting unbound tape or loss options.
	return len(call.Args) == signature.Params().Len()-1
}

func ps6116OnlyCalls(node ast.Node, allowed ...*ast.CallExpr) bool {
	wanted := make(map[*ast.CallExpr]bool, len(allowed))
	for _, call := range allowed {
		wanted[call] = true
	}
	valid := true
	ast.Inspect(node, func(node ast.Node) bool {
		if !valid {
			return false
		}
		if call, ok := node.(*ast.CallExpr); ok && !wanted[call] {
			valid = false
			return false
		}
		return true
	})
	return valid
}

func ps6116ForwardInputs(pass *analysis.Pass, call *ast.CallExpr, contract *config.ForwardLossBackwardGraphContract) bool {
	recorder := contract.ForwardRecorderArgument - 1
	for index, argument := range call.Args {
		if index == recorder {
			continue
		}
		identifier, ok := ps2110Unparen(argument).(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(identifier) == nil {
			return false
		}
	}
	selector, method := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !method || pass.TypesInfo.Selections[selector] == nil {
		return true
	}
	receiver, ok := ps2110Unparen(selector.X).(*ast.Ident)
	return ok && pass.TypesInfo.ObjectOf(receiver) != nil
}

func ps6116GradientSliceUses(pass *analysis.Pass, declaration *ast.FuncDecl, gradientSlice, key types.Object,
	rangeStatement *ast.RangeStmt, store ast.Node, finalReturn *ast.ReturnStmt, parents map[ast.Node]ast.Node,
) bool {
	valid := true
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[identifier] != gradientSlice {
			return true
		}
		if ps6099NodeWithin(identifier, store) || ps6116DirectReturnUse(identifier, finalReturn) ||
			ps6116NilGradientGuard(pass, identifier, key, rangeStatement, parents) {
			return true
		}
		valid = false
		return false
	})
	return valid
}

func ps6116NilGradientGuard(pass *analysis.Pass, identifier *ast.Ident, key types.Object,
	rangeStatement *ast.RangeStmt, parents map[ast.Node]ast.Node,
) bool {
	indexed, ok := parents[identifier].(*ast.IndexExpr)
	if !ok || ps6114ExprObject(pass, indexed.X) != pass.TypesInfo.ObjectOf(identifier) ||
		ps6114ExprObject(pass, indexed.Index) != key {
		return false
	}
	binary, ok := parents[indexed].(*ast.BinaryExpr)
	if !ok || binary.Op.String() != "==" || !ps6099NodeWithin(indexed, binary.X) {
		return false
	}
	nilIdentifier, ok := ps2110Unparen(binary.Y).(*ast.Ident)
	nilObject := pass.TypesInfo.ObjectOf(nilIdentifier)
	if !ok || nilIdentifier.Name != "nil" || nilObject == nil || nilObject.Parent() != types.Universe {
		return false
	}
	guard, ok := parents[binary].(*ast.IfStmt)
	if !ok || guard.Cond != binary || guard.Else != nil || len(guard.Body.List) != 1 ||
		!ps6099NodeWithin(guard, rangeStatement.Body) {
		return false
	}
	_, terminal := guard.Body.List[0].(*ast.ReturnStmt)
	return terminal
}

func ps6116AssignedObject(pass *analysis.Pass, call *ast.CallExpr, result int, parents map[ast.Node]ast.Node) (types.Object, bool) {
	current := ast.Node(call)
	for {
		parent := parents[current]
		if paren, ok := parent.(*ast.ParenExpr); ok {
			current = paren
			continue
		}
		switch statement := parent.(type) {
		case *ast.AssignStmt:
			if len(statement.Rhs) != 1 || ps2110Unparen(statement.Rhs[0]) != call || result >= len(statement.Lhs) {
				return nil, false
			}
			identifier, ok := ps2110Unparen(statement.Lhs[result]).(*ast.Ident)
			if !ok {
				return nil, false
			}
			object := pass.TypesInfo.ObjectOf(identifier)
			return object, identifier.Name != "_" && object != nil
		case *ast.ValueSpec:
			if len(statement.Values) != 1 || ps2110Unparen(statement.Values[0]) != call || result >= len(statement.Names) {
				return nil, false
			}
			object := pass.TypesInfo.ObjectOf(statement.Names[result])
			return object, statement.Names[result].Name != "_" && object != nil
		default:
			return nil, false
		}
	}
}

func ps6116ArgumentIs(pass *analysis.Pass, call *ast.CallExpr, position int, object types.Object) bool {
	index := position - 1
	return index >= 0 && index < len(call.Args) && ps6114ExprObject(pass, call.Args[index]) == object
}

func ps6116ReceiverObject(pass *analysis.Pass, call *ast.CallExpr) types.Object {
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	return ps6114ExprObject(pass, selector.X)
}

func ps6116Reduction(pass *analysis.Pass, call *ast.CallExpr, contract *config.ForwardLossBackwardGraphContract) bool {
	if contract.LossReductionConstant == "" {
		return true
	}
	index := contract.LossReductionArgument - 1
	if index < 0 || index >= len(call.Args) {
		return false
	}
	expected := pass.TypesInfo.TypeOf(call.Args[index])
	if function, signature, ok := typedCallee(pass, call.Fun); ok && function != nil && signature != nil && index < signature.Params().Len() {
		expected = signature.Params().At(index).Type()
	}
	return expected != nil && ps6113AddConstant(pass, call.Args[index], contract.LossReductionConstant, contract.LossReductionConstantValue, expected)
}

func ps6116TopLevelStatement(node ast.Node, body *ast.BlockStmt, parents map[ast.Node]ast.Node) ast.Stmt {
	for current := node; current != nil; current = parents[current] {
		if parents[current] == body {
			statement, _ := current.(ast.Stmt)
			return statement
		}
	}
	return nil
}

func ps6116OnlyTerminalBetween(body *ast.BlockStmt, first, second ast.Stmt) bool {
	firstIndex, secondIndex := -1, -1
	for index, statement := range body.List {
		if statement == first {
			firstIndex = index
		}
		if statement == second {
			secondIndex = index
		}
	}
	if firstIndex < 0 || secondIndex <= firstIndex {
		return false
	}
	for _, statement := range body.List[firstIndex+1 : secondIndex] {
		guard, ok := statement.(*ast.IfStmt)
		if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 {
			return false
		}
		if _, ok := guard.Body.List[0].(*ast.ReturnStmt); !ok {
			return false
		}
	}
	return true
}

func ps6116ExactBody(pass *analysis.Pass, body *ast.BlockStmt, parents map[ast.Node]ast.Node,
	factory, binding, forward ast.Stmt, forwardCall *ast.CallExpr, loss ast.Stmt, lossCall *ast.CallExpr,
	backward, parameters, gradientAllocation ast.Stmt, gradientRange *ast.RangeStmt, finalReturn *ast.ReturnStmt,
) bool {
	statements := body.List
	index := 0
	consume := func(want ast.Stmt) bool {
		if index >= len(statements) || statements[index] != want {
			return false
		}
		index++
		return true
	}
	if !consume(factory) || !consume(binding) || !consume(forward) {
		return false
	}
	if index < len(statements) && statements[index] != loss {
		if !ps6116ResultGuard(pass, forwardCall, statements[index], parents) {
			return false
		}
		index++
	}
	if !consume(loss) {
		return false
	}
	if index < len(statements) && statements[index] != backward {
		if !ps6116ResultGuard(pass, lossCall, statements[index], parents) {
			return false
		}
		index++
	}
	return consume(backward) && consume(parameters) && consume(gradientAllocation) && consume(gradientRange) &&
		consume(finalReturn) && index == len(statements)
}

func ps6116ResultGuard(pass *analysis.Pass, call *ast.CallExpr, statement ast.Stmt, parents map[ast.Node]ast.Node) bool {
	assignment, ok := parents[call].(*ast.AssignStmt)
	if !ok || len(assignment.Rhs) != 1 || len(assignment.Lhs) < 2 {
		return false
	}
	guard, ok := statement.(*ast.IfStmt)
	if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 {
		return false
	}
	if _, ok := guard.Body.List[0].(*ast.ReturnStmt); !ok {
		return false
	}
	binary, ok := ps2110Unparen(guard.Cond).(*ast.BinaryExpr)
	if !ok || binary.Op.String() != "!=" {
		return false
	}
	errorObject := ps6114ExprObject(pass, binary.X)
	nilIdentifier, ok := ps2110Unparen(binary.Y).(*ast.Ident)
	if !ok || nilIdentifier.Name != "nil" {
		return false
	}
	nilObject := pass.TypesInfo.ObjectOf(nilIdentifier)
	if errorObject == nil || nilObject == nil || nilObject.Parent() != types.Universe {
		return false
	}
	for _, expression := range assignment.Lhs[1:] {
		if ps6114ExprObject(pass, expression) == errorObject {
			return true
		}
	}
	return false
}

func ps6116BackwardGuard(pass *analysis.Pass, call *ast.CallExpr, statement ast.Stmt, parents map[ast.Node]ast.Node) bool {
	guard, ok := statement.(*ast.IfStmt)
	if !ok || guard.Else != nil || len(guard.Body.List) != 1 || !ps6099NodeWithin(call, guard.Init) {
		return false
	}
	if _, ok := guard.Body.List[0].(*ast.ReturnStmt); !ok {
		return false
	}
	assignment, ok := guard.Init.(*ast.AssignStmt)
	if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || ps2110Unparen(assignment.Rhs[0]) != call {
		return false
	}
	errorIdentifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	if !ok || errorIdentifier.Name == "_" {
		return false
	}
	errorObject := pass.TypesInfo.ObjectOf(errorIdentifier)
	binary, ok := ps2110Unparen(guard.Cond).(*ast.BinaryExpr)
	if !ok || binary.Op.String() != "!=" || ps6114ExprObject(pass, binary.X) != errorObject {
		return false
	}
	nilIdentifier, ok := ps2110Unparen(binary.Y).(*ast.Ident)
	if !ok || nilIdentifier.Name != "nil" {
		return false
	}
	nilObject := pass.TypesInfo.ObjectOf(nilIdentifier)
	return errorObject != nil && nilObject != nil && nilObject.Parent() == types.Universe && parents[call] == assignment
}

func ps6116FinalReturn(pass *analysis.Pass, body *ast.BlockStmt, loss types.Object) (*ast.ReturnStmt, ast.Node) {
	if len(body.List) == 0 {
		return nil, nil
	}
	result, ok := body.List[len(body.List)-1].(*ast.ReturnStmt)
	if !ok {
		return nil, nil
	}
	var use ast.Node
	for _, expression := range result.Results {
		if ps6114ExprObject(pass, expression) == loss {
			if use != nil {
				return nil, nil
			}
			use = expression
		}
	}
	if use == nil {
		return nil, nil
	}
	return result, use
}

func ps6116DirectReturnUse(identifier *ast.Ident, result *ast.ReturnStmt) bool {
	for _, expression := range result.Results {
		if ps2110Unparen(expression) == identifier {
			return true
		}
	}
	return false
}

func ps6116OnlyUses(pass *analysis.Pass, declaration *ast.FuncDecl, object types.Object, allowed ...ast.Node) bool {
	valid := true
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[identifier] != object {
			return true
		}
		for _, region := range allowed {
			if region != nil && ps6099NodeWithin(identifier, region) {
				return true
			}
		}
		valid = false
		return false
	})
	return valid
}

func ps6116GradientOrder(pass *analysis.Pass, declaration *ast.FuncDecl, gradient *ast.CallExpr,
	parameterSlice types.Object, parents map[ast.Node]ast.Node,
) (*ast.RangeStmt, types.Object, types.Object, types.Object, ast.Node, bool) {
	var rangeStatement *ast.RangeStmt
	for current := ast.Node(gradient); current != nil; current = parents[current] {
		if loop, ok := current.(*ast.RangeStmt); ok {
			rangeStatement = loop
			break
		}
	}
	if rangeStatement == nil || ps6116TopLevelStatement(rangeStatement, declaration.Body, parents) != rangeStatement ||
		ps6114ExprObject(pass, rangeStatement.X) != parameterSlice {
		return nil, nil, nil, nil, nil, false
	}
	keyIdentifier, keyOK := ps2110Unparen(rangeStatement.Key).(*ast.Ident)
	parameterIdentifier, parameterOK := ps2110Unparen(rangeStatement.Value).(*ast.Ident)
	if !keyOK || !parameterOK || keyIdentifier.Name == "_" || parameterIdentifier.Name == "_" {
		return nil, nil, nil, nil, nil, false
	}
	key := pass.TypesInfo.ObjectOf(keyIdentifier)
	parameter := pass.TypesInfo.ObjectOf(parameterIdentifier)
	store := ps6116GradientStore(gradient, parents)
	assignment, ok := store.(*ast.AssignStmt)
	if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || ps2110Unparen(assignment.Rhs[0]) != gradient {
		return nil, nil, nil, nil, nil, false
	}
	indexed, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.IndexExpr)
	if !ok || ps6114ExprObject(pass, indexed.Index) != key {
		return nil, nil, nil, nil, nil, false
	}
	gradientSlice := ps6114ExprObject(pass, indexed.X)
	lenUse := ps6116GradientSliceLength(pass, declaration, gradientSlice, parameterSlice)
	if key == nil || parameter == nil || gradientSlice == nil || lenUse == nil {
		return nil, nil, nil, nil, nil, false
	}
	if !ps6116ExactGradientLoopBody(pass, rangeStatement, assignment, gradientSlice, key) {
		return nil, nil, nil, nil, nil, false
	}
	unsafeControl := false
	ast.Inspect(rangeStatement.Body, func(node ast.Node) bool {
		switch node.(type) {
		case *ast.BranchStmt, *ast.GoStmt, *ast.DeferStmt:
			unsafeControl = true
			return false
		}
		return true
	})
	if unsafeControl {
		return nil, nil, nil, nil, nil, false
	}
	return rangeStatement, key, parameter, gradientSlice, lenUse, true
}

func ps6116ExactGradientLoopBody(pass *analysis.Pass, loop *ast.RangeStmt, store *ast.AssignStmt,
	gradientSlice, key types.Object,
) bool {
	if len(loop.Body.List) < 1 || len(loop.Body.List) > 2 || loop.Body.List[0] != store {
		return false
	}
	if len(loop.Body.List) == 1 {
		return true
	}
	guard, ok := loop.Body.List[1].(*ast.IfStmt)
	if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 {
		return false
	}
	condition, ok := ps2110Unparen(guard.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op.String() != "==" {
		return false
	}
	indexed, ok := ps2110Unparen(condition.X).(*ast.IndexExpr)
	if !ok || ps6114ExprObject(pass, indexed.X) != gradientSlice || ps6114ExprObject(pass, indexed.Index) != key {
		return false
	}
	nilIdentifier, ok := ps2110Unparen(condition.Y).(*ast.Ident)
	nilObject := pass.TypesInfo.ObjectOf(nilIdentifier)
	if !ok || nilIdentifier.Name != "nil" || nilObject == nil || nilObject.Parent() != types.Universe {
		return false
	}
	result, ok := guard.Body.List[0].(*ast.ReturnStmt)
	return ok && ps6116GradientGuardReturnSafe(pass, result)
}

func ps6116GradientGuardReturnSafe(pass *analysis.Pass, result *ast.ReturnStmt) bool {
	valid := true
	ast.Inspect(result, func(node ast.Node) bool {
		if !valid {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		function, signature, direct := typedCallee(pass, call.Fun)
		if !direct || function == nil || signature == nil || ps6087FunctionID(pass, call) != "fmt.Errorf" || call.Ellipsis.IsValid() {
			valid = false
			return false
		}
		return true
	})
	return valid
}

func ps6116GradientStore(gradient *ast.CallExpr, parents map[ast.Node]ast.Node) ast.Node {
	for current := ast.Node(gradient); current != nil; current = parents[current] {
		if statement, ok := current.(*ast.AssignStmt); ok {
			return statement
		}
		if _, boundary := current.(*ast.RangeStmt); boundary {
			return nil
		}
	}
	return nil
}

func ps6116GradientSliceLength(pass *analysis.Pass, declaration *ast.FuncDecl, gradientSlice, parameterSlice types.Object) ast.Node {
	var result ast.Node
	count := 0
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Defs[identifier] != gradientSlice {
			return true
		}
		count++
		parentAssignment, ok := ps6116ParentAssignment(identifier, declaration.Body)
		if !ok || len(parentAssignment.Rhs) != 1 {
			return true
		}
		makeCall, ok := ps2110Unparen(parentAssignment.Rhs[0]).(*ast.CallExpr)
		if !ok || len(makeCall.Args) != 2 || !ps6116Builtin(pass, makeCall.Fun, "make") {
			return true
		}
		length, ok := ps2110Unparen(makeCall.Args[1]).(*ast.CallExpr)
		if ok && len(length.Args) == 1 && ps6116Builtin(pass, length.Fun, "len") && ps6114ExprObject(pass, length.Args[0]) == parameterSlice {
			result = length.Args[0]
		}
		return true
	})
	if count != 1 {
		return nil
	}
	return result
}

func ps6116ParentAssignment(identifier *ast.Ident, body *ast.BlockStmt) (*ast.AssignStmt, bool) {
	var result *ast.AssignStmt
	ast.Inspect(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assignment.Lhs {
			if ps2110Unparen(lhs) == identifier {
				result = assignment
				return false
			}
		}
		return true
	})
	return result, result != nil
}

func ps6116Builtin(pass *analysis.Pass, expression ast.Expr, name string) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	object, builtin := pass.TypesInfo.Uses[identifier].(*types.Builtin)
	return ok && builtin && object.Name() == name
}

package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6118 implements owner issue #879 as a configured cross-step residency
// advisory. It is separate from whole-objective fusion and intra-objective
// synchronization checks: the matched boundary surrounds a host optimizer.
var PS6118 = register(&lint.Check{
	ID:          "PS6118",
	Category:    "verify",
	Slug:        "cross-step-accelerator-residency",
	Level:       lint.LevelAggressive,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"crossStepAcceleratorResidencyContracts"},
	Doc: lint.Documentation{
		Title: "an accelerator objective materializes training state around a host optimizer on every step",
		Text: `A fused accelerator objective can still lose its cross-step leverage
when every iteration uploads the same parameter set, materializes one dense
host gradient per parameter, runs a host optimizer, and uploads the updated
parameters again. If only a scalar metric is observed between steps, an
explicit resident training session may be worth validating.

PS6118 is deliberately config-gated, narrow, and fail-closed. A contract binds
one exact non-generic function, one exact fixed-count loop, direct objective,
optimizer, and scalar-observer callables, receiver-owned parameter roles, an
exact gradient callback, scalar/gradient/error result roles, returned-error
control flow, positive parameter and gradient byte estimates, exact target and
memory model, and the optimizer semantics. The configured avoidable bytes
must equal parameter-upload bytes plus dense-gradient materialization bytes.
They are an owner estimate per step, not a runtime measurement.

V1 accepts only a canonical zero-based fixed-count for loop. A concrete method
objective must directly declare distinct scalar, dense-gradient, and error
locals. An exact err-not-nil guard that returns that error must follow. The host
optimizer must be a concrete method on its parameter-owning receiver and accept
one exact callback that maps its parameter through an index into the objective's
gradient collection; its sole error result must have the same returned-error
guard. The immediately
following observer must consume only a statically scalar-like host metric.
Those are the loop's only three calls. No other branch, return, nested loop,
closure, go, or defer statement is accepted. Receiver, result, callback, and
error locals may have no additional use, alias, rebinding, address-taking, or
escape. Duplicate contracts, ambiguous loops or calls, unreachable loops,
dynamic dispatch, extra host consumers, source-visible checkpoint or
materialization requirements, and already-resident sessions stay silent.

Configuration must explicitly affirm dense per-parameter gradients, stable
parameter/gradient ordering, every-step upload, materialization, host update,
and next-step re-upload, scalar-only observation, no hidden state sync or
intermediate checkpoint need, synchronous non-retaining calls, no custom
hooks, aliases, escapes, or concurrent session access, serialized resident
session construction, exact dtype, layout, and
optimizer coverage, known device storage and transfer behavior, a confirmed
resident-session opportunity, and lifetime, numerical, checkpoint, error/panic,
mutation, ownership, autograd, and backend-selection parity. This verbosity is
intentional: Go syntax and API names cannot establish device/storage state or
resident-session safety.

There is NO automatic fix and the diagnostic does not claim a win. The remedy is an explicit,
bounded resident-session/state handle with scalar-only Step results and
explicit Sync/checkpoint materialization. Preserve the portable fallback and
adopt a resident path only after paired order-alternating end-to-end validation
of the exact target, geometry, dtype, optimizer, numerical tolerances, lifetime,
checkpoint, error, mutation, ownership, autograd, concurrency, and backend
contracts.`,
		Before: `for step := 0; step < 20; step++ {
	loss, gradients, err := model.LossAndGrad(context, batch)
	if err != nil { return err }
	if err := optimizer.Step(func(parameter *Tensor) *Tensor {
		return gradients[parameterIndex[parameter]]
	}); err != nil { return err }
	observe(loss)
}`,
		After: `// Candidate only: keep parameters, gradients, and optimizer state in
// an explicit resident session; return only loss from Step and materialize
// parameters only at explicit Sync/checkpoint boundaries. Validate end to end.`,
		MeasuredWin: `GoAI PR #1202 (head b86d9a755090df93ed64a30583774f30eefa0c3a)
measured a production-shape Apple M2 Pro F32 GPT objective at about 20.75 ms,
including about 1.91 ms parameter upload and 4.56 ms dense-gradient copy-out;
host AdamW added about 10.5 ms. A resident F32 AdamW session measured
15.529600 ms versus 28.190119 ms for the exact fused-objective plus host-F32-
AdamW control: 1.81890x paired median and 1.78324x worst of 21 aligned pairs.
That is one pinned target/model/shape/dtype/optimizer result, not a general win.
An unresolved post-merge P1 review on that exact head identified concurrent
construction around a shared graph cache as unsafe. That reinforces that
construction serialization, synchronization, and ownership require an explicit
project contract and validation rather than an inferred rewrite.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6118",
		Doc:  "configured repeated accelerator objective and host optimizer boundaries that merit resident-session validation",
		Run:  runPS6118,
	},
})

type ps6118Match struct {
	objective *ast.CallExpr
	optimizer *ast.CallExpr
	observer  *ast.CallExpr
}

func runPS6118(pass *analysis.Pass) (any, error) {
	return runPS6118WithContracts(pass, config.Current().CrossStepAcceleratorResidencyContracts)
}

func runPS6118WithContracts(pass *analysis.Pass, configured []config.CrossStepAcceleratorResidencyContract) (any, error) {
	declarations := ps6115Declarations(pass)
	for _, contract := range ps6118Contracts(configured) {
		if contract.ExistingResidentSession || contract.IntentionalHostOptimizer {
			continue
		}
		declaration := declarations[contract.ConfiguredSite]
		if declaration == nil || ps6113GenericFunction(pass, declaration) {
			continue
		}
		match, ok := ps6118UniqueMatch(pass, declaration, contract)
		if !ok {
			continue
		}
		total := contract.ConfiguredAvoidableBytes * contract.ConfiguredLoopIterations
		pass.Report(analysis.Diagnostic{
			Pos: match.objective.Pos(), End: match.objective.End(),
			Message: contract.Name + ": configured cross-step accelerator boundary uploads " +
				strconv.FormatInt(contract.ConfiguredParameterBytes, 10) + " parameter bytes and materializes " +
				strconv.FormatInt(contract.ConfiguredGradientBytes, 10) + " dense-gradient bytes per step around host optimizer " +
				contract.OptimizerCallable + "; " + strconv.FormatInt(contract.ConfiguredAvoidableBytes, 10) +
				" configured avoidable bytes/step (" + strconv.FormatInt(total, 10) + " across " +
				strconv.FormatInt(contract.ConfiguredLoopIterations, 10) + " fixed iterations) on " + contract.TargetGOOS + "/" +
				contract.TargetGOARCH + " " + contract.MemoryModel + " memory. Validate an explicit resident session with scalar-only Step, explicit Sync/checkpoint materialization, and paired order-alternating end-to-end gates; this diagnostic does not claim a win (PS6118 advisory, no automatic fix)",
			Related: []analysis.RelatedInformation{
				{Pos: match.optimizer.Pos(), End: match.optimizer.End(), Message: "configured parameter-owning host optimizer consumes dense gradients through the exact receiver-parameter callback"},
				{Pos: match.observer.Pos(), End: match.observer.End(), Message: "configured between-step observation consumes only the scalar objective result"},
			},
		})
	}
	return nil, nil
}

func ps6118Contracts(configured []config.CrossStepAcceleratorResidencyContract) []*config.CrossStepAcceleratorResidencyContract {
	nameCount := make(map[string]int)
	claimCount := make(map[string]int)
	for index := range configured {
		contract := &configured[index]
		if contract.Valid() {
			nameCount[contract.Name]++
			claimCount[ps6118ContractKey(contract)]++
		}
	}
	var result []*config.CrossStepAcceleratorResidencyContract
	for index := range configured {
		contract := &configured[index]
		if contract.Valid() && nameCount[contract.Name] == 1 && claimCount[ps6118ContractKey(contract)] == 1 {
			result = append(result, contract)
		}
	}
	slices.SortFunc(result, func(left, right *config.CrossStepAcceleratorResidencyContract) int {
		return strings.Compare(left.Name, right.Name)
	})
	return result
}

func ps6118ContractKey(contract *config.CrossStepAcceleratorResidencyContract) string {
	return contract.ConfiguredSite + "\x00" + contract.ObjectiveCallable + "\x00" + contract.OptimizerCallable
}

func ps6118UniqueMatch(pass *analysis.Pass, declaration *ast.FuncDecl, contract *config.CrossStepAcceleratorResidencyContract) (ps6118Match, bool) {
	unreachable := ps2144Unreachable(pass, declaration.Body)
	var matches []ps6118Match
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if node != declaration.Body {
			switch node.(type) {
			case *ast.FuncLit:
				return false
			}
		}
		loop, ok := node.(*ast.ForStmt)
		if !ok || ps2144PositionIn(loop.Pos(), unreachable) || !ps6118FixedLoop(pass, loop, contract.ConfiguredLoopIterations) {
			return true
		}
		if match, matched := ps6118Loop(pass, loop, contract); matched {
			matches = append(matches, match)
		}
		return false
	})
	if len(matches) != 1 {
		return ps6118Match{}, false
	}
	return matches[0], true
}

func ps6118FixedLoop(pass *analysis.Pass, loop *ast.ForStmt, configured int64) bool {
	initial, ok := loop.Init.(*ast.AssignStmt)
	if !ok || initial.Tok != token.DEFINE || len(initial.Lhs) != 1 || len(initial.Rhs) != 1 || !ps6118IntegerConstant(pass, initial.Rhs[0], 0) {
		return false
	}
	indexID, ok := ps2110Unparen(initial.Lhs[0]).(*ast.Ident)
	if !ok || indexID.Name == "_" {
		return false
	}
	index := pass.TypesInfo.Defs[indexID]
	if index == nil {
		return false
	}
	condition, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op != token.LSS || ps6114ExprObject(pass, condition.X) != index || !ps6118IntegerConstant(pass, condition.Y, configured) {
		return false
	}
	post, ok := loop.Post.(*ast.IncDecStmt)
	return ok && post.Tok == token.INC && ps6114ExprObject(pass, post.X) == index && !ps6118ObjectUsed(pass, loop.Body, index)
}

func ps6118IntegerConstant(pass *analysis.Pass, expression ast.Expr, expected int64) bool {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	if value == nil || value.Kind() != constant.Int {
		return false
	}
	integer, exact := constant.Int64Val(value)
	return exact && integer == expected
}

func ps6118ObjectUsed(pass *analysis.Pass, node ast.Node, object types.Object) bool {
	used := false
	ast.Inspect(node, func(current ast.Node) bool {
		identifier, ok := current.(*ast.Ident)
		if ok && pass.TypesInfo.ObjectOf(identifier) == object {
			used = true
			return false
		}
		return !used
	})
	return used
}

func ps6118Loop(pass *analysis.Pass, loop *ast.ForStmt, contract *config.CrossStepAcceleratorResidencyContract) (ps6118Match, bool) {
	if len(loop.Body.List) != 4 || ps6118CallCount(loop.Body) != 3 {
		return ps6118Match{}, false
	}
	assignment, objective, ok := ps6118ObjectiveStatement(pass, loop.Body.List[0], contract)
	if !ok {
		return ps6118Match{}, false
	}
	objectiveReceiver := ps6118Receiver(pass, objective)
	scalar, gradients, objectiveError, ok := ps6118ObjectiveBindings(pass, assignment, objective, contract)
	if objectiveReceiver == nil || !ok || !ps6118ReturnedErrorGuard(pass, loop.Body.List[1], objectiveError, nil) {
		return ps6118Match{}, false
	}
	optimizer, optimizerReceiver, callback, optimizerError, ok := ps6118OptimizerGuard(pass, loop.Body.List[2], contract)
	if !ok || optimizerReceiver == nil || optimizerReceiver == objectiveReceiver || !ps6118GradientCallback(pass, callback, gradients) {
		return ps6118Match{}, false
	}
	observer, ok := ps6118ObserverStatement(pass, loop.Body.List[3], contract)
	if !ok || !ps6118ObserverArgument(pass, observer, scalar, contract) ||
		!ps6118ScalarLikeResult(pass, objective, contract.ObjectiveScalarResult) ||
		!ps6118CollectionResult(pass.TypesInfo.TypeOf(objective), contract.ObjectiveGradientResult) {
		return ps6118Match{}, false
	}
	allowed := map[types.Object]map[*ast.Ident]bool{
		objectiveReceiver: ps6118IdentifiersIn(pass, ps6118ReceiverExpression(objective)),
		optimizerReceiver: ps6118IdentifiersIn(pass, ps6118ReceiverExpression(optimizer)),
		gradients:         ps6118IdentifiersInNode(pass, callback),
		scalar:            ps6118IdentifiersIn(pass, observer.Args[contract.ScalarObserverArgument-1]),
		objectiveError:    ps6118IdentifiersInNode(pass, loop.Body.List[1]),
		optimizerError:    ps6118IdentifiersInNode(pass, loop.Body.List[2]),
	}
	if ps6118UnsafeRoleUses(pass, loop.Body, allowed) {
		return ps6118Match{}, false
	}
	return ps6118Match{objective: objective, optimizer: optimizer, observer: observer}, true
}

func ps6118ObjectiveStatement(pass *analysis.Pass, statement ast.Stmt, contract *config.CrossStepAcceleratorResidencyContract) (*ast.AssignStmt, *ast.CallExpr, bool) {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 3 || len(assignment.Rhs) != 1 {
		return nil, nil, false
	}
	call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	return assignment, call, ok && ps6115DirectCall(pass, nil, call, contract.ObjectiveCallable)
}

func ps6118CallCount(body *ast.BlockStmt) int {
	count := 0
	ast.Inspect(body, func(node ast.Node) bool {
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		if _, ok := node.(*ast.CallExpr); ok {
			count++
		}
		return true
	})
	return count
}

func ps6118Receiver(pass *analysis.Pass, call *ast.CallExpr) types.Object {
	expression := ps6118ReceiverExpression(call)
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok || identifier.Name == "_" {
		return nil
	}
	return pass.TypesInfo.ObjectOf(identifier)
}

func ps6118ReceiverExpression(call *ast.CallExpr) ast.Expr {
	selector, _ := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return nil
	}
	return selector.X
}

func ps6118ObjectiveBindings(pass *analysis.Pass, assignment *ast.AssignStmt, call *ast.CallExpr, contract *config.CrossStepAcceleratorResidencyContract) (types.Object, types.Object, types.Object, bool) {
	scalarID, scalarOK := ps2110Unparen(assignment.Lhs[contract.ObjectiveScalarResult-1]).(*ast.Ident)
	gradientID, gradientOK := ps2110Unparen(assignment.Lhs[contract.ObjectiveGradientResult-1]).(*ast.Ident)
	errorID, errorOK := ps2110Unparen(assignment.Lhs[contract.ObjectiveErrorResult-1]).(*ast.Ident)
	if !scalarOK || !gradientOK || !errorOK || scalarID.Name == "_" || gradientID.Name == "_" || errorID.Name == "_" ||
		!ps6118ErrorResult(pass, call, contract.ObjectiveErrorResult) {
		return nil, nil, nil, false
	}
	scalar := pass.TypesInfo.Defs[scalarID]
	gradients := pass.TypesInfo.Defs[gradientID]
	err := pass.TypesInfo.Defs[errorID]
	if scalar == nil || gradients == nil || err == nil || scalar == gradients || scalar == err || gradients == err {
		return nil, nil, nil, false
	}
	return scalar, gradients, err, true
}

func ps6118OptimizerGuard(pass *analysis.Pass, statement ast.Stmt, contract *config.CrossStepAcceleratorResidencyContract) (*ast.CallExpr, types.Object, *ast.FuncLit, types.Object, bool) {
	guard, ok := statement.(*ast.IfStmt)
	if !ok || guard.Else != nil || len(guard.Body.List) != 1 {
		return nil, nil, nil, nil, false
	}
	assignment, ok := guard.Init.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return nil, nil, nil, nil, false
	}
	errorID, errorOK := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	call, callOK := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if !errorOK || !callOK || errorID.Name == "_" || !ps6115DirectCall(pass, nil, call, contract.OptimizerCallable) ||
		!ps6118ErrorResult(pass, call, contract.OptimizerErrorResult) || len(call.Args) != 1 {
		return nil, nil, nil, nil, false
	}
	callback := ps6118CallbackArgument(call, contract.OptimizerGradientCallbackArgument)
	errorObject := pass.TypesInfo.Defs[errorID]
	if callback == nil || errorObject == nil || !ps6118ReturnedErrorGuard(pass, guard, errorObject, assignment) {
		return nil, nil, nil, nil, false
	}
	return call, ps6118Receiver(pass, call), callback, errorObject, true
}

func ps6118CallbackArgument(call *ast.CallExpr, position int) *ast.FuncLit {
	if position <= 0 || position > len(call.Args) {
		return nil
	}
	callback, _ := ps2110Unparen(call.Args[position-1]).(*ast.FuncLit)
	return callback
}

func ps6118ReturnedErrorGuard(pass *analysis.Pass, statement ast.Stmt, errorObject types.Object, expectedInit ast.Stmt) bool {
	guard, ok := statement.(*ast.IfStmt)
	if !ok || guard.Init != expectedInit || guard.Else != nil || len(guard.Body.List) != 1 {
		return false
	}
	condition, ok := ps2110Unparen(guard.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op != token.NEQ || ps6114ExprObject(pass, condition.X) != errorObject || !ps6118Nil(pass, condition.Y) {
		return false
	}
	returned, ok := guard.Body.List[0].(*ast.ReturnStmt)
	return ok && len(returned.Results) == 1 && ps6114ExprObject(pass, returned.Results[0]) == errorObject
}

func ps6118Nil(pass *analysis.Pass, expression ast.Expr) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && identifier.Name == "nil" && pass.TypesInfo.ObjectOf(identifier) == types.Universe.Lookup("nil")
}

func ps6118GradientCallback(pass *analysis.Pass, callback *ast.FuncLit, gradients types.Object) bool {
	if callback == nil || callback.Type.Params == nil || callback.Type.Results == nil || callback.Type.Params.NumFields() != 1 ||
		callback.Type.Results.NumFields() != 1 || len(callback.Body.List) != 1 {
		return false
	}
	parameterField := callback.Type.Params.List[0]
	if len(parameterField.Names) != 1 || parameterField.Names[0].Name == "_" {
		return false
	}
	parameter := pass.TypesInfo.Defs[parameterField.Names[0]]
	returned, ok := callback.Body.List[0].(*ast.ReturnStmt)
	if parameter == nil || !ok || len(returned.Results) != 1 {
		return false
	}
	gradientIndex, ok := ps2110Unparen(returned.Results[0]).(*ast.IndexExpr)
	if !ok || ps6114ExprObject(pass, gradientIndex.X) != gradients {
		return false
	}
	parameterIndex, ok := ps2110Unparen(gradientIndex.Index).(*ast.IndexExpr)
	return ok && ps6114ExprObject(pass, parameterIndex.X) != nil && ps6114ExprObject(pass, parameterIndex.Index) == parameter
}

func ps6118ObserverStatement(pass *analysis.Pass, statement ast.Stmt, contract *config.CrossStepAcceleratorResidencyContract) (*ast.CallExpr, bool) {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return nil, false
	}
	call, ok := ps2110Unparen(expression.X).(*ast.CallExpr)
	return call, ok && ps6115DirectCall(pass, nil, call, contract.ScalarObserverCallable)
}

func ps6118ObserverArgument(pass *analysis.Pass, call *ast.CallExpr, scalar types.Object, contract *config.CrossStepAcceleratorResidencyContract) bool {
	index := contract.ScalarObserverArgument - 1
	return len(call.Args) == 1 && index == 0 && ps6114ExprObject(pass, call.Args[index]) == scalar
}

func ps6118CollectionResult(value types.Type, position int) bool {
	tuple, ok := value.(*types.Tuple)
	if !ok || position <= 0 || position > tuple.Len() {
		return false
	}
	switch types.Unalias(tuple.At(position - 1).Type()).Underlying().(type) {
	case *types.Slice, *types.Array:
		return true
	}
	return false
}

func ps6118ScalarLikeResult(pass *analysis.Pass, call *ast.CallExpr, position int) bool {
	value := ps6118ResultType(pass, call, position)
	if value == nil {
		return false
	}
	if basic, ok := types.Unalias(value).Underlying().(*types.Basic); ok {
		return basic.Info()&types.IsNumeric != 0
	}
	pointer, ok := types.Unalias(value).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(pointer.Elem()).(*types.Named)
	if !ok {
		return false
	}
	_, structure := named.Underlying().(*types.Struct)
	return structure
}

func ps6118ErrorResult(pass *analysis.Pass, call *ast.CallExpr, position int) bool {
	return ps6091ErrorType(ps6118ResultType(pass, call, position))
}

func ps6118ResultType(pass *analysis.Pass, call *ast.CallExpr, position int) types.Type {
	_, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature == nil || position <= 0 || position > signature.Results().Len() {
		return nil
	}
	return signature.Results().At(position - 1).Type()
}

func ps6118IdentifiersIn(pass *analysis.Pass, expressions ...ast.Expr) map[*ast.Ident]bool {
	result := make(map[*ast.Ident]bool)
	for _, expression := range expressions {
		for identifier := range ps6118IdentifiersInNode(pass, expression) {
			result[identifier] = true
		}
	}
	return result
}

func ps6118IdentifiersInNode(pass *analysis.Pass, node ast.Node) map[*ast.Ident]bool {
	result := make(map[*ast.Ident]bool)
	ast.Inspect(node, func(current ast.Node) bool {
		identifier, ok := current.(*ast.Ident)
		if ok && pass.TypesInfo.ObjectOf(identifier) != nil {
			result[identifier] = true
		}
		return true
	})
	return result
}

func ps6118UnsafeRoleUses(pass *analysis.Pass, body *ast.BlockStmt, allowed map[types.Object]map[*ast.Ident]bool) bool {
	unsafe := false
	ast.Inspect(body, func(node ast.Node) bool {
		if unsafe {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		permitted, tracked := allowed[object]
		if tracked && !permitted[identifier] && pass.TypesInfo.Defs[identifier] == nil {
			unsafe = true
			return false
		}
		return true
	})
	return unsafe
}

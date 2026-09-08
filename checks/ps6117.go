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

// PS6117 implements owner issue #878. Unlike PS6116's exact
// forward-loss-backward chain, this check requires a configured inventory of
// many synchronized eager accelerator calls across a complete objective.
var PS6117 = register(&lint.Check{
	ID:          "PS6117",
	Category:    "verify",
	Slug:        "fragmented-accelerator-objective",
	Level:       lint.LevelAggressive,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"fragmentedAcceleratorObjectiveContracts"},
	Doc: lint.Documentation{
		Title: "a complete accelerator objective is fragmented by repeated host synchronization",
		Text: `A training objective may route every primitive through an accelerator
and still spend most of its time crossing the host boundary. Repeated eager
submission and blocking completion between embeddings, model layers, loss, and
reverse mode prevent the backend from retaining the complete dependency graph.

PS6117 is deliberately broader than an exact forward -> loss -> backward call
chain and correspondingly more explicit. A unique
fragmentedAcceleratorObjectiveContracts entry names the complete objective and
every exact direct accelerator callable/operation identity in every configured
site it reaches. Occurrence counts, the total accelerator-call count, and the
number of synchronizing boundaries are exact. At least four calls and two
distinct records are required; a partial inventory stays silent.

The owner contract must say that the boundary is complete forward, mean loss,
and reverse mode; that its geometry is stable and fully represented by a
reusable graph-cache key; that every current call submits and synchronizes; and
that a candidate would submit once. It identifies exactly three public results:
the scalar objective, all parameter gradients, and an error. Exact dtype,
layout, attributes, reduction, floating-point, error, panic, recorder,
autograd, and backend-selection behavior remain validation obligations.

Two unresolved owner-review hazards are separate mandatory gates. A candidate
must preserve every per-operation backend route, including low-memory spill or
override routes, rather than bypass them through a whole-objective capability.
It must also implement true causal-mask semantics: a finite sentinel such as
-1e30 is not exact exclusion because sufficiently large finite logits can leak
probability to future tokens. Use negative infinity or a true masked-softmax
construction and validate the scalar and every gradient over the supported
finite-input domain.

Source matching fails closed. Every configured function and callable must
resolve concretely and non-generically, every configured site must be reached
from the objective exactly once by direct package calls, and every expected
call must be on one unconditional, source-proven reachable execution path. The
result of every configured accelerator call must flow into the configured
scalar or gradient result; discarded work and unrelated returned values stay
silent. A selected struct field or container index also stays silent when the
flat source model cannot distinguish it from accelerator-dependent aggregate
members that were discarded. Interface or function-value dispatch, branches,
loops, go/defer,
closures, variadics, explicit generic instantiation, custom hook/callback
registration, post-definition mutation, transfer or device/graph/recorder/
stream/command-buffer context, and fused objective calls suppress the finding.
So do configuration flags recording an existing whole-objective route or an
intentionally retained fragmented route.

There is NO automatic fix and the diagnostic does not claim a win. It asks for
an exact one-submission candidate plus paired, same-binary application
benchmarks and numerical comparison of the scalar and every gradient. Graph
construction, cache misses, dense gradient materialization, and device
residency can reverse a micro-level expectation.

Embedding-gradient lowering is explicitly shape-sensitive. The accepted GoAI
campaign found dense one-hot transpose GEMM faster than MPSGraph scatterND at
its pinned geometry. Never recommend scatterND from repeated indices alone;
benchmark the exact vocabulary, sequence, width, duplicate-index distribution,
dtype, layout, and hardware while checking repeated-index accumulation parity.`,
		Before: `loss := accelerator.ForwardAndLoss(inputs, parameters)
gradients := accelerator.Reverse(loss, parameters)
// Internally: many eager submissions, each followed by blocking completion.
return loss, gradients, nil`,
		After: `// Candidate only: one geometry-keyed complete-objective graph.
// Keep the fragmented fallback and promote only after scalar+all-gradient
// numerical parity and paired application benchmarks pass.`,
		MeasuredWin: `GoAI merge ba513def (PR #1201), backed by
fab7e6e5 and evidence m2-metal-gpt-loss-grad-20260824, replaced a portable
per-operation Metal tape with one cached causal MPSGraph for vocab/context/
sequence 4096/256/256, width 512, eight heads, FFN 2048, depth six, batch one,
contiguous F32 mean cross-entropy and all 77 parameter gradients. All 21 aligned
M2 Pro samples improved: median 3.689x, minimum 2.822x, candidate median 20.748
ms/objective (12,338 tok/s) versus 76.238 ms control. The candidate-only screen
measured 245 allocations/objective versus about 1,650 on the previous public
path. This is shape-specific evidence, not a promised win. A scatterND embedding
gradient prototype lost at roughly 26.3-31.6 ms versus 24.7-25.6 ms for dense
one-hot transpose GEMM, so scatter lowering remains a shape-aware benchmark
question. The merged evidence still has unresolved PR #1201 review findings:
discussion_r3840936867 identifies bypassed per-operation backend routes, and
discussion_r3840936874 identifies finite -1e30 causal-mask leakage. Both are
mandatory contract and validation gates, not evidence that the candidate is
already safe to promote.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6117",
		Doc:  "configured complete accelerator objectives fragmented by repeated synchronous boundaries",
		Run:  runPS6117,
	},
})

type ps6117RecordMatch struct {
	record *config.FragmentedAcceleratorObjectiveCall
	calls  []*ast.CallExpr
}

func runPS6117(pass *analysis.Pass) (any, error) {
	return runPS6117WithContracts(pass, config.Current().FragmentedAcceleratorObjectiveContracts)
}

func runPS6117WithContracts(pass *analysis.Pass, configured []config.FragmentedAcceleratorObjectiveContract) (any, error) {
	declarations := ps6115Declarations(pass)
	for _, contract := range ps6117Contracts(configured) {
		if contract.ExistingWholeObjectiveRoute || contract.IntentionalRetainedFragmentedRoute {
			continue
		}
		objective := declarations[contract.ObjectiveSite]
		if objective == nil || !ps6117ObjectiveResults(pass, objective, contract) {
			continue
		}
		sites := ps6117ContractSites(pass, contract, declarations)
		if sites == nil || !ps6117SitesReachable(pass, objective, sites) || ps6117UnsafeSites(pass, sites, contract) {
			continue
		}
		matches, ok := ps6117MatchRecords(pass, contract, sites)
		if !ok || !ps6117SingleInvocationGraph(pass, objective, sites) ||
			!ps6117ObjectiveFlow(pass, objective, sites, matches, contract) {
			continue
		}
		related := make([]analysis.RelatedInformation, 0, contract.ConfiguredAcceleratorCallCount)
		for _, match := range matches {
			for _, call := range match.calls {
				related = append(related, analysis.RelatedInformation{Pos: call.Pos(), End: call.End(), Message: "configured synchronized accelerator call " + ps6117RecordLabel(match.record)})
			}
		}
		slices.SortFunc(related, func(left, right analysis.RelatedInformation) int { return int(left.Pos - right.Pos) })
		pass.Report(analysis.Diagnostic{
			Pos: objective.Name.Pos(), End: objective.Name.End(),
			Message: contract.Name + ": complete scalar-objective plus parameter-gradient boundary reaches exactly " + strconv.Itoa(contract.ConfiguredAcceleratorCallCount) + " configured accelerator calls and " + strconv.Itoa(contract.ConfiguredSynchronousBoundaryCount) + " blocking boundaries with stable cache geometry " + strconv.Quote(contract.GeometryCacheKey) + "; screen a one-submission whole-objective graph with scalar-and-every-gradient numerical parity, per-operation backend-route preservation, true causal-mask exclusion, and paired same-binary application benchmarks—this is not a performance win or lowering recommendation, and scatterND versus one-hot embedding gradients requires shape-aware validation (PS6117 advisory, no automatic fix)",
			Related: related,
		})
	}
	return nil, nil
}

func ps6117Contracts(configured []config.FragmentedAcceleratorObjectiveContract) []*config.FragmentedAcceleratorObjectiveContract {
	nameCount := make(map[string]int)
	objectiveCount := make(map[string]int)
	claimCount := make(map[string]int)
	for index := range configured {
		contract := &configured[index]
		if !contract.Valid() {
			continue
		}
		nameCount[contract.Name]++
		objectiveCount[contract.ObjectiveSite]++
		for callIndex := range contract.Calls {
			claimCount[ps6117RecordKey(&contract.Calls[callIndex])]++
		}
	}
	var result []*config.FragmentedAcceleratorObjectiveContract
	for index := range configured {
		contract := &configured[index]
		if !contract.Valid() || nameCount[contract.Name] != 1 || objectiveCount[contract.ObjectiveSite] != 1 {
			continue
		}
		unique := true
		for callIndex := range contract.Calls {
			unique = unique && claimCount[ps6117RecordKey(&contract.Calls[callIndex])] == 1
		}
		if unique {
			result = append(result, contract)
		}
	}
	slices.SortFunc(result, func(left, right *config.FragmentedAcceleratorObjectiveContract) int {
		return strings.Compare(left.Name, right.Name)
	})
	return result
}

func ps6117RecordKey(record *config.FragmentedAcceleratorObjectiveCall) string {
	return record.ConfiguredSite + "\x00" + record.AcceleratorCallable + "\x00" + record.OperationConstant
}

func ps6117RecordLabel(record *config.FragmentedAcceleratorObjectiveCall) string {
	if record.OperationConstant == "" {
		return record.AcceleratorCallable
	}
	return record.AcceleratorCallable + "(" + record.OperationConstant + ")"
}

func ps6117ContractSites(pass *analysis.Pass, contract *config.FragmentedAcceleratorObjectiveContract, declarations map[string]*ast.FuncDecl) map[string]*ast.FuncDecl {
	sites := make(map[string]*ast.FuncDecl)
	sites[contract.ObjectiveSite] = declarations[contract.ObjectiveSite]
	for _, record := range contract.Calls {
		declaration := declarations[record.ConfiguredSite]
		if declaration == nil || ps6113GenericFunction(pass, declaration) {
			return nil
		}
		sites[record.ConfiguredSite] = declaration
	}
	return sites
}

func ps6117ObjectiveResults(pass *analysis.Pass, declaration *ast.FuncDecl, contract *config.FragmentedAcceleratorObjectiveContract) bool {
	function, _ := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	if function == nil || ps6113GenericFunction(pass, declaration) {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Results().Len() != 3 {
		return false
	}
	positions := []int{contract.ScalarObjectiveResult, contract.ParameterGradientsResult, contract.ErrorResult}
	for _, position := range positions {
		if position < 1 || position > signature.Results().Len() {
			return false
		}
	}
	gradient := types.Unalias(signature.Results().At(contract.ParameterGradientsResult - 1).Type()).Underlying()
	if _, ok := gradient.(*types.Slice); !ok {
		return false
	}
	errorType := signature.Results().At(contract.ErrorResult - 1).Type()
	if !types.Identical(errorType, types.Universe.Lookup("error").Type()) {
		return false
	}
	scalar := types.Unalias(signature.Results().At(contract.ScalarObjectiveResult - 1).Type()).Underlying()
	switch scalar.(type) {
	case *types.Slice, *types.Array, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		return false
	}
	return true
}

func ps6117SitesReachable(pass *analysis.Pass, objective *ast.FuncDecl, sites map[string]*ast.FuncDecl) bool {
	reached := map[*ast.FuncDecl]bool{objective: true}
	queue := []*ast.FuncDecl{objective}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		unreachable := ps2144Unreachable(pass, current.Body)
		ast.Inspect(current.Body, func(node ast.Node) bool {
			if _, closure := node.(*ast.FuncLit); closure {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok || ps2144PositionIn(call.Pos(), unreachable) {
				return true
			}
			declaration := sites[ps6087FunctionID(pass, call)]
			if declaration != nil && !reached[declaration] {
				reached[declaration] = true
				queue = append(queue, declaration)
			}
			return true
		})
	}
	for _, site := range sites {
		if !reached[site] {
			return false
		}
	}
	return true
}

func ps6117UnsafeSites(pass *analysis.Pass, sites map[string]*ast.FuncDecl, contract *config.FragmentedAcceleratorObjectiveContract) bool {
	allowed := make(map[string]bool, len(sites)+len(contract.Calls))
	for site := range sites {
		allowed[site] = true
	}
	for _, record := range contract.Calls {
		allowed[record.AcceleratorCallable] = true
	}
	for _, declaration := range sites {
		if ps6115DeclarationContext(pass, declaration) {
			return true
		}
		unreachable := ps2144Unreachable(pass, declaration.Body)
		unsafe := false
		ast.Inspect(declaration.Body, func(node ast.Node) bool {
			if unsafe || node == nil {
				return !unsafe
			}
			if ps2144PositionIn(node.Pos(), unreachable) {
				return false
			}
			switch value := node.(type) {
			case *ast.FuncLit, *ast.GoStmt, *ast.DeferStmt, *ast.IncDecStmt,
				*ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt,
				*ast.TypeSwitchStmt, *ast.SelectStmt, *ast.BranchStmt:
				unsafe = true
				return false
			case *ast.BinaryExpr:
				if value.Op.String() == "&&" || value.Op.String() == "||" {
					unsafe = true
					return false
				}
			case *ast.AssignStmt:
				if value.Tok.String() != ":=" && value.Tok.String() != "=" ||
					value.Tok.String() == "=" && ps6117BoundaryMutation(pass, declaration, value.Lhs) {
					unsafe = true
					return false
				}
			case *ast.CallExpr:
				if ps6117DynamicCall(pass, value) {
					unsafe = true
					return false
				}
				id := strings.ToLower(ps6087FunctionID(pass, value))
				if !allowed[ps6087FunctionID(pass, value)] && !ps6117IntrinsicCall(pass, value) ||
					ps6115TransferOrContextName(id) || ps6117CustomHook(id) || ps6117ExistingObjectiveRoute(id) {
					unsafe = true
					return false
				}
				for _, argument := range value.Args {
					if ps6115ExplicitDeviceType(pass.TypesInfo.TypeOf(argument)) {
						unsafe = true
						return false
					}
				}
			}
			return true
		})
		if unsafe {
			return true
		}
	}
	return false
}

func ps6117IntrinsicCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	var object types.Object
	switch function := ps2110Unparen(call.Fun).(type) {
	case *ast.Ident:
		object = pass.TypesInfo.ObjectOf(function)
	case *ast.SelectorExpr:
		object = pass.TypesInfo.ObjectOf(function.Sel)
	}
	switch object.(type) {
	case *types.Builtin, *types.TypeName:
		return true
	}
	return false
}

func ps6117DynamicCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	if call == nil || call.Ellipsis.IsValid() || ps6115ExplicitInstantiation(call.Fun) {
		return true
	}
	switch function := ps2110Unparen(call.Fun).(type) {
	case *ast.Ident:
		if _, variable := pass.TypesInfo.ObjectOf(function).(*types.Var); variable {
			_, callable := types.Unalias(pass.TypesInfo.TypeOf(function)).Underlying().(*types.Signature)
			return callable
		}
	case *ast.SelectorExpr:
		selection := pass.TypesInfo.Selections[function]
		if selection != nil {
			_, dynamic := types.Unalias(selection.Recv()).Underlying().(*types.Interface)
			return dynamic || selection.Kind() != types.MethodVal || len(selection.Index()) != 1
		}
	}
	return false
}

func ps6117CustomHook(id string) bool {
	return ps6007ContainsAny(id, "hook", "callback")
}

func ps6117BoundaryMutation(pass *analysis.Pass, declaration *ast.FuncDecl, expressions []ast.Expr) bool {
	for _, expression := range expressions {
		switch target := ps2110Unparen(expression).(type) {
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(target)
			if object != nil && object.Pos() < declaration.Body.Pos() {
				return true
			}
		case *ast.SelectorExpr, *ast.IndexExpr, *ast.IndexListExpr, *ast.StarExpr:
			return true
		default:
			return true
		}
	}
	return false
}

func ps6117ExistingObjectiveRoute(id string) bool {
	return ps6007ContainsAny(id, "objectivegraph", "lossandgrad", "loss_and_grad", "forwardlossbackward")
}

func ps6117MatchRecords(pass *analysis.Pass, contract *config.FragmentedAcceleratorObjectiveContract, sites map[string]*ast.FuncDecl) ([]ps6117RecordMatch, bool) {
	allConfiguredCallables := make(map[string]bool, len(contract.Calls))
	for index := range contract.Calls {
		record := &contract.Calls[index]
		if !ps6115CallableResolves(pass, record.AcceleratorCallable) {
			return nil, false
		}
		allConfiguredCallables[record.AcceleratorCallable] = true
	}
	results := make([]ps6117RecordMatch, 0, len(contract.Calls))
	totalMatched := 0
	for index := range contract.Calls {
		record := &contract.Calls[index]
		declaration := sites[record.ConfiguredSite]
		file := ps6115FileContaining(pass, declaration)
		unreachable := ps2144Unreachable(pass, declaration.Body)
		var calls []*ast.CallExpr
		ast.Inspect(declaration.Body, func(node ast.Node) bool {
			if _, closure := node.(*ast.FuncLit); closure {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok || ps2144PositionIn(call.Pos(), unreachable) {
				return true
			}
			if ps6115DirectCall(pass, file, call, record.AcceleratorCallable) && ps6117Operation(pass, call, record) {
				calls = append(calls, call)
			}
			return true
		})
		if len(calls) != record.ExpectedOccurrences {
			return nil, false
		}
		totalMatched += len(calls)
		results = append(results, ps6117RecordMatch{record: record, calls: calls})
	}
	totalConfiguredCallableCalls := 0
	for _, declaration := range sites {
		unreachable := ps2144Unreachable(pass, declaration.Body)
		ast.Inspect(declaration.Body, func(node ast.Node) bool {
			if _, closure := node.(*ast.FuncLit); closure {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if ok && !ps2144PositionIn(call.Pos(), unreachable) && allConfiguredCallables[ps6087FunctionID(pass, call)] {
				totalConfiguredCallableCalls++
			}
			return true
		})
	}
	return results, totalMatched == contract.ConfiguredAcceleratorCallCount && totalConfiguredCallableCalls == totalMatched
}

func ps6117Operation(pass *analysis.Pass, call *ast.CallExpr, record *config.FragmentedAcceleratorObjectiveCall) bool {
	if record.OperationConstant == "" {
		return true
	}
	index := record.OperationArgument - 1
	if index < 0 || index >= len(call.Args) {
		return false
	}
	expectedType := pass.TypesInfo.TypeOf(call.Args[index])
	if function, signature, ok := typedCallee(pass, call.Fun); ok && function != nil && signature != nil && index < signature.Params().Len() {
		expectedType = signature.Params().At(index).Type()
	}
	return expectedType != nil && ps6113AddConstant(pass, call.Args[index], record.OperationConstant, record.OperationConstantValue, expectedType)
}

type ps6117FlowSet map[*ast.CallExpr]bool

type ps6117Flow struct {
	pass         *analysis.Pass
	sites        map[string]*ast.FuncDecl
	accelerators map[*ast.CallExpr]bool
	stack        map[*ast.FuncDecl]bool
}

func ps6117SingleInvocationGraph(pass *analysis.Pass, objective *ast.FuncDecl, sites map[string]*ast.FuncDecl) bool {
	incoming := make(map[*ast.FuncDecl]int, len(sites))
	for _, declaration := range sites {
		unreachable := ps2144Unreachable(pass, declaration.Body)
		ast.Inspect(declaration.Body, func(node ast.Node) bool {
			if _, closure := node.(*ast.FuncLit); closure {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok || ps2144PositionIn(call.Pos(), unreachable) {
				return true
			}
			if callee := sites[ps6087FunctionID(pass, call)]; callee != nil {
				incoming[callee]++
			}
			return true
		})
	}
	if incoming[objective] != 0 {
		return false
	}
	for _, declaration := range sites {
		if declaration != objective && incoming[declaration] != 1 {
			return false
		}
		function, _ := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
		if function == nil {
			return false
		}
		signature, _ := function.Type().(*types.Signature)
		if signature == nil || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 {
			return false
		}
	}
	return true
}

func ps6117ObjectiveFlow(pass *analysis.Pass, objective *ast.FuncDecl, sites map[string]*ast.FuncDecl,
	matches []ps6117RecordMatch, contract *config.FragmentedAcceleratorObjectiveContract,
) bool {
	accelerators := make(map[*ast.CallExpr]bool, contract.ConfiguredAcceleratorCallCount)
	for _, match := range matches {
		for _, call := range match.calls {
			accelerators[call] = true
		}
	}
	flow := &ps6117Flow{pass: pass, sites: sites, accelerators: accelerators, stack: make(map[*ast.FuncDecl]bool)}
	results, ok := flow.function(objective, nil)
	if !ok || len(results) != 3 {
		return false
	}
	scalar := results[contract.ScalarObjectiveResult-1]
	gradients := results[contract.ParameterGradientsResult-1]
	if len(scalar) == 0 || len(gradients) == 0 {
		return false
	}
	outputs := ps6117MergeFlow(scalar, gradients)
	if len(outputs) != len(accelerators) {
		return false
	}
	for call := range accelerators {
		if !outputs[call] {
			return false
		}
	}
	return true
}

func (flow *ps6117Flow) function(declaration *ast.FuncDecl, inputs map[types.Object]ps6117FlowSet) ([]ps6117FlowSet, bool) {
	if declaration == nil || declaration.Body == nil || flow.stack[declaration] || len(declaration.Body.List) == 0 {
		return nil, false
	}
	flow.stack[declaration] = true
	defer delete(flow.stack, declaration)
	environment := make(map[types.Object]ps6117FlowSet, len(inputs))
	for object, dependencies := range inputs {
		environment[object] = ps6117CloneFlow(dependencies)
	}
	for index, statement := range declaration.Body.List {
		last := index == len(declaration.Body.List)-1
		switch value := statement.(type) {
		case *ast.AssignStmt:
			if value.Tok.String() != ":=" && value.Tok.String() != "=" || !flow.assignment(value.Lhs, value.Rhs, environment) {
				return nil, false
			}
		case *ast.DeclStmt:
			declaration, ok := value.Decl.(*ast.GenDecl)
			if !ok {
				return nil, false
			}
			for _, specification := range declaration.Specs {
				values, ok := specification.(*ast.ValueSpec)
				if !ok || !flow.valueSpec(values, environment) {
					return nil, false
				}
			}
		case *ast.ExprStmt:
			if _, ok := flow.expression(value.X, environment); !ok {
				return nil, false
			}
		case *ast.ReturnStmt:
			if !last {
				return nil, false
			}
			return flow.expressions(value.Results, environment)
		case *ast.EmptyStmt:
		default:
			return nil, false
		}
	}
	return nil, false
}

func (flow *ps6117Flow) assignment(left, right []ast.Expr, environment map[types.Object]ps6117FlowSet) bool {
	dependencies, ok := flow.expressions(right, environment)
	if !ok || len(dependencies) != len(left) {
		return false
	}
	for index, expression := range left {
		identifier, ok := ps2110Unparen(expression).(*ast.Ident)
		if !ok {
			return false
		}
		if identifier.Name == "_" {
			continue
		}
		object := flow.pass.TypesInfo.ObjectOf(identifier)
		if object == nil {
			return false
		}
		environment[object] = ps6117CloneFlow(dependencies[index])
	}
	return true
}

func (flow *ps6117Flow) valueSpec(specification *ast.ValueSpec, environment map[types.Object]ps6117FlowSet) bool {
	if len(specification.Values) == 0 {
		for _, name := range specification.Names {
			environment[flow.pass.TypesInfo.ObjectOf(name)] = nil
		}
		return true
	}
	left := make([]ast.Expr, len(specification.Names))
	for index := range specification.Names {
		left[index] = specification.Names[index]
	}
	return flow.assignment(left, specification.Values, environment)
}

func (flow *ps6117Flow) expressions(expressions []ast.Expr, environment map[types.Object]ps6117FlowSet) ([]ps6117FlowSet, bool) {
	var result []ps6117FlowSet
	for _, expression := range expressions {
		dependencies, ok := flow.expression(expression, environment)
		if !ok {
			return nil, false
		}
		result = append(result, dependencies...)
	}
	return result, true
}

func (flow *ps6117Flow) expression(expression ast.Expr, environment map[types.Object]ps6117FlowSet) ([]ps6117FlowSet, bool) {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		if dependencies, ok := environment[flow.pass.TypesInfo.ObjectOf(value)]; ok {
			return []ps6117FlowSet{ps6117CloneFlow(dependencies)}, true
		}
		return []ps6117FlowSet{nil}, true
	case *ast.BasicLit:
		return []ps6117FlowSet{nil}, true
	case *ast.CallExpr:
		return flow.call(value, environment)
	case *ast.CompositeLit:
		dependencies := make(ps6117FlowSet)
		for _, element := range value.Elts {
			parts, ok := flow.expression(element, environment)
			if !ok {
				return nil, false
			}
			for _, part := range parts {
				ps6117AddFlow(dependencies, part)
			}
		}
		return []ps6117FlowSet{dependencies}, true
	case *ast.KeyValueExpr:
		left, leftOK := flow.expression(value.Key, environment)
		right, rightOK := flow.expression(value.Value, environment)
		if !leftOK || !rightOK {
			return nil, false
		}
		return []ps6117FlowSet{ps6117MergeFlow(ps6117FlattenFlow(left), ps6117FlattenFlow(right))}, true
	case *ast.SelectorExpr:
		parts, ok := flow.expression(value.X, environment)
		if !ok || len(ps6117FlattenFlow(parts)) != 0 {
			// The flat flow model cannot distinguish a selected field from
			// accelerator-dependent aggregate members that are discarded.
			return nil, false
		}
		return []ps6117FlowSet{nil}, true
	case *ast.IndexExpr:
		left, leftOK := flow.expression(value.X, environment)
		right, rightOK := flow.expression(value.Index, environment)
		if !leftOK || !rightOK || len(ps6117FlattenFlow(left)) != 0 || len(ps6117FlattenFlow(right)) != 0 {
			// A flat aggregate dependency cannot prove that this exact index,
			// rather than a discarded element, carries the matched result.
			return nil, false
		}
		return []ps6117FlowSet{nil}, true
	case *ast.SliceExpr:
		parts, ok := flow.expression(value.X, environment)
		if !ok {
			return nil, false
		}
		dependencies := ps6117FlattenFlow(parts)
		for _, bound := range []ast.Expr{value.Low, value.High, value.Max} {
			if bound == nil {
				continue
			}
			boundParts, boundOK := flow.expression(bound, environment)
			if !boundOK {
				return nil, false
			}
			ps6117AddFlow(dependencies, ps6117FlattenFlow(boundParts))
		}
		return []ps6117FlowSet{dependencies}, true
	case *ast.UnaryExpr:
		return flow.expression(value.X, environment)
	case *ast.BinaryExpr:
		left, leftOK := flow.expression(value.X, environment)
		right, rightOK := flow.expression(value.Y, environment)
		if !leftOK || !rightOK {
			return nil, false
		}
		return []ps6117FlowSet{ps6117MergeFlow(ps6117FlattenFlow(left), ps6117FlattenFlow(right))}, true
	case *ast.TypeAssertExpr:
		return flow.expression(value.X, environment)
	default:
		return nil, false
	}
}

func (flow *ps6117Flow) call(call *ast.CallExpr, environment map[types.Object]ps6117FlowSet) ([]ps6117FlowSet, bool) {
	dependencies := make(ps6117FlowSet)
	var receiverDependencies ps6117FlowSet
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
		parts, partsOK := flow.expression(selector.X, environment)
		if !partsOK {
			return nil, false
		}
		receiverDependencies = ps6117FlattenFlow(parts)
		ps6117AddFlow(dependencies, receiverDependencies)
	}
	arguments := make([]ps6117FlowSet, len(call.Args))
	for index, argument := range call.Args {
		parts, ok := flow.expression(argument, environment)
		if !ok || len(parts) != 1 {
			return nil, false
		}
		arguments[index] = parts[0]
		ps6117AddFlow(dependencies, parts[0])
	}
	if flow.accelerators[call] {
		dependencies[call] = true
		return ps6117RepeatFlow(dependencies, ps6117ExpressionResultCount(flow.pass, call)), true
	}
	if declaration := flow.sites[ps6087FunctionID(flow.pass, call)]; declaration != nil {
		function, _ := flow.pass.TypesInfo.Defs[declaration.Name].(*types.Func)
		signature, _ := function.Type().(*types.Signature)
		if signature == nil || signature.Params().Len() != len(arguments) {
			return nil, false
		}
		inputs := make(map[types.Object]ps6117FlowSet, len(arguments)+1)
		for index := range arguments {
			inputs[signature.Params().At(index)] = arguments[index]
		}
		if signature.Recv() != nil {
			inputs[signature.Recv()] = receiverDependencies
		}
		return flow.function(declaration, inputs)
	}
	if !ps6117IntrinsicCall(flow.pass, call) {
		return nil, false
	}
	return ps6117RepeatFlow(dependencies, ps6117ExpressionResultCount(flow.pass, call)), true
}

func ps6117ExpressionResultCount(pass *analysis.Pass, expression ast.Expr) int {
	if tuple, ok := pass.TypesInfo.TypeOf(expression).(*types.Tuple); ok {
		return tuple.Len()
	}
	return 1
}

func ps6117RepeatFlow(dependencies ps6117FlowSet, count int) []ps6117FlowSet {
	if count <= 0 {
		return nil
	}
	result := make([]ps6117FlowSet, count)
	for index := range result {
		result[index] = ps6117CloneFlow(dependencies)
	}
	return result
}

func ps6117CloneFlow(source ps6117FlowSet) ps6117FlowSet {
	if len(source) == 0 {
		return nil
	}
	result := make(ps6117FlowSet, len(source))
	ps6117AddFlow(result, source)
	return result
}

func ps6117MergeFlow(sets ...ps6117FlowSet) ps6117FlowSet {
	result := make(ps6117FlowSet)
	for _, set := range sets {
		ps6117AddFlow(result, set)
	}
	return result
}

func ps6117FlattenFlow(sets []ps6117FlowSet) ps6117FlowSet {
	result := make(ps6117FlowSet)
	for _, set := range sets {
		ps6117AddFlow(result, set)
	}
	return result
}

func ps6117AddFlow(destination, source ps6117FlowSet) {
	for call := range source {
		destination[call] = true
	}
}

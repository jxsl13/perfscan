package checks

import (
	"fmt"
	"go/ast"
	"go/types"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6081FaithfulOwner(t *testing.T) {
	t.Parallel()
	contract := ps6081TestContract("ps6081owner.QuantSwiGLU.Forward")
	if !contract.Valid() {
		t.Fatal("test contract is invalid")
	}
	results := analysistest.Run(t, analysistest.TestData(), ps6081TestAnalyzer([]config.SharedFanOutContract{contract}), "ps6081owner")
	diagnostics := 0
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			diagnostics++
			if len(diagnostic.SuggestedFixes) != 0 {
				t.Fatalf("PS6081 diagnostic unexpectedly has %d suggested fixes", len(diagnostic.SuggestedFixes))
			}
		}
	}
	if diagnostics != 1 {
		t.Fatalf("PS6081 emitted %d diagnostics, want one exact owner", diagnostics)
	}
}

func TestPS6081SilentWithoutContract(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6081TestAnalyzer(nil), "ps6081silent")
}

func TestPS6081AmbiguousContractsStaySilent(t *testing.T) {
	t.Parallel()
	contract := ps6081TestContract("ps6081owner.QuantSwiGLU.Forward")
	duplicateName := contract
	duplicateName.OwnerSite = "ps6081owner.Other.Forward"
	duplicateClaim := contract
	duplicateClaim.Name = "other-name"
	if got := ps6081Contracts([]config.SharedFanOutContract{contract, duplicateName, duplicateClaim}); len(got) != 0 {
		t.Fatalf("ambiguous PS6081 contracts accepted: %+v", got)
	}
}

func TestPS6081DuplicateTransformCallableVariantsStaySilent(t *testing.T) {
	t.Parallel()
	contract := ps6081ReviewContract("ReceiverFields")
	duplicate := contract.TransformConsumers[0]
	duplicate.OperationConstant = "ps6081owner/backend.OpMul"
	duplicate.OperationConstantValue = "7"
	contract.TransformConsumers = append(contract.TransformConsumers, duplicate)
	if contract.Valid() {
		t.Fatal("duplicate transform callable variants formed a valid contract")
	}
	analysistest.Run(t, analysistest.TestData(), ps6081TestAnalyzer([]config.SharedFanOutContract{contract}), "ps6081review")
}

func TestPS6081DualFinalAndAlternateOperandsStaySilent(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"final", "alternate"} {
		target := target
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			contract := ps6081ReviewContract("ReceiverFields")
			consumer := &contract.FinalConsumer
			if target == "alternate" {
				consumer = &contract.AlternateConsumers[0]
			}
			consumer.CollectionArgument = 3
			consumer.CollectionIndexes = append([]int(nil), consumer.OperandArguments...)
			if contract.Valid() {
				t.Fatalf("dual %s operand representations formed a valid contract", target)
			}
			analysistest.Run(t, analysistest.TestData(), ps6081TestAnalyzer([]config.SharedFanOutContract{contract}), "ps6081review")
		})
	}
}

func TestPS6081ContractOrderIndependent(t *testing.T) {
	t.Parallel()
	contracts := []config.SharedFanOutContract{ps6081TestContract("ps6081owner.QuantSwiGLU.Forward")}
	slices.Reverse(contracts)
	analysistest.Run(t, analysistest.TestData(), ps6081TestAnalyzer(contracts), "ps6081owner")
}

func TestPS6081AlternateConsumerMatchingOrderIndependent(t *testing.T) {
	t.Parallel()
	contract := ps6081TestContract("ps6081owner.QuantSwiGLU.Forward")
	missing := config.SharedFanOutConsumer{
		Callable: "ps6081owner/backend.MissingFuser.Fuse", Kind: config.SharedFanOutMethod,
		ResultMode: config.SharedFanOutBoolCondition, OperandArguments: []int{2, 3},
	}
	contract.AlternateConsumers = append([]config.SharedFanOutConsumer{missing}, contract.AlternateConsumers...)
	if !contract.Valid() {
		t.Fatal("ordered alternate-consumer test contract is invalid")
	}
	analysistest.Run(t, analysistest.TestData(), ps6081TestAnalyzer([]config.SharedFanOutContract{contract}), "ps6081owner")
}

func TestPS6081OwnerMutationAndOrderingRegressions(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"InputRebinding", "ReceiverWeightMutation", "AliasAndFunctionHook", "BeforeDefinition", "OptionalTransform"} {
		method := method
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			contract := ps6081ReviewContract(method)
			if !contract.Valid() {
				t.Fatal("review contract is invalid")
			}
			analysistest.Run(t, analysistest.TestData(), ps6081TestAnalyzer([]config.SharedFanOutContract{contract}), "ps6081review")
		})
	}
}

func TestPS6081TypedReceiverRoleRegressions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*config.SharedFanOutContract)
	}{
		{name: "nonexistent field", mutate: func(contract *config.SharedFanOutContract) {
			contract.Producers[0].WeightReceiverField = "Missing"
			contract.Producers[1].WeightReceiverField = "Missing"
		}},
		{name: "swapped weight and metadata", mutate: func(contract *config.SharedFanOutContract) {
			for index := range contract.Producers {
				contract.Producers[index].WeightReceiverField = "Backend"
				contract.Producers[index].MetadataReceiverFields = []string{"Weight"}
			}
		}},
		{name: "repeated missing weight field", mutate: func(contract *config.SharedFanOutContract) {
			contract.Producers[0].WeightReceiverField = "Weight.Weight"
			contract.Producers[1].WeightReceiverField = "Weight.Weight"
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			contract := ps6081ReviewContract("ReceiverFields")
			test.mutate(&contract)
			if !contract.Valid() {
				t.Fatal("syntactically valid contradictory contract unexpectedly rejected before typed source resolution")
			}
			analysistest.Run(t, analysistest.TestData(), ps6081TestAnalyzer([]config.SharedFanOutContract{contract}), "ps6081review")
		})
	}
}

func TestPS6081RepeatedReceiverFieldPathsResolveExactly(t *testing.T) {
	t.Parallel()
	analyzer := &analysis.Analyzer{Name: "ps6081_repeated_field_path_test", Doc: "checks repeated PS6081 receiver field paths", Run: func(pass *analysis.Pass) (any, error) {
		for _, file := range pass.Files {
			for _, node := range file.Decls {
				declaration, ok := node.(*ast.FuncDecl)
				if !ok || declaration.Name.Name != "RepeatedForward" {
					continue
				}
				function, _ := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
				signature, _ := function.Type().(*types.Signature)
				producer := ps6081TestContract("unused").Producers[0]
				producer.DomainReceiverFields = []string{"Node.Node"}
				producer.WorkReceiverField = "A.B.A"
				if !ps6081ProducerRolesTyped(signature, &producer) {
					return nil, fmt.Errorf("valid repeated receiver field paths were rejected")
				}
				producer.WeightReceiverField = "Node.Missing.Node"
				if ps6081ProducerRolesTyped(signature, &producer) {
					return nil, fmt.Errorf("missing nested component was skipped in repeated receiver field path")
				}
				return nil, nil
			}
		}
		return nil, fmt.Errorf("RepeatedForward fixture not found")
	}}
	analysistest.Run(t, analysistest.TestData(), analyzer, "ps6081review")
}

func TestPS6081SameStorageContractIsInvalid(t *testing.T) {
	t.Parallel()
	contract := ps6081ReviewContract("ReceiverFields")
	contract.Producers[1].ReceiverPath = contract.Producers[0].ReceiverPath
	if contract.Valid() {
		t.Fatal("same-storage receiver paths formed a valid sibling contract")
	}
}

func TestPS6081ReachabilityIgnoresTerminatedAndNestedDeadRoutes(t *testing.T) {
	t.Parallel()
	wanted := map[string]bool{"DeadRoutes": true, "AfterReturn": true, "AfterPanic": true, "AfterTrueReturn": true}
	analyzer := &analysis.Analyzer{Name: "ps6081_reachability_test", Doc: "checks PS6081 route reachability", Run: func(pass *analysis.Pass) (any, error) {
		for _, file := range pass.Files {
			for _, node := range file.Decls {
				declaration, ok := node.(*ast.FuncDecl)
				if !ok || !wanted[declaration.Name.Name] {
					continue
				}
				found := 0
				ps6081ReachableCalls(pass, declaration, func(call *ast.CallExpr) {
					if ps6087FunctionID(pass, call) == "ps6081review.deadHelper" {
						found++
					}
				})
				if found != 0 {
					return nil, fmt.Errorf("%s counted %d unreachable helper calls", declaration.Name.Name, found)
				}
				delete(wanted, declaration.Name.Name)
			}
		}
		if len(wanted) != 0 {
			return nil, fmt.Errorf("missing dead-route fixtures: %v", wanted)
		}
		return nil, nil
	}}
	analysistest.Run(t, analysistest.TestData(), analyzer, "ps6081review")
}

func TestPS6081RouteCountRejectsTerminatedRoutes(t *testing.T) {
	t.Parallel()
	wanted := map[string]string{
		"RouteProducerAfterReturn":     "ps6081review.routeLive",
		"RouteProducerAfterPanic":      "ps6081review.routeLive",
		"RouteProducerAfterTrueReturn": "ps6081review.routeLive",
		"RouteTerminalAfterReturn":     "ps6081review.routeDeadReturn",
		"RouteTerminalAfterPanic":      "ps6081review.routeDeadPanic",
		"RouteTerminalAfterTrueReturn": "ps6081review.routeDeadTrueReturn",
	}
	analyzer := &analysis.Analyzer{Name: "ps6081_route_count_test", Doc: "checks PS6081 recursive route reachability", Run: func(pass *analysis.Pass) (any, error) {
		state := &ps6081State{pass: pass, declarations: make(map[*types.Func]*ast.FuncDecl)}
		byName := make(map[string]*ast.FuncDecl)
		for _, file := range pass.Files {
			for _, node := range file.Decls {
				declaration, ok := node.(*ast.FuncDecl)
				if !ok {
					continue
				}
				function, _ := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
				state.declarations[function] = declaration
				byName[declaration.Name.Name] = declaration
			}
		}
		for name, firstCallable := range wanted {
			declaration := byName[name]
			function, _ := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
			signature, _ := function.Type().(*types.Signature)
			route := []config.SharedFanOutRouteStep{
				{Callable: firstCallable, Kind: config.SharedFanOutFunction, DomainArguments: []int{1}, WorkArgument: 2},
				{Callable: "ps6081review.deadHelper", Kind: config.SharedFanOutFunction, DomainArguments: []int{1}, WorkArgument: 2, CallbackArgument: 3},
			}
			if got := state.routeCount(declaration, []types.Object{signature.Params().At(0)}, signature.Params().At(1), route, make(map[*types.Func]bool)); got != 0 {
				return nil, fmt.Errorf("%s counted %d terminated routes", name, got)
			}
		}
		for _, name := range []string{"OptionalRouteProducer", "OptionalNilSuccessRouteProducer"} {
			declaration := byName[name]
			function, _ := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
			signature, _ := function.Type().(*types.Signature)
			domains := []types.Object{signature.Params().At(0)}
			work := signature.Params().At(1)
			route := []config.SharedFanOutRouteStep{{Callable: "ps6081review.deadHelper", Kind: config.SharedFanOutFunction, DomainArguments: []int{1}, WorkArgument: 2, CallbackArgument: 3}}
			if got := state.routeCount(declaration, domains, work, route, make(map[*types.Func]bool)); got != 1 {
				return nil, fmt.Errorf("%s route count = %d, want one reachable syntactic route", name, got)
			}
			if state.routeCoversSuccessfulReturns(declaration, domains, work, route, 1, 2, make(map[*types.Func]bool)) {
				return nil, fmt.Errorf("%s route treated as covering alternate return", name)
			}
		}
		for _, test := range []struct {
			name  string
			route []config.SharedFanOutRouteStep
		}{
			{name: "DeferredRouteProducer", route: []config.SharedFanOutRouteStep{{Callable: "ps6081review.deadHelper", Kind: config.SharedFanOutFunction, DomainArguments: []int{1}, WorkArgument: 2, CallbackArgument: 3}}},
			{name: "RecursiveAsyncRouteProducer", route: []config.SharedFanOutRouteStep{
				{Callable: "ps6081review.asyncRouteWrapper", Kind: config.SharedFanOutFunction, DomainArguments: []int{1}, WorkArgument: 2},
				{Callable: "ps6081review.deadHelper", Kind: config.SharedFanOutFunction, DomainArguments: []int{1}, WorkArgument: 2, CallbackArgument: 3},
			}},
		} {
			declaration := byName[test.name]
			function, _ := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
			signature, _ := function.Type().(*types.Signature)
			domains := []types.Object{signature.Params().At(0)}
			work := signature.Params().At(1)
			if got := state.routeCount(declaration, domains, work, test.route, make(map[*types.Func]bool)); got != 1 {
				return nil, fmt.Errorf("%s route count = %d, want one syntactic route", test.name, got)
			}
			if state.routeCoversSuccessfulReturns(declaration, domains, work, test.route, 1, 2, make(map[*types.Func]bool)) {
				return nil, fmt.Errorf("%s treated async/deferred route as synchronous", test.name)
			}
		}
		return nil, nil
	}}
	analysistest.Run(t, analysistest.TestData(), analyzer, "ps6081review")
}

func TestPS6081OwnerStabilityTracksPreexistingAliases(t *testing.T) {
	t.Parallel()
	wanted := map[string]bool{
		"SafeAliases": true, "PointerMutation": false, "SliceMutation": false,
		"MapMutation": false, "ReceiverMutation": false, "WeightMutation": false,
		"AliasRebinding": false, "AliasEscape": false,
		"ContainerMutation": false, "PostVarContainerMutation": false, "InterfaceRebinding": false,
		"PreRangeAliasMutation": false, "PostRangeAliasMutation": false,
		"CommaOKAliasMutation": false, "MultiResultValueSpecAliasMutation": false,
		"MultiResultCallAliasMutation": false,
		"IndexedStoreAliasMutation":    false,
		"ClosureEscape":                false, "ClosureInvocation": false,
		"MethodValueInvocation": false, "CopyMutation": false,
		"DeleteMutation": false, "ClearMutation": false, "UnknownEffect": false,
	}
	analyzer := &analysis.Analyzer{Name: "ps6081_alias_stability_test", Doc: "checks PS6081 owner alias stability", Run: func(pass *analysis.Pass) (any, error) {
		state := &ps6081State{pass: pass}
		for _, file := range pass.Files {
			for _, node := range file.Decls {
				declaration, ok := node.(*ast.FuncDecl)
				if !ok {
					continue
				}
				expect, relevant := wanted[declaration.Name.Name]
				if !relevant {
					continue
				}
				root := ps6081ReceiverObject(pass, declaration)
				bindings := state.bindings(declaration.Body, "ps6081owner/linear.QuantLinear.Forward", "Gate", root, 1, 2)
				if len(bindings) != 1 {
					return nil, fmt.Errorf("%s has %d producer bindings", declaration.Name.Name, len(bindings))
				}
				permitted := map[*ast.CallExpr]bool{bindings[0].call: true}
				if got := state.ownerStableAfterProducer(declaration, root, bindings[0], 2, permitted); got != expect {
					return nil, fmt.Errorf("%s stability = %v, want %v", declaration.Name.Name, got, expect)
				}
				delete(wanted, declaration.Name.Name)
			}
		}
		if len(wanted) != 0 {
			return nil, fmt.Errorf("missing alias fixtures: %v", wanted)
		}
		return nil, nil
	}}
	analysistest.Run(t, analysistest.TestData(), analyzer, "ps6081alias")
}

func TestPS6081OwnerCFGRejectsDeadAndDisjointFlow(t *testing.T) {
	t.Parallel()
	wanted := map[string]bool{
		"OwnerAfterReturn": true, "OwnerAfterPanic": true, "OwnerAfterTrueReturn": true,
		"SplitBranches": true, "TransformAfterTerminatingBranch": true,
		"OptionalGate": true, "OptionalUp": true, "OptionalEarlyUp": true, "OptionalFallback": true,
	}
	analyzer := &analysis.Analyzer{Name: "ps6081_owner_cfg_test", Doc: "checks PS6081 owner control flow", Run: func(pass *analysis.Pass) (any, error) {
		for _, file := range pass.Files {
			for _, node := range file.Decls {
				declaration, ok := node.(*ast.FuncDecl)
				if !ok || !wanted[declaration.Name.Name] {
					continue
				}
				state := &ps6081State{pass: pass}
				parents := ps6087Parents(declaration.Body)
				state.reachable = ps6099ReachableCalls(pass, declaration, parents)
				state.flow = ps6089NewLifecycleFlow(pass, declaration.Body)
				root := ps6081ReceiverObject(pass, declaration)
				producers := state.bindings(declaration.Body, "ps6081owner/linear.QuantLinear.Forward", "Gate", root, 1, 2)
				up := state.bindings(declaration.Body, "ps6081owner/linear.QuantLinear.Forward", "Up", root, 1, 2)
				transforms := state.calls(declaration.Body, "ps6081owner/backend.Execute", config.SharedFanOutFunction, "", root)
				alternates := state.calls(declaration.Body, "ps6081owner/backend.SwiGLUInPlaceFuser.FuseSwiGLUInPlace", config.SharedFanOutMethod, "", root)
				switch declaration.Name.Name {
				case "OwnerAfterReturn", "OwnerAfterPanic", "OwnerAfterTrueReturn":
					if len(producers) != 0 {
						return nil, fmt.Errorf("%s exposed unreachable producer", declaration.Name.Name)
					}
				case "TransformAfterTerminatingBranch":
					if len(producers) != 1 || len(transforms) != 0 {
						return nil, fmt.Errorf("terminating branch exposed flow: producers=%d transforms=%d", len(producers), len(transforms))
					}
				case "SplitBranches":
					if len(producers) != 1 || len(transforms) != 1 || state.canReach(producers[0].call, transforms[0]) {
						return nil, fmt.Errorf("mutually exclusive producer/consumer accepted")
					}
				case "OptionalGate":
					if len(producers) != 1 || len(transforms) != 1 || state.dominates(producers[0].call, transforms[0]) {
						return nil, fmt.Errorf("optional Gate treated as dominating transform")
					}
				case "OptionalUp":
					if len(up) != 1 || len(transforms) != 1 || state.setDominates([]*ast.CallExpr{up[0].call}, transforms[0]) {
						return nil, fmt.Errorf("optional Up treated as dominating composite")
					}
				case "OptionalEarlyUp":
					if len(up) != 1 || len(alternates) != 1 || state.dominates(up[0].call, alternates[0]) {
						return nil, fmt.Errorf("optional early Up treated as dominating alternate consumer")
					}
				case "OptionalFallback":
					if len(up) != 2 || len(transforms) != 1 || state.fallbackCompletesUp(declaration, up, transforms[0]) {
						return nil, fmt.Errorf("optional outer fallback treated as completing Up")
					}
				}
				delete(wanted, declaration.Name.Name)
			}
		}
		if len(wanted) != 0 {
			return nil, fmt.Errorf("missing owner CFG fixtures: %v", wanted)
		}
		return nil, nil
	}}
	analysistest.Run(t, analysistest.TestData(), analyzer, "ps6081review")
}

func TestPS6081MetadataAndDocumentation(t *testing.T) {
	t.Parallel()
	if PS6081.AutoFix || !PS6081.NeedsConfig || PS6081.Level != 2 || PS6081.Category != "verify" || len(PS6081.Vocab) != 1 || PS6081.Vocab[0] != "sharedFanOutContracts" {
		t.Fatalf("PS6081 metadata drift: AutoFix=%v NeedsConfig=%v Level=%d Category=%q Vocab=%v", PS6081.AutoFix, PS6081.NeedsConfig, PS6081.Level, PS6081.Category, PS6081.Vocab)
	}
	documentation := strings.Join(strings.Fields(PS6081.Doc.Text+" "+PS6081.Doc.MeasuredWin), " ")
	for _, fragment := range []string{"fail closed", "exact typed", "receiver paths", "shape, dtype, backend, domain", "NO automatic fix", "no universal performance claim", "exact-output", "odd-tail", "alternating-order", "pre-GoAI-d5548e24"} {
		if !strings.Contains(documentation, fragment) {
			t.Errorf("PS6081 documentation missing %q", fragment)
		}
	}
}

func ps6081TestAnalyzer(contracts []config.SharedFanOutContract) *analysis.Analyzer {
	compiled := config.Config{SharedFanOutContracts: contracts}.Compile().SharedFanOutContracts
	analyzer := *PS6081.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) { return runPS6081WithContracts(pass, compiled) }
	return &analyzer
}

func ps6081TestContract(owner string) config.SharedFanOutContract {
	producer := func(path string) config.SharedFanOutProducer {
		return config.SharedFanOutProducer{
			Callable: "ps6081owner/linear.QuantLinear.Forward", Kind: config.SharedFanOutMethod, ReceiverPath: path,
			InputArgument: 2, WeightReceiverField: "Weight", MetadataReceiverFields: []string{"Backend"},
			DomainReceiverFields: []string{"N"}, WorkReceiverField: "K", DataResult: 1, ErrorResult: 2,
			Route: []config.SharedFanOutRouteStep{
				{Callable: "ps6081owner/gguf.QMatMul", Kind: config.SharedFanOutFunction, DomainArguments: []int{4}, WorkArgument: 5},
				{Callable: "ps6081owner/gguf.qmatmulParallelChunks", Kind: config.SharedFanOutFunction, DomainArguments: []int{1}, WorkArgument: 2, CallbackArgument: 3},
			},
		}
	}
	return config.SharedFanOutContract{
		Name: "goai-quant-swiglu", OwnerSite: owner, OwnerKind: config.SharedFanOutMethod,
		Producers:    []config.SharedFanOutProducer{producer("Gate"), producer("Up")},
		FanOutHelper: "ps6081owner/gguf.qmatmulParallelChunks", FanOutHelperKind: config.SharedFanOutFunction,
		FanOutDomainArguments: []int{1}, FanOutWorkArgument: 2, FanOutCallbackArgument: 3,
		TransformConsumers: []config.SharedFanOutConsumer{{Callable: "ps6081owner/backend.Execute", Kind: config.SharedFanOutFunction, ResultMode: config.SharedFanOutCollectionError, CollectionArgument: 3, CollectionIndexes: []int{1}, ResultCollectionIndexes: []int{1}, OperationArgument: 2, OperationConstant: "ps6081owner/backend.OpSiLU", OperationConstantValue: "5", DataResult: 1, ErrorResult: 2}},
		CompositeConsumer:  config.SharedFanOutConsumer{Callable: "ps6081owner/backend.Execute", Kind: config.SharedFanOutFunction, ResultMode: config.SharedFanOutCollectionError, CollectionArgument: 3, CollectionIndexes: []int{1, 2}, ResultCollectionIndexes: []int{1}, OperationArgument: 2, OperationConstant: "ps6081owner/backend.OpMul", OperationConstantValue: "7", DataResult: 1, ErrorResult: 2},
		FinalConsumer:      config.SharedFanOutConsumer{Callable: "ps6081owner/linear.QuantLinear.Forward", Kind: config.SharedFanOutMethod, ResultMode: config.SharedFanOutDirectReturn, ReceiverPath: "Down", OperandArguments: []int{2}},
		AlternateConsumers: []config.SharedFanOutConsumer{{Callable: "ps6081owner/backend.SwiGLUInPlaceFuser.FuseSwiGLUInPlace", Kind: config.SharedFanOutMethod, ResultMode: config.SharedFanOutBoolCondition, OperandArguments: []int{1, 2}}},
		AllowedEligibilityGuards: []config.SharedFanOutCallable{
			{Callable: "ps6081owner/backend.Default", Kind: config.SharedFanOutFunction},
			{Callable: "ps6081owner/backend.Backend.Name", Kind: config.SharedFanOutMethod},
		},
		InputsImmutable: true, WeightsImmutable: true, ProducersSideEffectFree: true,
		FreshOwnedNonescapingOutputs: true, DisjointWrites: true, SynchronousCompletion: true,
		CompositeMeaningPreserved: true, ErrorParity: true, PanicParity: true, BackendSelectionParity: true,
		FallbackParity: true, ExactShapeCompatibility: true, ExactDTypeCompatibility: true,
		ExactBackendCompatibility: true, ExactFanOutDomainMapping: true, ExactOutputValidationRequired: true,
		OddTailValidationRequired: true, PairedBenchmarkRequired: true,
	}
}

func ps6081ReviewContract(method string) config.SharedFanOutContract {
	contract := ps6081TestContract("ps6081review.Layer." + method)
	contract.Name = "review-" + method
	return contract
}

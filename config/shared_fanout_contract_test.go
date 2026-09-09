package config

import "testing"

func TestSharedFanOutContractValid(t *testing.T) {
	t.Parallel()
	contract := sharedFanOutConfigContract()
	if !contract.Valid() {
		t.Fatal("complete shared-fan-out contract is invalid")
	}
	contract.InputsImmutable = false
	if contract.Valid() {
		t.Fatal("incomplete semantic assertions must fail closed")
	}
}

func TestSharedFanOutContractRejectsAmbiguousRoles(t *testing.T) {
	t.Parallel()
	contract := sharedFanOutConfigContract()
	contract.Producers[1] = contract.Producers[0]
	if contract.Valid() {
		t.Fatal("duplicate producer claims must be invalid")
	}
}

func TestSharedFanOutContractRejectsDuplicateTransformCallableVariants(t *testing.T) {
	t.Parallel()
	contract := sharedFanOutConfigContract()
	duplicate := contract.TransformConsumers[0]
	duplicate.OperationArgument = 2
	duplicate.OperationConstant = "example.com/p.OtherOperation"
	duplicate.OperationConstantValue = "2"
	contract.TransformConsumers = append(contract.TransformConsumers, duplicate)
	if contract.Valid() {
		t.Fatal("duplicate transform callable variants formed a valid contract")
	}
}

func TestSharedFanOutContractRejectsDualFinalAndAlternateOperands(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"final", "alternate"} {
		target := target
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			contract := sharedFanOutConfigContract()
			selected := &contract.FinalConsumer
			if target == "alternate" {
				alternate := SharedFanOutConsumer{Callable: "example.com/p.Fuser.Fuse", Kind: SharedFanOutMethod, ResultMode: SharedFanOutBoolCondition, OperandArguments: []int{1, 2}}
				contract.AlternateConsumers = []SharedFanOutConsumer{alternate}
				selected = &contract.AlternateConsumers[0]
			}
			selected.CollectionArgument = 3
			selected.CollectionIndexes = append([]int(nil), selected.OperandArguments...)
			if contract.Valid() {
				t.Fatalf("dual %s operand representations formed a valid contract", target)
			}
		})
	}
}

func TestSharedFanOutContractRequiresAnalyzerRepresentableConsumerFlow(t *testing.T) {
	t.Parallel()
	if got := UsableSharedFanOutContractCount([]SharedFanOutContract{sharedFanOutConfigContract()}); got != 1 {
		t.Fatalf("supported contract count = %d, want one", got)
	}
	if got := UsableSharedFanOutContractCount([]SharedFanOutContract{sharedFanOutMethodConfigContract()}); got != 1 {
		t.Fatalf("supported method-role contract count = %d, want one", got)
	}
	tests := []struct {
		name   string
		mutate func(*SharedFanOutContract)
	}{
		{name: "no alternate", mutate: func(contract *SharedFanOutContract) { contract.AlternateConsumers = nil }},
		{name: "transform bool", mutate: func(contract *SharedFanOutContract) {
			contract.TransformConsumers[0].ResultMode = SharedFanOutBoolCondition
			contract.TransformConsumers[0].DataResult, contract.TransformConsumers[0].ErrorResult = 0, 0
		}},
		{name: "composite direct return", mutate: func(contract *SharedFanOutContract) {
			contract.CompositeConsumer.ResultMode = SharedFanOutDirectReturn
			contract.CompositeConsumer.DataResult, contract.CompositeConsumer.ErrorResult = 0, 0
		}},
		{name: "final data error", mutate: func(contract *SharedFanOutContract) {
			contract.FinalConsumer.ResultMode = SharedFanOutDataError
			contract.FinalConsumer.DataResult, contract.FinalConsumer.ErrorResult = 1, 2
		}},
		{name: "alternate direct return", mutate: func(contract *SharedFanOutContract) {
			contract.AlternateConsumers[0].ResultMode = SharedFanOutDirectReturn
		}},
		{name: "function owner", mutate: func(contract *SharedFanOutContract) {
			contract.OwnerKind = SharedFanOutFunction
			contract.OwnerSite = "example.com/p.owner"
		}},
		{name: "function producer receiver path", mutate: func(contract *SharedFanOutContract) {
			contract.Producers[0].ReceiverPath = "Gate"
		}},
		{name: "missing producer domain role", mutate: func(contract *SharedFanOutContract) {
			contract.Producers[0].DomainArguments = nil
			contract.Producers[1].DomainArguments = nil
		}},
		{name: "differing receiver field lists", mutate: func(contract *SharedFanOutContract) {
			*contract = sharedFanOutMethodConfigContract()
			contract.Producers[1].DomainReceiverFields = []string{"N"}
		}},
		{name: "method weight work field overlap", mutate: func(contract *SharedFanOutContract) {
			*contract = sharedFanOutMethodConfigContract()
			contract.Producers[0].WorkReceiverField = "Weight"
			contract.Producers[1].WorkReceiverField = "Weight"
		}},
		{name: "operation operand overlap", mutate: func(contract *SharedFanOutContract) {
			contract.TransformConsumers[0].OperationArgument = 1
			contract.TransformConsumers[0].OperationConstant = "example.com/p.Op"
			contract.TransformConsumers[0].OperationConstantValue = "1"
		}},
		{name: "function consumer receiver path", mutate: func(contract *SharedFanOutContract) {
			contract.FinalConsumer.ReceiverPath = "Down"
		}},
		{name: "duplicate alternate lookup", mutate: func(contract *SharedFanOutContract) {
			contract.AlternateConsumers = append(contract.AlternateConsumers, contract.AlternateConsumers[0])
		}},
		{name: "final producer lookup collision", mutate: func(contract *SharedFanOutContract) {
			contract.FinalConsumer.Callable = contract.Producers[0].Callable
			contract.FinalConsumer.Kind = contract.Producers[0].Kind
			contract.FinalConsumer.ReceiverPath = contract.Producers[0].ReceiverPath
		}},
		{name: "all transforms collide with producer lookup", mutate: func(contract *SharedFanOutContract) {
			contract.TransformConsumers[0].Callable = contract.Producers[0].Callable
			contract.TransformConsumers[0].Kind = contract.Producers[0].Kind
			contract.TransformConsumers[0].ReceiverPath = contract.Producers[0].ReceiverPath
		}},
		{name: "callback domain overlap", mutate: func(contract *SharedFanOutContract) {
			contract.FanOutCallbackArgument = contract.FanOutDomainArguments[0]
			for index := range contract.Producers {
				last := len(contract.Producers[index].Route) - 1
				contract.Producers[index].Route[last].CallbackArgument = contract.FanOutCallbackArgument
			}
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			contract := sharedFanOutConfigContract()
			test.mutate(&contract)
			if contract.Valid() {
				t.Fatal("analyzer-unrepresentable consumer flow formed a valid contract")
			}
			if got := UsableSharedFanOutContractCount([]SharedFanOutContract{contract}); got != 0 {
				t.Fatalf("unrepresentable usable contract count = %d, want zero", got)
			}
		})
	}
}

func TestSharedFanOutContractAllowsDistinctFinalAndAlternateOperations(t *testing.T) {
	t.Parallel()
	contract := sharedFanOutConfigContract()
	contract.FinalConsumer.OperationArgument = 2
	contract.FinalConsumer.OperationConstant = "example.com/p.FinalOp"
	contract.FinalConsumer.OperationConstantValue = "1"
	contract.AlternateConsumers[0].OperationArgument = 3
	contract.AlternateConsumers[0].OperationConstant = "example.com/p.AlternateOp"
	contract.AlternateConsumers[0].OperationConstantValue = "2"
	if !contract.Valid() {
		t.Fatal("distinct exact final/alternate operation roles were rejected")
	}
}

func TestCompileDeepClonesSharedFanOutContracts(t *testing.T) {
	t.Parallel()
	contract := sharedFanOutConfigContract()
	configured := Config{SharedFanOutContracts: []SharedFanOutContract{contract}}
	compiled := configured.Compile()
	configured.SharedFanOutContracts[0].Producers[0].Route[0].DomainArguments[0] = 99
	configured.SharedFanOutContracts[0].TransformConsumers[0].OperandArguments[0] = 99
	configured.SharedFanOutContracts[0].CompositeConsumer.OperandArguments[0] = 99
	configured.SharedFanOutContracts[0].AllowedEligibilityGuards[0].Callable = "changed.example/p.Guard"
	if compiled.SharedFanOutContracts[0].Producers[0].Route[0].DomainArguments[0] != 1 ||
		compiled.SharedFanOutContracts[0].TransformConsumers[0].OperandArguments[0] != 1 ||
		compiled.SharedFanOutContracts[0].CompositeConsumer.OperandArguments[0] != 1 ||
		compiled.SharedFanOutContracts[0].AllowedEligibilityGuards[0].Callable != "example.com/p.Value.Ready" {
		t.Fatal("Compile did not deeply clone shared-fan-out contracts")
	}
}

func TestSharedFanOutContractRejectsTerminalMappingMismatch(t *testing.T) {
	t.Parallel()
	for _, mutate := range []func(*SharedFanOutContract){
		func(contract *SharedFanOutContract) { contract.Producers[0].Route[0].DomainArguments = []int{2} },
		func(contract *SharedFanOutContract) { contract.Producers[0].Route[0].WorkArgument = 3 },
		func(contract *SharedFanOutContract) { contract.Producers[0].Route[0].CallbackArgument = 4 },
		func(contract *SharedFanOutContract) { contract.Producers[0].Route[0].Kind = SharedFanOutMethod },
	} {
		contract := sharedFanOutConfigContract()
		mutate(&contract)
		if contract.Valid() {
			t.Fatal("terminal route mapping disagreement formed a valid contract")
		}
	}
}

func TestUsableSharedFanOutContractsRejectDuplicateClaims(t *testing.T) {
	t.Parallel()
	contract := sharedFanOutConfigContract()
	duplicateName := contract
	duplicateName.OwnerSite = "example.com/p.Other.Forward"
	duplicateClaim := contract
	duplicateClaim.Name = "other"
	if got := UsableSharedFanOutContractCount([]SharedFanOutContract{contract, duplicateName, duplicateClaim}); got != 0 {
		t.Fatalf("usable contracts = %d, want zero for duplicate names and claims", got)
	}
}

func sharedFanOutConfigContract() SharedFanOutContract {
	producer := func(callable string) SharedFanOutProducer {
		return SharedFanOutProducer{Callable: callable, Kind: SharedFanOutFunction, InputArgument: 1, WeightArgument: 2, DomainArguments: []int{3}, WorkArgument: 4, DataResult: 1, ErrorResult: 2, Route: []SharedFanOutRouteStep{{Callable: "example.com/p.parallel", Kind: SharedFanOutFunction, DomainArguments: []int{1}, WorkArgument: 2, CallbackArgument: 3}}}
	}
	consumer := func(callable string, operands ...int) SharedFanOutConsumer {
		return SharedFanOutConsumer{Callable: callable, Kind: SharedFanOutFunction, ResultMode: SharedFanOutDataError, OperandArguments: operands, DataResult: 1, ErrorResult: 2}
	}
	return SharedFanOutContract{Name: "shared", OwnerSite: "example.com/p.Owner.Forward", OwnerKind: SharedFanOutMethod, Producers: []SharedFanOutProducer{producer("example.com/p.gate"), producer("example.com/p.up")}, FanOutHelper: "example.com/p.parallel", FanOutHelperKind: SharedFanOutFunction, FanOutDomainArguments: []int{1}, FanOutWorkArgument: 2, FanOutCallbackArgument: 3, TransformConsumers: []SharedFanOutConsumer{consumer("example.com/p.silu", 1)}, CompositeConsumer: consumer("example.com/p.mul", 1, 2), FinalConsumer: SharedFanOutConsumer{Callable: "example.com/p.down", Kind: SharedFanOutFunction, ResultMode: SharedFanOutDirectReturn, OperandArguments: []int{1}}, AlternateConsumers: []SharedFanOutConsumer{{Callable: "example.com/p.fuse", Kind: SharedFanOutFunction, ResultMode: SharedFanOutBoolCondition, OperandArguments: []int{1, 2}}}, AllowedEligibilityGuards: []SharedFanOutCallable{{Callable: "example.com/p.Value.Ready", Kind: SharedFanOutMethod}}, InputsImmutable: true, WeightsImmutable: true, ProducersSideEffectFree: true, FreshOwnedNonescapingOutputs: true, DisjointWrites: true, SynchronousCompletion: true, CompositeMeaningPreserved: true, ErrorParity: true, PanicParity: true, BackendSelectionParity: true, FallbackParity: true, ExactShapeCompatibility: true, ExactDTypeCompatibility: true, ExactBackendCompatibility: true, ExactFanOutDomainMapping: true, ExactOutputValidationRequired: true, OddTailValidationRequired: true, PairedBenchmarkRequired: true}
}

func sharedFanOutMethodConfigContract() SharedFanOutContract {
	contract := sharedFanOutConfigContract()
	for index, path := range []string{"Gate", "Up"} {
		producer := &contract.Producers[index]
		producer.Callable = "example.com/p.Producer.Forward"
		producer.Kind = SharedFanOutMethod
		producer.ReceiverPath = path
		producer.WeightArgument = 0
		producer.WeightReceiverField = "Weight"
		producer.WorkArgument = 0
		producer.WorkReceiverField = "K"
	}
	return contract
}

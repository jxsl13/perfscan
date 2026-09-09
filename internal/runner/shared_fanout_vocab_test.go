package runner

import (
	"testing"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

func TestMissingVocabSharedFanOutContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"sharedFanOutContracts"}}
	if got := missingVocab(check, &config.Config{}); len(got) != 1 || got[0] != "sharedFanOutContracts" {
		t.Fatalf("missingVocab(empty) = %v", got)
	}
	bareFanOut := config.Config{FanOutHelpers: []string{"example.com/p.parallel"}}
	if got := missingVocab(check, &bareFanOut); len(got) != 1 {
		t.Fatalf("bare fanOutHelpers activated PS6081: %v", got)
	}
	invalid := config.Config{SharedFanOutContracts: []config.SharedFanOutContract{{Name: "incomplete"}}}
	if got := missingVocab(check, &invalid); len(got) != 1 {
		t.Fatalf("missingVocab(invalid) = %v", got)
	}
	configured := config.Config{SharedFanOutContracts: []config.SharedFanOutContract{sharedFanOutRunnerContract()}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(valid) = %v", got)
	}
	unsupported := []struct {
		name   string
		mutate func(*config.SharedFanOutContract)
	}{
		{name: "no alternate", mutate: func(contract *config.SharedFanOutContract) { contract.AlternateConsumers = nil }},
		{name: "transform bool", mutate: func(contract *config.SharedFanOutContract) {
			contract.TransformConsumers[0].ResultMode = config.SharedFanOutBoolCondition
			contract.TransformConsumers[0].DataResult, contract.TransformConsumers[0].ErrorResult = 0, 0
		}},
		{name: "composite direct return", mutate: func(contract *config.SharedFanOutContract) {
			contract.CompositeConsumer.ResultMode = config.SharedFanOutDirectReturn
			contract.CompositeConsumer.DataResult, contract.CompositeConsumer.ErrorResult = 0, 0
		}},
		{name: "final data error", mutate: func(contract *config.SharedFanOutContract) {
			contract.FinalConsumer.ResultMode = config.SharedFanOutDataError
			contract.FinalConsumer.DataResult, contract.FinalConsumer.ErrorResult = 1, 2
		}},
		{name: "alternate direct return", mutate: func(contract *config.SharedFanOutContract) {
			contract.AlternateConsumers[0].ResultMode = config.SharedFanOutDirectReturn
		}},
	}
	for _, test := range unsupported {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			contract := sharedFanOutRunnerContract()
			test.mutate(&contract)
			candidate := config.Config{SharedFanOutContracts: []config.SharedFanOutContract{contract}}
			if got := missingVocab(check, &candidate); len(got) != 1 || got[0] != "sharedFanOutContracts" {
				t.Fatalf("missingVocab(unrepresentable) = %v", got)
			}
		})
	}
	contract := sharedFanOutRunnerContract()
	duplicateName := contract
	duplicateName.OwnerSite = "example.com/p.Other.Forward"
	duplicateClaim := contract
	duplicateClaim.Name = "other"
	ambiguous := config.Config{SharedFanOutContracts: []config.SharedFanOutContract{contract, duplicateName, duplicateClaim}}
	if got := missingVocab(check, &ambiguous); len(got) != 1 || got[0] != "sharedFanOutContracts" {
		t.Fatalf("ambiguous contracts did not produce missing vocabulary: %v", got)
	}
}

func sharedFanOutRunnerContract() config.SharedFanOutContract {
	producer := func(callable string) config.SharedFanOutProducer {
		return config.SharedFanOutProducer{Callable: callable, Kind: config.SharedFanOutFunction, InputArgument: 1, WeightArgument: 2, DomainArguments: []int{3}, WorkArgument: 4, DataResult: 1, ErrorResult: 2, Route: []config.SharedFanOutRouteStep{{Callable: "example.com/p.parallel", Kind: config.SharedFanOutFunction, DomainArguments: []int{1}, WorkArgument: 2, CallbackArgument: 3}}}
	}
	consumer := func(callable string, operands ...int) config.SharedFanOutConsumer {
		return config.SharedFanOutConsumer{Callable: callable, Kind: config.SharedFanOutFunction, ResultMode: config.SharedFanOutDataError, OperandArguments: operands, DataResult: 1, ErrorResult: 2}
	}
	return config.SharedFanOutContract{Name: "shared", OwnerSite: "example.com/p.Owner.Forward", OwnerKind: config.SharedFanOutMethod, Producers: []config.SharedFanOutProducer{producer("example.com/p.gate"), producer("example.com/p.up")}, FanOutHelper: "example.com/p.parallel", FanOutHelperKind: config.SharedFanOutFunction, FanOutDomainArguments: []int{1}, FanOutWorkArgument: 2, FanOutCallbackArgument: 3, TransformConsumers: []config.SharedFanOutConsumer{consumer("example.com/p.silu", 1)}, CompositeConsumer: consumer("example.com/p.mul", 1, 2), FinalConsumer: config.SharedFanOutConsumer{Callable: "example.com/p.down", Kind: config.SharedFanOutFunction, ResultMode: config.SharedFanOutDirectReturn, OperandArguments: []int{1}}, AlternateConsumers: []config.SharedFanOutConsumer{{Callable: "example.com/p.fuse", Kind: config.SharedFanOutFunction, ResultMode: config.SharedFanOutBoolCondition, OperandArguments: []int{1, 2}}}, InputsImmutable: true, WeightsImmutable: true, ProducersSideEffectFree: true, FreshOwnedNonescapingOutputs: true, DisjointWrites: true, SynchronousCompletion: true, CompositeMeaningPreserved: true, ErrorParity: true, PanicParity: true, BackendSelectionParity: true, FallbackParity: true, ExactShapeCompatibility: true, ExactDTypeCompatibility: true, ExactBackendCompatibility: true, ExactFanOutDomainMapping: true, ExactOutputValidationRequired: true, OddTailValidationRequired: true, PairedBenchmarkRequired: true}
}

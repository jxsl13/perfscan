package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6109(t *testing.T) {
	t.Parallel()
	analyzer := *PS6109.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6109WithContracts(pass, ps6109TestContracts())
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6109")
}

func TestPS6109NoConfig(t *testing.T) {
	t.Parallel()
	analyzer := *PS6109.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6109WithContracts(pass, nil)
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6109noconfig")
}

func TestPS6109NoSuggestedFixes(t *testing.T) {
	t.Parallel()
	analyzer := *PS6109.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6109WithContracts(pass, ps6109TestContracts())
	}
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), &analyzer, "ps6109")
}

func TestPS6109RejectsMisboundConcreteMethod(t *testing.T) {
	t.Parallel()
	contract := ps6109MisboundMethodContract()
	analyzer := *PS6109.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6109WithContracts(pass, []config.ReusableOneShotWrapperContract{contract})
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6109misbound")
}

func TestPS6109RejectsMalformedTypedRoles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*config.ReusableOneShotWrapperContract)
	}{
		{"wrong reset status", func(contract *config.ReusableOneShotWrapperContract) {
			contract.Reset = config.ReusableOneShotMethod{Static: "ps6109badmethod.recorder.ResetInt", Concrete: "ps6109badmethod.recorder.ResetInt"}
			contract.ResetFailureState = config.ReusableOneShotResetErrorEmptySafe
			contract.ResetStatusResult = 1
		}},
		{"variadic use", func(contract *config.ReusableOneShotWrapperContract) {
			contract.AllowedSynchronousUses = []config.ReusableOneShotMethod{{Static: "ps6109badmethod.recorder.EncodeVariadic", Concrete: "ps6109badmethod.recorder.EncodeVariadic"}}
		}},
		{"wrong wrapper result", func(contract *config.ReusableOneShotWrapperContract) {
			contract.AcquisitionWrapperResult = 2
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			contract := ps6109BadMethodContract()
			contract.Terminal.Concrete = contract.Terminal.Static
			test.mutate(&contract)
			analyzer := *PS6109.Analyzer
			analyzer.Run = func(pass *analysis.Pass) (any, error) {
				return runPS6109WithContracts(pass, []config.ReusableOneShotWrapperContract{contract})
			}
			analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6109badmethod")
		})
	}
}

func TestPS6109ImportedFactoryFact(t *testing.T) {
	t.Parallel()
	contract := ps6109ImportedContract()
	analyzer := *PS6109.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6109WithContracts(pass, []config.ReusableOneShotWrapperContract{contract})
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6109backend", "ps6109import")
}

func TestPS6109ImportedMissingFact(t *testing.T) {
	t.Parallel()
	contract := ps6109ImportedContract()
	contract.Name = "imported-unproved-shell"
	contract.WrapperType = "ps6109opaque.Recorder"
	contract.ProviderType = "ps6109opaque.Device"
	contract.Acquisition = "ps6109opaque.CommandFactory.Acquire"
	contract.ConcreteAcquisition = "ps6109opaque.Device.Acquire"
	contract.WrapperConstructor = "ps6109opaque.NewRecorder"
	contract.Terminal = config.ReusableOneShotMethod{Static: "ps6109opaque.CommandRecorder.Free", Concrete: "ps6109opaque.Recorder.Free"}
	contract.Reset = config.ReusableOneShotMethod{Static: "ps6109opaque.CommandRecorder.Reset", Concrete: "ps6109opaque.Recorder.Reset"}
	contract.FreshNativeHandleFactory = "ps6109opaque.Device.Fresh"
	contract.NativeHandleField = "ps6109opaque.Recorder.Handle"
	contract.MutableStateFields = []string{"ps6109opaque.Recorder.State"}
	contract.AllowedSynchronousUses = []config.ReusableOneShotMethod{{Static: "ps6109opaque.CommandRecorder.Encode", Concrete: "ps6109opaque.Recorder.Encode"}}
	analyzer := *PS6109.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6109WithContracts(pass, []config.ReusableOneShotWrapperContract{contract})
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6109opaque", "ps6109importnofact")
}

func TestPS6109ImportedFactsPreserveBothContractOrders(t *testing.T) {
	t.Parallel()
	interfaceContract := ps6109ImportedContract()
	directContract := interfaceContract
	directContract.Name = "imported-direct-shell"
	directContract.Acquisition = directContract.ConcreteAcquisition
	for _, test := range []struct {
		name      string
		contracts []config.ReusableOneShotWrapperContract
	}{
		{"interface then direct", []config.ReusableOneShotWrapperContract{interfaceContract, directContract}},
		{"direct then interface", []config.ReusableOneShotWrapperContract{directContract, interfaceContract}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			analyzer := *PS6109.Analyzer
			analyzer.Run = func(pass *analysis.Pass) (any, error) {
				return runPS6109WithContracts(pass, test.contracts)
			}
			analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6109backend", "ps6109importboth")
		})
	}
}

func TestPS6109ImportedFactSupportsConsumerOwnedStaticInterface(t *testing.T) {
	t.Parallel()
	contract := ps6109ImportedContract()
	contract.Name = "consumer-interface-shell"
	contract.Acquisition = "ps6109importconsumer.consumerFactory.Acquire"
	analyzer := *PS6109.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6109WithContracts(pass, []config.ReusableOneShotWrapperContract{contract})
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6109backend", "ps6109importconsumer")
}

func TestPS6109ImportedFactRejectsMismatchedConstructorProof(t *testing.T) {
	t.Parallel()
	carrier := ps6109ImportedContract()
	carrier.Name = "producer-proof-carrier"
	carrier.Acquisition = carrier.ConcreteAcquisition
	mismatch := ps6109ImportedContract()
	mismatch.Name = "mismatched-constructor"
	mismatch.WrapperConstructor = "ps6109backend.NewRecorderAlternate"
	analyzer := *PS6109.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6109WithContracts(pass, []config.ReusableOneShotWrapperContract{carrier, mismatch})
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6109backend", "ps6109importmismatch")
}

func ps6109TestContract() config.ReusableOneShotWrapperContract {
	return config.ReusableOneShotWrapperContract{
		Name:                              "interface-recorder-shell",
		WrapperType:                       "ps6109.recorder",
		ProviderType:                      "ps6109.device",
		Acquisition:                       "ps6109.commandFactory.NewRecorder",
		ConcreteAcquisition:               "ps6109.device.NewRecorder",
		WrapperConstructor:                "ps6109.newRecorder",
		Terminal:                          config.ReusableOneShotMethod{Static: "ps6109.commandRecorder.Free", Concrete: "ps6109.recorder.Free"},
		Reset:                             config.ReusableOneShotMethod{Static: "ps6109.commandRecorder.Reset", Concrete: "ps6109.recorder.Reset"},
		FreshNativeHandleFactory:          "ps6109.device.NewCommandBuffer",
		NativeHandleField:                 "ps6109.recorder.handle",
		MutableStateFields:                []string{"ps6109.recorder.encoder", "ps6109.recorder.committed"},
		AllowedSynchronousUses:            []config.ReusableOneShotMethod{{Static: "ps6109.commandRecorder.Encode", Concrete: "ps6109.recorder.Encode"}},
		AcquisitionWrapperResult:          1,
		AcquisitionStatusResult:           2,
		ConstructorWrapperResult:          1,
		ResetStatusResult:                 1,
		AcquisitionFailureMode:            config.ReusableOneShotAcquisitionNilError,
		ResetFailureState:                 config.ReusableOneShotResetErrorEmptySafe,
		SlotBound:                         1,
		AcquisitionCreatesFreshGoWrapper:  true,
		AcquisitionHasExactDynamicWrapper: true,
		FailedAcquisitionHasNoGeneration:  true,
		NativeHandleIsOneShot:             true,
		ResetAlwaysCreatesFreshHandle:     true,
		TerminalSynchronouslyReleases:     true,
		TerminalClearsHandle:              true,
		TerminalIsIdempotent:              true,
		ResetClearsMutableState:           true,
		UsesExecuteSynchronously:          true,
		UsesDoNotRetainGeneration:         true,
		NoStaleGenerationReferences:       true,
		OwnerAccessIsNonConcurrent:        true,
		ProviderFallbackIsPreserved:       true,
		FailuresAndPanicsArePreserved:     true,
	}
}

func ps6109TestContracts() []config.ReusableOneShotWrapperContract {
	interfaceContract := ps6109TestContract()
	directContract := interfaceContract
	directContract.Name = "direct-recorder-shell"
	directContract.Acquisition = directContract.ConcreteAcquisition
	twoLiveContract := interfaceContract
	twoLiveContract.Name = "two-live-recorder-shell"
	twoLiveContract.Acquisition = "ps6109.device.NewRecorderFast"
	twoLiveContract.ConcreteAcquisition = twoLiveContract.Acquisition
	twoLiveContract.AcquisitionFailureMode = config.ReusableOneShotAcquisitionInfallible
	twoLiveContract.AcquisitionStatusResult = 0
	twoLiveContract.Reset = config.ReusableOneShotMethod{Static: "ps6109.commandRecorder.ResetFast", Concrete: "ps6109.recorder.ResetFast"}
	twoLiveContract.ResetFailureState = config.ReusableOneShotResetInfallibleEmpty
	twoLiveContract.ResetStatusResult = 0
	twoLiveContract.SlotBound = 2
	contracts := []config.ReusableOneShotWrapperContract{interfaceContract, directContract, twoLiveContract}
	for _, invalid := range []struct {
		name        string
		acquisition string
		constructor string
	}{
		{"altered-provider", "ps6109.device.NewRecorderAltered", "ps6109.newRecorder"},
		{"constructor-state", "ps6109.device.NewRecorderWithState", "ps6109.newRecorderWithState"},
		{"distinct-native-root", "ps6109.device.NewRecorderDistinctNative", "ps6109.newRecorderDistinctNative"},
		{"boxed-provider", "ps6109.device.NewRecorderBoxed", "ps6109.newRecorderBoxed"},
	} {
		contract := twoLiveContract
		contract.Name = invalid.name
		contract.Acquisition = invalid.acquisition
		contract.ConcreteAcquisition = invalid.acquisition
		contract.WrapperConstructor = invalid.constructor
		contract.SlotBound = 1
		contracts = append(contracts, contract)
	}
	return contracts
}

func ps6109BadMethodContract() config.ReusableOneShotWrapperContract {
	contract := config.ReusableOneShotWrapperContract{
		Name:                              "misbound-terminal",
		WrapperType:                       "ps6109badmethod.recorder",
		ProviderType:                      "ps6109badmethod.device",
		Acquisition:                       "ps6109badmethod.device.Acquire",
		ConcreteAcquisition:               "ps6109badmethod.device.Acquire",
		WrapperConstructor:                "ps6109badmethod.newRecorder",
		Terminal:                          config.ReusableOneShotMethod{Static: "ps6109badmethod.recorder.Free", Concrete: "ps6109badmethod.recorder.Other"},
		Reset:                             config.ReusableOneShotMethod{Static: "ps6109badmethod.recorder.Reset", Concrete: "ps6109badmethod.recorder.Reset"},
		FreshNativeHandleFactory:          "ps6109badmethod.device.Fresh",
		NativeHandleField:                 "ps6109badmethod.recorder.handle",
		MutableStateFields:                []string{"ps6109badmethod.recorder.state"},
		AllowedSynchronousUses:            []config.ReusableOneShotMethod{{Static: "ps6109badmethod.recorder.Encode", Concrete: "ps6109badmethod.recorder.Encode"}},
		AcquisitionWrapperResult:          1,
		ConstructorWrapperResult:          1,
		AcquisitionFailureMode:            config.ReusableOneShotAcquisitionInfallible,
		ResetFailureState:                 config.ReusableOneShotResetInfallibleEmpty,
		SlotBound:                         1,
		AcquisitionCreatesFreshGoWrapper:  true,
		AcquisitionHasExactDynamicWrapper: true,
		FailedAcquisitionHasNoGeneration:  true,
		NativeHandleIsOneShot:             true,
		ResetAlwaysCreatesFreshHandle:     true,
		TerminalSynchronouslyReleases:     true,
		TerminalClearsHandle:              true,
		TerminalIsIdempotent:              true,
		ResetClearsMutableState:           true,
		UsesExecuteSynchronously:          true,
		UsesDoNotRetainGeneration:         true,
		NoStaleGenerationReferences:       true,
		OwnerAccessIsNonConcurrent:        true,
		ProviderFallbackIsPreserved:       true,
		FailuresAndPanicsArePreserved:     true,
	}
	return contract
}

func ps6109MisboundMethodContract() config.ReusableOneShotWrapperContract {
	contract := ps6109BadMethodContract()
	contract.Name = "misbound-import-fact"
	contract.WrapperType = "ps6109misbound.recorder"
	contract.ProviderType = "ps6109misbound.device"
	contract.Acquisition = "ps6109misbound.device.Acquire"
	contract.ConcreteAcquisition = "ps6109misbound.device.Acquire"
	contract.WrapperConstructor = "ps6109misbound.newRecorder"
	contract.Terminal = config.ReusableOneShotMethod{Static: "ps6109misbound.recorder.Free", Concrete: "ps6109misbound.recorder.Other"}
	contract.Reset = config.ReusableOneShotMethod{Static: "ps6109misbound.recorder.Reset", Concrete: "ps6109misbound.recorder.Reset"}
	contract.FreshNativeHandleFactory = "ps6109misbound.device.Fresh"
	contract.NativeHandleField = "ps6109misbound.recorder.handle"
	contract.MutableStateFields = []string{"ps6109misbound.recorder.state"}
	contract.AllowedSynchronousUses = []config.ReusableOneShotMethod{{Static: "ps6109misbound.recorder.Encode", Concrete: "ps6109misbound.recorder.Encode"}}
	return contract
}

func ps6109ImportedContract() config.ReusableOneShotWrapperContract {
	contract := ps6109TestContract()
	contract.Name = "imported-recorder-shell"
	contract.WrapperType = "ps6109backend.Recorder"
	contract.ProviderType = "ps6109backend.Device"
	contract.Acquisition = "ps6109backend.CommandFactory.Acquire"
	contract.ConcreteAcquisition = "ps6109backend.Device.Acquire"
	contract.WrapperConstructor = "ps6109backend.NewRecorder"
	contract.Terminal = config.ReusableOneShotMethod{Static: "ps6109backend.CommandRecorder.Free", Concrete: "ps6109backend.Recorder.Free"}
	contract.Reset = config.ReusableOneShotMethod{Static: "ps6109backend.CommandRecorder.Reset", Concrete: "ps6109backend.Recorder.Reset"}
	contract.FreshNativeHandleFactory = "ps6109backend.Device.Fresh"
	contract.NativeHandleField = "ps6109backend.Recorder.Handle"
	contract.MutableStateFields = []string{"ps6109backend.Recorder.State"}
	contract.AllowedSynchronousUses = []config.ReusableOneShotMethod{{Static: "ps6109backend.CommandRecorder.Encode", Concrete: "ps6109backend.Recorder.Encode"}}
	return contract
}

package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6087(t *testing.T) {
	t.Parallel()
	contract := func(name, producer, activation, binary, capability, method string) config.InPlaceFusionContract {
		return config.InPlaceFusionContract{
			Name:                         name,
			Producer:                     producer,
			Activation:                   activation,
			ActivationInputArg:           0,
			Binary:                       binary,
			BinaryActivationArg:          0,
			BinaryOtherArg:               1,
			CapabilityInterface:          capability,
			CapabilityMethod:             method,
			NonRecordingGuard:            "ps6087cap.Executor.IsEager",
			ProducerReturnsFreshOwned:    true,
			ActivationReturnsFreshOwned:  true,
			BinaryReturnsFreshOwned:      true,
			CapabilityOverwritesFirstArg: true,
			CapabilityPreservesSecondArg: true,
			CapabilityRejectsUnsupported: true,
			CapabilityFailureUnmodified:  true,
			CapabilityMatchesComposition: true,
			GuardProvesNonRecording:      true,
		}
	}
	concrete := contract("concrete-incapable", "ps6087cap.Linear.Forward", "ps6087cap.ConcreteOps.SiLU", "ps6087cap.ConcreteOps.Mul", "ps6087cap.SwiGLUInPlaceFuser", "FuseSwiGLUInPlace")
	concrete.NonRecordingGuard = "ps6087cap.ConcreteOps.IsEager"
	contracts := []config.InPlaceFusionContract{
		contract("swiglu", "ps6087cap.Linear.Forward", "ps6087cap.Executor.SiLU", "ps6087cap.Executor.Mul", "ps6087cap.SwiGLUInPlaceFuser", "FuseSwiGLUInPlace"),
		contract("pair-producer", "ps6087cap.Linear.ForwardPair", "ps6087cap.Executor.SiLU", "ps6087cap.Executor.Mul", "ps6087cap.SwiGLUInPlaceFuser", "FuseSwiGLUInPlace"),
		contract("pair-activation", "ps6087cap.Linear.Forward", "ps6087cap.Executor.SiLUPair", "ps6087cap.Executor.Mul", "ps6087cap.SwiGLUInPlaceFuser", "FuseSwiGLUInPlace"),
		contract("pair-binary", "ps6087cap.Linear.Forward", "ps6087cap.Executor.SiLU", "ps6087cap.Executor.MulPair", "ps6087cap.SwiGLUInPlaceFuser", "FuseSwiGLUInPlace"),
		contract("activation-extra-arg", "ps6087cap.Linear.Forward", "ps6087cap.Executor.SiLUWithMode", "ps6087cap.Executor.Mul", "ps6087cap.SwiGLUInPlaceFuser", "FuseSwiGLUInPlace"),
		contract("binary-extra-arg", "ps6087cap.Linear.Forward", "ps6087cap.Executor.SiLU", "ps6087cap.Executor.MulScaled", "ps6087cap.SwiGLUInPlaceFuser", "FuseSwiGLUInPlace"),
		concrete,
		contract("copy-into-any", "ps6087cap.Linear.Forward", "ps6087cap.Executor.SiLU", "ps6087cap.Executor.Mul", "ps6087cap.CopyIntoCapability", "CopyInto"),
		contract("no-parameter-capability", "ps6087cap.Linear.Forward", "ps6087cap.Executor.SiLU", "ps6087cap.Executor.Mul", "ps6087cap.NoParameterCapability", "Fuse"),
		contract("no-result-capability", "ps6087cap.Linear.Forward", "ps6087cap.Executor.SiLU", "ps6087cap.Executor.Mul", "ps6087cap.NoResultCapability", "Fuse"),
		contract("constraint-only-capability", "ps6087cap.Linear.Forward", "ps6087cap.Executor.SiLU", "ps6087cap.Executor.Mul", "ps6087cap.ConstraintOnlyCapability", "FuseSwiGLUInPlace"),
		contract("generic-capability", "ps6087cap.Linear.Forward", "ps6087cap.Executor.SiLU", "ps6087cap.Executor.Mul", "ps6087cap.GenericCapability", "FuseSwiGLUInPlace"),
		contract("unrelated-import", "ps6087cap.Linear.Forward", "ps6087cap.Executor.SiLU", "ps6087cap.Executor.Mul", "ps6087other.UnrelatedInPlace", "Overwrite"),
	}
	analyzer := *PS6087.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6087WithContracts(pass, contracts)
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6087", "ps6087transitive")
}

func TestPS6087ValidContractRequiresSemanticPromises(t *testing.T) {
	t.Parallel()
	valid := config.InPlaceFusionContract{
		Name:                         "swiglu",
		Producer:                     "example.com/tensor.Linear.Forward",
		Activation:                   "example.com/backend.Executor.SiLU",
		ActivationInputArg:           0,
		Binary:                       "example.com/backend.Executor.Mul",
		BinaryActivationArg:          0,
		BinaryOtherArg:               1,
		CapabilityInterface:          "example.com/backend.SwiGLUInPlaceFuser",
		CapabilityMethod:             "FuseSwiGLUInPlace",
		NonRecordingGuard:            "example.com/backend.Executor.IsEager",
		ProducerReturnsFreshOwned:    true,
		ActivationReturnsFreshOwned:  true,
		BinaryReturnsFreshOwned:      true,
		CapabilityOverwritesFirstArg: true,
		CapabilityPreservesSecondArg: true,
		CapabilityRejectsUnsupported: true,
		CapabilityFailureUnmodified:  true,
		CapabilityMatchesComposition: true,
		GuardProvesNonRecording:      true,
	}
	if !ps6087ValidContract(&valid) {
		t.Fatal("fully affirmed contract must be valid")
	}
	tests := map[string]func(*config.InPlaceFusionContract){
		"producer ownership":      func(c *config.InPlaceFusionContract) { c.ProducerReturnsFreshOwned = false },
		"activation ownership":    func(c *config.InPlaceFusionContract) { c.ActivationReturnsFreshOwned = false },
		"binary ownership":        func(c *config.InPlaceFusionContract) { c.BinaryReturnsFreshOwned = false },
		"overwrite first":         func(c *config.InPlaceFusionContract) { c.CapabilityOverwritesFirstArg = false },
		"preserve second":         func(c *config.InPlaceFusionContract) { c.CapabilityPreservesSecondArg = false },
		"reject unsupported":      func(c *config.InPlaceFusionContract) { c.CapabilityRejectsUnsupported = false },
		"failure unchanged":       func(c *config.InPlaceFusionContract) { c.CapabilityFailureUnmodified = false },
		"composition equivalence": func(c *config.InPlaceFusionContract) { c.CapabilityMatchesComposition = false },
		"non-recording guard":     func(c *config.InPlaceFusionContract) { c.GuardProvesNonRecording = false },
	}
	for name, invalidate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			invalidate(&candidate)
			if ps6087ValidContract(&candidate) {
				t.Fatal("contract missing semantic promise must be invalid")
			}
		})
	}
}

package config

import (
	"encoding/json"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUnusedProjectionScratchConfigRoundTrip(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"yaml", "json"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			original := Config{UnusedProjectionScratchContracts: []UnusedProjectionScratchContract{unusedProjectionScratchContractForTest()}}
			marshal, unmarshal := yaml.Marshal, yaml.Unmarshal
			if format == "json" {
				marshal, unmarshal = json.Marshal, json.Unmarshal
			}
			data, err := marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			var decoded Config
			if err := unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decoded.UnusedProjectionScratchContracts, original.UnusedProjectionScratchContracts) || UsableUnusedProjectionScratchContractCount(decoded.UnusedProjectionScratchContracts) != 1 {
				t.Fatal("typed vocabulary lost in configuration round trip")
			}
			compiled := decoded.Compile()
			compiled.UnusedProjectionScratchContracts[0].Projections[0].Field = "changed"
			compiled.UnusedProjectionScratchContracts[0].ClassFlags[0] = "changed"
			if !reflect.DeepEqual(decoded.UnusedProjectionScratchContracts, original.UnusedProjectionScratchContracts) {
				t.Fatal("compiled vocabulary aliases source config slices")
			}
		})
	}
}

func TestUnusedProjectionScratchUsableClasses(t *testing.T) {
	t.Parallel()
	c := unusedProjectionScratchContractForTest()
	if UsableUnusedProjectionScratchContractCount(nil) != 0 || UsableUnusedProjectionScratchContractCount([]UnusedProjectionScratchContract{c, c}) != 0 {
		t.Fatal("missing or duplicate class vocabulary usable")
	}
	other := c
	other.ConstructorEntry = "project.NewOther"
	if UsableUnusedProjectionScratchContractCount([]UnusedProjectionScratchContract{c, other}) != 2 {
		t.Fatal("distinct constructor classes at one workspace were conflated")
	}
	other = c
	other.NativeAllocationCountUnit = "float32-elements"
	if UsableUnusedProjectionScratchContractCount([]UnusedProjectionScratchContract{c, other}) != 0 {
		t.Fatal("contradictory native metadata at the same class admitted")
	}
}

func TestUnusedProjectionScratchExampleRequiresIndependentReview(t *testing.T) {
	t.Parallel()
	cfg, err := Load("../docs/perfscan.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.UnusedProjectionScratchContracts) != 1 || UsableUnusedProjectionScratchContractCount(cfg.UnusedProjectionScratchContracts) != 0 {
		t.Fatal("generic example must document one disabled scratch contract")
	}
	c := cfg.UnusedProjectionScratchContracts[0]
	if c.FreshIndependentFloat32StorageReviewed || c.NativeFailureOwnershipReviewed || c.NativeInputCopyAndAliasesReviewed ||
		c.NativeReleaseAndFinalizerReviewed || c.SequentialCompletionReviewed || c.PositiveCheckedSourceGeometryReviewed ||
		c.NativeByteCountRangeReviewed || c.ExternalOwnerObservationsReviewed || c.AllProviderBuildPathsReviewed {
		t.Fatal("generic example asserts an independent review was already completed")
	}
	// Check the template's typed roles and units independently of those gates.
	c.FreshIndependentFloat32StorageReviewed = true
	c.NativeFailureOwnershipReviewed = true
	c.NativeInputCopyAndAliasesReviewed = true
	c.NativeReleaseAndFinalizerReviewed = true
	c.SequentialCompletionReviewed = true
	c.PositiveCheckedSourceGeometryReviewed = true
	c.NativeByteCountRangeReviewed = true
	c.ExternalOwnerObservationsReviewed = true
	c.AllProviderBuildPathsReviewed = true
	if !c.Valid() {
		t.Fatal("documented typed roles, geometry and native units are invalid")
	}
}

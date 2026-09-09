package runner

import (
	"testing"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

func TestMissingVocabStateExpandedLookupContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"stateExpandedLookupContracts"}}
	tests := []struct {
		name        string
		config      config.Config
		wantMissing bool
	}{
		{name: "empty", wantMissing: true},
		{name: "invalid", config: config.Config{StateExpandedLookupContracts: []config.StateExpandedLookupContract{{Name: "incomplete"}}}, wantMissing: true},
		{name: "valid", config: config.Config{StateExpandedLookupContracts: []config.StateExpandedLookupContract{stateExpandedLookupRunnerContract()}}},
		{name: "duplicate claim", config: config.Config{StateExpandedLookupContracts: stateExpandedLookupDuplicateClaim()}, wantMissing: true},
		{name: "duplicate name", config: config.Config{StateExpandedLookupContracts: stateExpandedLookupDuplicateName()}, wantMissing: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := missingVocab(check, &test.config)
			if test.wantMissing {
				if len(got) != 1 || got[0] != "stateExpandedLookupContracts" {
					t.Fatalf("missingVocab() = %v, want stateExpandedLookupContracts", got)
				}
				return
			}
			if len(got) != 0 {
				t.Fatalf("missingVocab() = %v, want none", got)
			}
		})
	}
}

func stateExpandedLookupRunnerContract() config.StateExpandedLookupContract {
	return config.StateExpandedLookupContract{
		Name:                                  "decode-grid",
		OwnerSite:                             "example.com/codec.decodeRow",
		OwnerKind:                             config.StateExpandedLookupFunction,
		TableObject:                           "example.com/codec.grid",
		MaxStateCardinality:                   2,
		MaxExpandedBytes:                      131072,
		HotPathEvidence:                       true,
		TableImmutableAfterInitialization:     true,
		InitializationReproducesExactDType:    true,
		InitializationOrderValidationRequired: true,
		CacheFootprintReviewRequired:          true,
		ExactOutputValidationRequired:         true,
		PairedBenchmarkRequired:               true,
	}
}

func stateExpandedLookupDuplicateClaim() []config.StateExpandedLookupContract {
	first := stateExpandedLookupRunnerContract()
	second := first
	second.Name = "other-name"
	return []config.StateExpandedLookupContract{first, second}
}

func stateExpandedLookupDuplicateName() []config.StateExpandedLookupContract {
	first := stateExpandedLookupRunnerContract()
	second := first
	second.OwnerSite = "example.com/codec.otherDecodeRow"
	second.TableObject = "example.com/codec.otherGrid"
	return []config.StateExpandedLookupContract{first, second}
}

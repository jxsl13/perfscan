package config

import (
	"encoding/json"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestStateExpandedLookupContractValid(t *testing.T) {
	t.Parallel()
	contract := stateExpandedLookupTestContract()
	if !contract.Valid() {
		t.Fatal("complete state-expanded lookup contract is invalid")
	}

	tests := []struct {
		name   string
		mutate func(*StateExpandedLookupContract)
	}{
		{"nil contract", nil},
		{"blank name", func(c *StateExpandedLookupContract) { c.Name = "" }},
		{"padded name", func(c *StateExpandedLookupContract) { c.Name = " lookup" }},
		{"control in name", func(c *StateExpandedLookupContract) { c.Name = "look\nup" }},
		{"short owner", func(c *StateExpandedLookupContract) { c.OwnerSite = "Hot" }},
		{"wrong owner kind", func(c *StateExpandedLookupContract) { c.OwnerKind = "closure" }},
		{"function parsed as method", func(c *StateExpandedLookupContract) { c.OwnerKind = StateExpandedLookupMethod }},
		{"short table", func(c *StateExpandedLookupContract) { c.TableObject = "grid" }},
		{"one state", func(c *StateExpandedLookupContract) { c.MaxStateCardinality = 1 }},
		{"too many states", func(c *StateExpandedLookupContract) { c.MaxStateCardinality = 9 }},
		{"zero bytes", func(c *StateExpandedLookupContract) { c.MaxExpandedBytes = 0 }},
		{"oversized budget", func(c *StateExpandedLookupContract) { c.MaxExpandedBytes = MaxStateExpandedLookupBytes + 1 }},
		{"no hot evidence", func(c *StateExpandedLookupContract) { c.HotPathEvidence = false }},
		{"mutable table", func(c *StateExpandedLookupContract) { c.TableImmutableAfterInitialization = false }},
		{"dtype gap", func(c *StateExpandedLookupContract) { c.InitializationReproducesExactDType = false }},
		{"init-order gap", func(c *StateExpandedLookupContract) { c.InitializationOrderValidationRequired = false }},
		{"cache review gap", func(c *StateExpandedLookupContract) { c.CacheFootprintReviewRequired = false }},
		{"output validation gap", func(c *StateExpandedLookupContract) { c.ExactOutputValidationRequired = false }},
		{"benchmark gap", func(c *StateExpandedLookupContract) { c.PairedBenchmarkRequired = false }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if test.mutate == nil {
				var candidate *StateExpandedLookupContract
				if candidate.Valid() {
					t.Fatal("nil state-expanded lookup contract is valid")
				}
				return
			}
			candidate := contract
			test.mutate(&candidate)
			if candidate.Valid() {
				t.Fatal("incomplete state-expanded lookup contract is valid")
			}
		})
	}
}

func TestStateExpandedLookupContractBoundaryValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		cardinality int
		bytes       int64
	}{
		{"minimum", 2, 1},
		{"maximum", 8, MaxStateExpandedLookupBytes},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			contract := stateExpandedLookupTestContract()
			contract.MaxStateCardinality = test.cardinality
			contract.MaxExpandedBytes = test.bytes
			if !contract.Valid() {
				t.Fatal("boundary state-expanded lookup contract is invalid")
			}
		})
	}
}

func TestStateExpandedLookupMethodContractValid(t *testing.T) {
	t.Parallel()
	contract := stateExpandedLookupTestContract()
	contract.OwnerSite = "example.com/p.Decoder.Hot"
	contract.OwnerKind = StateExpandedLookupMethod
	if !contract.Valid() {
		t.Fatal("complete method-owned state-expanded lookup contract is invalid")
	}
}

func TestUsableStateExpandedLookupContractsRejectAmbiguity(t *testing.T) {
	t.Parallel()
	contract := stateExpandedLookupTestContract()
	if got := UsableStateExpandedLookupContractCount([]StateExpandedLookupContract{contract}); got != 1 {
		t.Fatalf("usable contract count = %d, want 1", got)
	}
	tests := []struct {
		name      string
		duplicate StateExpandedLookupContract
	}{
		{"name", func() StateExpandedLookupContract {
			duplicate := contract
			duplicate.OwnerSite = "example.com/p.Other"
			return duplicate
		}()},
		{"claim", func() StateExpandedLookupContract {
			duplicate := contract
			duplicate.Name = "other"
			return duplicate
		}()},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := UsableStateExpandedLookupContractCount([]StateExpandedLookupContract{contract, test.duplicate}); got != 0 {
				t.Fatalf("ambiguous contract count = %d, want 0", got)
			}
		})
	}
}

func TestUsableStateExpandedLookupContractsRetainIndependentContract(t *testing.T) {
	t.Parallel()
	contract := stateExpandedLookupTestContract()
	duplicate := contract
	duplicate.Name = "duplicate-claim"
	independent := contract
	independent.Name = "independent"
	independent.OwnerSite = "example.com/p.Other"
	if got := UsableStateExpandedLookupContractCount([]StateExpandedLookupContract{contract, duplicate, independent}); got != 1 {
		t.Fatalf("usable contract count = %d, want 1", got)
	}
}

func TestCompileClonesStateExpandedLookupContracts(t *testing.T) {
	t.Parallel()
	configuration := Config{StateExpandedLookupContracts: []StateExpandedLookupContract{stateExpandedLookupTestContract()}}
	compiled := configuration.Compile()
	configuration.StateExpandedLookupContracts[0].Name = "mutated"
	if got := compiled.StateExpandedLookupContracts[0].Name; got != "owner-grid" {
		t.Fatalf("compiled contract changed with source: %q", got)
	}
	compiled.StateExpandedLookupContracts[0].OwnerSite = "example.com/p.Other"
	if got := configuration.StateExpandedLookupContracts[0].OwnerSite; got != "example.com/p.Hot" {
		t.Fatalf("source contract changed with compiled view: %q", got)
	}
}

func TestStateExpandedLookupContractWireKeys(t *testing.T) {
	t.Parallel()
	want := []string{
		"cacheFootprintReviewRequired",
		"exactOutputValidationRequired",
		"hotPathEvidence",
		"initializationOrderValidationRequired",
		"initializationReproducesExactDtype",
		"maxExpandedBytes",
		"maxStateCardinality",
		"name",
		"ownerKind",
		"ownerSite",
		"pairedBenchmarkRequired",
		"tableImmutableAfterInitialization",
		"tableObject",
	}
	tests := []struct {
		name      string
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{
		{"json", json.Marshal, json.Unmarshal},
		{"yaml", yaml.Marshal, yaml.Unmarshal},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := test.marshal(Config{StateExpandedLookupContracts: []StateExpandedLookupContract{stateExpandedLookupTestContract()}})
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := test.unmarshal(encoded, &document); err != nil {
				t.Fatal(err)
			}
			contracts, ok := document["stateExpandedLookupContracts"].([]any)
			if !ok || len(contracts) != 1 {
				t.Fatalf("wire contract collection = %#v", document["stateExpandedLookupContracts"])
			}
			contract, ok := contracts[0].(map[string]any)
			if !ok {
				t.Fatalf("wire contract = %#v", contracts[0])
			}
			got := make([]string, 0, len(contract))
			for key := range contract {
				got = append(got, key)
			}
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Fatalf("contract keys = %v, want %v", got, want)
			}
		})
	}
}

func TestExampleConfigHasValidStateExpandedLookupContract(t *testing.T) {
	t.Parallel()
	configuration, err := Load("../docs/perfscan.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(configuration.StateExpandedLookupContracts) != 1 ||
		!configuration.StateExpandedLookupContracts[0].Valid() {
		t.Fatalf("example state-expanded lookup contract is incomplete: %+v", configuration.StateExpandedLookupContracts)
	}
}

func stateExpandedLookupTestContract() StateExpandedLookupContract {
	return StateExpandedLookupContract{
		Name:                                  "owner-grid",
		OwnerSite:                             "example.com/p.Hot",
		OwnerKind:                             StateExpandedLookupFunction,
		TableObject:                           "example.com/p.grid",
		MaxStateCardinality:                   2,
		MaxExpandedBytes:                      128 << 10,
		HotPathEvidence:                       true,
		TableImmutableAfterInitialization:     true,
		InitializationReproducesExactDType:    true,
		InitializationOrderValidationRequired: true,
		CacheFootprintReviewRequired:          true,
		ExactOutputValidationRequired:         true,
		PairedBenchmarkRequired:               true,
	}
}

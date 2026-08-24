package runner

import (
	"testing"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

func TestMissingVocabSelectorPromotionSymbols(t *testing.T) {
	t.Parallel()
	check := &lint.Check{
		NeedsConfig: true,
		Vocab:       []string{"selectorPromotionSymbols"},
	}
	empty := config.Config{}
	if got := missingVocab(check, &empty); len(got) != 1 || got[0] != "selectorPromotionSymbols" {
		t.Fatalf("missingVocab(empty) = %v, want selectorPromotionSymbols", got)
	}
	configured := config.Config{SelectorPromotionSymbols: []string{"useSplitK"}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(configured) = %v, want none", got)
	}
}

func TestMissingVocabInPlaceFusionContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"inPlaceFusionContracts"}}
	empty := config.Config{}
	if got := missingVocab(check, &empty); len(got) != 1 || got[0] != "inPlaceFusionContracts" {
		t.Fatalf("missingVocab(empty) = %v, want inPlaceFusionContracts", got)
	}
	configured := config.Config{InPlaceFusionContracts: []config.InPlaceFusionContract{{Name: "swiglu"}}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(configured) = %v, want none", got)
	}
}

func TestMissingVocabTopKOneContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"topKOneContracts"}}
	empty := config.Config{}
	if got := missingVocab(check, &empty); len(got) != 1 || got[0] != "topKOneContracts" {
		t.Fatalf("missingVocab(empty) = %v, want topKOneContracts", got)
	}
	for _, contract := range []config.TopKOneContract{
		{Name: "invalid import path", Function: "example.com/a:bad.TopKN", KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "ambiguous without kind", Function: "example.com/project.Buffer.TopKN", KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "blank function", Function: "example.com/project._", Kind: config.TopKOneContractFunction, KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "blank receiver", Function: "example.com/project._.TopKN", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "blank method", Function: "example.com/project.Buffer._", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
	} {
		contract := contract
		t.Run(contract.Name, func(t *testing.T) {
			t.Parallel()
			invalid := config.Config{TopKOneContracts: []config.TopKOneContract{contract}}
			if got := missingVocab(check, &invalid); len(got) != 1 || got[0] != "topKOneContracts" {
				t.Fatalf("missingVocab(invalid-only) = %v, want topKOneContracts", got)
			}
		})
	}
	configured := config.Config{TopKOneContracts: []config.TopKOneContract{
		{Name: "blank sibling", Function: "example.com/project._", Kind: config.TopKOneContractFunction, KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "resident", Function: "example.com/dotted.pkg.Búffer.TópKN", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
	}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(mixed valid/invalid) = %v, want none", got)
	}
}

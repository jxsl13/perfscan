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

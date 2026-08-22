package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2013 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the rewrite forms (verbatim s
// expressions, named constants, concatenated constant pairs, parenthesized
// receivers, nested matches, an aliased strings import that must keep its
// alias). See equiv_PS2013_test.go for the runtime proof that the rewrite
// is byte-identical for every non-empty constant old.
func TestPS2013(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2013.Analyzer, "ps2013")
}

// A comment anywhere in the replaced punctuation (between the constructor
// arguments, before .Replace) would be silently destroyed by the rewrite,
// so the fix is withheld and the report stays advisory — the golden is
// identical to the source.
func TestPS2013Advisory(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2013.Analyzer, "ps2013adv")
}

// Negative shapes — empty old (the one divergent input), multi-pair or
// runtime-value replacers (PS2132's territory), WriteString, a stored
// replacer, fake NewReplacer methods, a dot import — must produce no
// diagnostics at all.
func TestPS2013Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2013.Analyzer, "ps2013neg")
}

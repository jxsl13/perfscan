package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5049 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. The suite covers all three twins (Sprintf->Appendf with
// args, format-only, and an inner (args...) spread preserved; verbless
// Sprint->Append incl. the zero-arg form; Sprintln->Appendln) plus a
// side-effecting destination expression. Advisory cases pin the guards: a
// NAMED byte-slice destination and a comment inside the scaffolding. Negatives
// stay SILENT: a non-spread append (a single []string element), a
// parenthesized inner call, and a non-fmt spread. See equiv_PS5049_test.go for
// the byte-identity proof.
func TestPS5049(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5049.Analyzer, "ps5049")
}

// TestPS5049Alias pins that an aliased fmt import is carried through the
// rewrite (f.Sprintf -> f.Appendf), never a hard-coded "fmt".
func TestPS5049Alias(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5049.Analyzer, "ps5049alias")
}

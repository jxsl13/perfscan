package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS2002(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS2002.Analyzer, "ps2002")
}

func TestPS2005(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2005.Analyzer, "ps2005")
}

func TestPS2101(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2101.Analyzer, "ps2101")
}

func TestPS2003(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS2003.Analyzer, "ps2003")
}

func TestPS2008(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS2008.Analyzer, "ps2008")
}

func TestPS3007(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS3007.Analyzer, "ps3007")
}

func TestPS3077(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS3077.Analyzer, "ps3077")
}

func TestPS3082(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS3082.Analyzer, "ps3082")
}

func TestPS3001(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS3001.Analyzer, "ps3001")
}

func TestPS3002(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS3002.Analyzer, "ps3002")
}

func TestPS3003(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS3003.Analyzer, "ps3003")
}

// goaiVocab mirrors the goai reference vocabulary used by domain fixtures.
var goaiVocab = config.Config{
	ElementAccessors:    []string{"AtF64", "SetF64"},
	FastPathHelpers:     []string{"flatF64", "flatF32"},
	ElementCountMethods: []string{"Numel"},
	ShapeMethods:        []string{"Shape"},
	IndexDecomposeFuncs: []string{"Unravel"},
	PerElementVisitors:  []string{"readGen", "fillGen"},
}

func TestPS1001(t *testing.T) {
	defer config.SetForTesting(goaiVocab)()
	analysistest.Run(t, analysistest.TestData(), PS1001.Analyzer, "ps1001")
}

func TestPS1002(t *testing.T) {
	defer config.SetForTesting(goaiVocab)()
	analysistest.Run(t, analysistest.TestData(), PS1002.Analyzer, "ps1002")
}

func TestPS1003(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS1003.Analyzer, "ps1003")
}

func TestPS1005(t *testing.T) {
	defer config.SetForTesting(goaiVocab)()
	analysistest.Run(t, analysistest.TestData(), PS1005.Analyzer, "ps1005")
}

func TestPS2001(t *testing.T) {
	restore := config.SetForTesting(config.Config{
		AllocatorFuncs: []string{"New", "Zeros"},
	})
	defer restore()
	analysistest.Run(t, analysistest.TestData(), PS2001.Analyzer, "ps2001")
}

func TestPS2001SilentWithoutVocabulary(t *testing.T) {
	restore := config.SetForTesting(config.Config{})
	defer restore()
	// The fixture contains findings; with an empty vocabulary the check
	// must stay silent. analysistest would fail on unexpected
	// diagnostics, so running against a fixture without want-comments
	// verifies silence.
	analysistest.Run(t, analysistest.TestData(), PS2001.Analyzer, "ps2001silent")
}

func TestRegistryInvariant(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range All() {
		if seen[c.ID] {
			t.Fatalf("duplicate ID %s", c.ID)
		}
		seen[c.ID] = true
		if c.Analyzer == nil || c.Analyzer.Name != c.ID {
			t.Fatalf("check %s: analyzer name mismatch", c.ID)
		}
		if c.Doc.Title == "" {
			t.Fatalf("check %s: missing doc title", c.ID)
		}
		if c.Level < 1 || c.Level > 3 {
			t.Fatalf("check %s: invalid level %d", c.ID, c.Level)
		}
		if c.NeedsConfig && len(c.Vocab) == 0 {
			t.Fatalf("check %s: NeedsConfig without Vocab", c.ID)
		}
	}
}

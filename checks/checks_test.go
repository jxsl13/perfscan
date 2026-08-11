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

func TestPS2103(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS2103.Analyzer, "ps2103")
}

func TestPS4008(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS4008.Analyzer, "ps4008")
}

func TestPS5001(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS5001.Analyzer, "ps5001")
}

func TestPS5002(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS5002.Analyzer, "ps5002")
}

func TestPS2102(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS2102.Analyzer, "ps2102")
}

func TestPS2104(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2104.Analyzer, "ps2104")
}

func TestPS3101(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS3101.Analyzer, "ps3101")
}

func TestPS4101(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS4101.Analyzer, "ps4101")
}

func TestPS2004(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS2004.Analyzer, "ps2004")
}

func TestPS2007(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS2007.Analyzer, "ps2007")
}

func TestPS3005(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS3005.Analyzer, "ps3005")
}

func TestPS2006(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS2006.Analyzer, "ps2006")
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

func TestPS3034(t *testing.T) {
	defer config.SetForTesting(config.Config{FanOutHelpers: []string{"parallelFor"}})()
	analysistest.Run(t, analysistest.TestData(), PS3034.Analyzer, "ps3034")
}

func TestPS3059(t *testing.T) {
	defer config.SetForTesting(config.Config{FanOutHelpers: []string{"parallelFor"}})()
	analysistest.Run(t, analysistest.TestData(), PS3059.Analyzer, "ps3059")
}

func TestPS3063(t *testing.T) {
	defer config.SetForTesting(config.Config{FanOutHelpers: []string{"parallelFor"}})()
	analysistest.Run(t, analysistest.TestData(), PS3063.Analyzer, "ps3063")
}

func TestPS3065(t *testing.T) {
	defer config.SetForTesting(config.Config{FanOutHelpers: []string{"parallelFor"}})()
	analysistest.Run(t, analysistest.TestData(), PS3065.Analyzer, "ps3065")
}

func TestPS3066(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS3066.Analyzer, "ps3066")
}

func TestPS3071(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS3071.Analyzer, "ps3071")
}

func TestPS3083(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS3083.Analyzer, "ps3083")
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

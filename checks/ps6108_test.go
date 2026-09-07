package checks

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6108(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6108.Analyzer, "ps6108")
}

func TestPS6108DeterministicAdvisory(t *testing.T) {
	t.Parallel()
	run := func() []string {
		results := analysistest.Run(t, analysistest.TestData(), PS6108.Analyzer, "ps6108")
		var signatures []string
		for _, result := range results {
			for _, diagnostic := range result.Diagnostics {
				if len(diagnostic.SuggestedFixes) != 0 {
					t.Fatalf("PS6108 diagnostic unexpectedly has %d suggested fixes", len(diagnostic.SuggestedFixes))
				}
				position := result.Pass.Fset.Position(diagnostic.Pos)
				signatures = append(signatures, fmt.Sprintf("%s:%d:%d:%s", filepath.Base(position.Filename), position.Line, position.Column, diagnostic.Message))
			}
		}
		return signatures
	}
	first, second := run(), run()
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("PS6108 diagnostics are not deterministic:\nfirst:  %q\nsecond: %q", first, second)
	}
}

func TestPS6108Metadata(t *testing.T) {
	t.Parallel()
	if PS6108.AutoFix {
		t.Fatal("PS6108 must remain advisory")
	}
	if PS6108.Level != 3 || PS6108.Category != "verify" {
		t.Fatalf("PS6108 metadata = level %d category %q", PS6108.Level, PS6108.Category)
	}
}

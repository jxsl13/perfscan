package checks

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2004CgoCrossingCaveat(t *testing.T) {
	t.Parallel()
	results := analysistest.Run(t, analysistest.TestData(), PS2004.Analyzer, "ps2004", "ps2004cgo")
	confirmed := 0
	advisory := 0
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			if len(diagnostic.SuggestedFixes) == 0 {
				advisory++
				if strings.Contains(diagnostic.Message, "this leaves C.") {
					t.Errorf("unproven scratch shape received the confirmed native-call message: %s", diagnostic.Message)
				}
				continue
			}
			confirmed++
			for _, required := range []string{
				"autofix preserves the local method-call lifetime",
				"this leaves C.fill_label inside the record loop",
				"fewer allocations do not imply proportional latency gains",
				"measure the reused-scratch control",
				"semantics-preserving bulk ABI or immutable snapshot",
				"separate advisory, no bulk autofix",
			} {
				if !strings.Contains(diagnostic.Message, required) {
					t.Errorf("confirmed producer message lacks %q: %s", required, diagnostic.Message)
				}
			}
			if len(diagnostic.SuggestedFixes) != 1 || len(diagnostic.SuggestedFixes[0].TextEdits) != 2 {
				t.Errorf("the message must preserve the existing two-edit scratch hoist: %#v", diagnostic.SuggestedFixes)
			}
		}
	}
	if confirmed != 2 || advisory == 0 {
		t.Fatalf("expected both existing cgo hoists and advisory negative controls, got %d hoists and %d advisories", confirmed, advisory)
	}
}

package checks

import (
	"strings"
	"testing"
)

func TestPS6128MacroOperandExpansion(t *testing.T) {
	t.Parallel()
	changed := strings.Replace(ps6128ReviewAsm, " MOVD $0x5000000000000000, R16", " MOVD $0, R5\n#define R5 R4\n MOVD $0x5000000000000000, R16", 1)
	if changed == ps6128ReviewAsm {
		t.Fatal("derivation did not apply")
	}
	for _, tc := range []struct {
		name, assembly string
		finding        bool
	}{
		{"unchanged_live_source", ps6128ReviewAsm, true},
		{"active_register_macro_retargets_descriptor_away_from_actual_operand", changed, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ps6128ReviewRun(t, ps6128ReviewBase, tc.assembly, tc.finding)
		})
	}
}

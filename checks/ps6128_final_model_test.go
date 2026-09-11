package checks

import (
	"strings"
	"testing"
)

func TestPS6128FinalSourceModelPermanent(t *testing.T) {
	t.Parallel()
	replace := func(t *testing.T, source, old, next string) string {
		t.Helper()
		if strings.Count(source, old) != 1 {
			t.Fatalf("derivation %q not unique", old)
		}
		return strings.Replace(source, old, next, 1)
	}
	rawClear := replace(t, ps6128ReviewAsm, "#define AMX_LDY_R5", "#define CLEAR_R5 WORD $0xd2800005\n#define AMX_LDY_R5")
	rawClear = replace(t, rawClear, " LSR $2", " CLEAR_R5\n LSR $2")
	laterMacro := replace(t, ps6128ReviewAsm, "#define AMX_LDY_R5 WORD $0x00201025", "#define AMX_LDY_R5 WORD $0xd503201f") + "\n#undef AMX_LDY_R5\n#define AMX_LDY_R5 WORD $0x00201025\n"
	twoArgument := replace(t, replace(t, ps6128ReviewBase, "tile(k) })", "tile(0,k) })"), "func tile(k int)", "func tile(unrelated,k int)")
	for _, tc := range []struct {
		name, source, assembly string
		finding                bool
		nativeArgument         int
	}{
		{"live_reviewed_chain", ps6128ReviewBase, ps6128ReviewAsm, true, 0},
		{"pair_only_control", ps6128ReviewBase, replace(t, ps6128ReviewAsm, "0x5000000000000000", "0x4000000000000000"), false, 0},
		{"masked_dispatch_dimension_is_always_tail_only", replace(t, ps6128ReviewBase, "gemm(k)", "gemm(k&3)"), ps6128ReviewAsm, false, 0},
		{"raw_word_macro_clears_live_descriptor_register", ps6128ReviewBase, rawClear, false, 0},
		{"later_macro_definition_cannot_change_earlier_invocation", ps6128ReviewBase, laterMacro, false, 0},
		{"configured_offset_must_agree_with_actual_parameter_layout", twoArgument, replace(t, ps6128ReviewAsm, "$0-8", "$0-16"), false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ps6128ReviewRunWithNativeArgument(t, tc.source, tc.assembly, tc.finding, tc.nativeArgument)
		})
	}
}

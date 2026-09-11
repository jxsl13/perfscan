package checks

import (
	"strings"
	"testing"
)

func TestPS6128SecondCompleteEffects(t *testing.T) {
	t.Parallel()
	replaceGo := func(old, next string) string { return strings.Replace(ps6128ReviewBase, old, next, 1) }
	replaceAsm := func(old, next string) string { return strings.Replace(ps6128ReviewAsm, old, next, 1) }
	for _, tc := range []struct {
		name, source, assembly string
		finding                bool
	}{
		{"live_control", ps6128ReviewBase, ps6128ReviewAsm, true},
		{"supported_brand_local_return", replaceGo(`return "Apple M1"`, `value:="Apple M2"; return value`), ps6128ReviewAsm, false},
		{"supported_brand_named_result", replaceGo(`func brandString() string { return "Apple M1" }`, `func brandString() (brand string) { brand="Apple M2"; return }`), ps6128ReviewAsm, false},
		{"constant_native_tail", replaceGo(`tile(k) })`, `tile(3) })`), ps6128ReviewAsm, false},
		{"overwritten_native_tail", replaceGo(`func compute(k int){`, `func compute(k int){ k=3;`), ps6128ReviewAsm, false},
		{"local_dispatch_tail", replaceGo(`func caller(k int){ gemm(k) }`, `func caller(k int){ count:=3; gemm(count) }`), ps6128ReviewAsm, false},
		{"declared_dispatch_tail", replaceGo(`func caller(k int){ gemm(k) }`, `func caller(k int){ var count=3; gemm(count) }`), ps6128ReviewAsm, false},
		{"later_write_does_not_reach_call", replaceGo(`func caller(k int){ gemm(k) }`, `func caller(k int){ count:=3; gemm(count); count=9; _=count }`), ps6128ReviewAsm, false},
		{"incremented_dispatch_argument_unknown", replaceGo(`func caller(k int){ gemm(k) }`, `func caller(k int){ count:=3; count++; gemm(count) }`), ps6128ReviewAsm, false},
		{"mutated_native_argument_unknown", replaceGo(`func compute(k int){`, `func compute(k int){ k++;`), ps6128ReviewAsm, false},
		{"descriptor_bic_clear", ps6128ReviewBase, replaceAsm(" LSR $2", " BIC $0x1000000000000000, R5, R5\n LSR $2"), false},
		{"descriptor_lsr_clear", ps6128ReviewBase, replaceAsm(" LSR $2", " LSR $63, R5, R5\n LSR $2"), false},
		{"zero_native_dimension", ps6128ReviewBase, replaceAsm(" LSR $2", " MOVD $0, R3\n LSR $2"), false},
		{"zero_branch_register", ps6128ReviewBase, replaceAsm(" CBZ R15, tail", " MOVD $0, R15\n CBZ R15, tail"), false},
		{"branch_before_gate_skips_load", ps6128ReviewBase, replaceAsm(" LSR $2", " B tail\n LSR $2"), false},
		{"defined_active_quad", ps6128ReviewBase, "#define ACTIVE_QUAD 1\n#ifdef ACTIVE_QUAD\n" + ps6128ReviewAsm + "#endif\n", true},
		{"active_else_quad", ps6128ReviewBase, "#ifdef UNDEFINED_QUAD\n" + replaceAsm("0x5000000000000000", "0x4000000000000000") + "#else\n" + ps6128ReviewAsm + "#endif\n", true},
		{"undefined_after_undef", ps6128ReviewBase, "#define ACTIVE_QUAD 1\n#undef ACTIVE_QUAD\n#ifdef ACTIVE_QUAD\n" + ps6128ReviewAsm + "#else\n" + replaceAsm("0x5000000000000000", "0x4000000000000000") + "#endif\n", false},
		{"pair_control", ps6128ReviewBase, replaceAsm("0x5000000000000000", "0x4000000000000000"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ps6128ReviewRun(t, tc.source, tc.assembly, tc.finding)
		})
	}
}

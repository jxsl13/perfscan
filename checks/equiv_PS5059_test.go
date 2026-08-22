package checks

import (
	"fmt"
	"math"
	"strconv"
	"testing"
)

// TestEquivPS5059 proves the rewrite is byte-identical: fmt.Appendf of one
// integer verb (%d/%x/%b/%o) spliced into literal text produces exactly what the
// nested append/strconv.AppendInt chain writes, across every width and the base
// extremes (including MinInt64, which FormatInt handles in uint64 magnitude
// space).
func TestEquivPS5059(t *testing.T) {
	seed := []byte("seed>")
	ci := func(format string, base int, n int64) {
		want := fmt.Appendf(append([]byte(nil), seed...), format, n)
		got := append(strconv.AppendInt(append(append([]byte(nil), seed...), pre(format)...), n, base), suf(format)...)
		if string(want) != string(got) {
			t.Fatalf("int %q n=%d: appendf=%q chain=%q", format, n, want, got)
		}
	}
	cu := func(format string, base int, n uint64) {
		want := fmt.Appendf(append([]byte(nil), seed...), format, n)
		got := append(strconv.AppendUint(append(append([]byte(nil), seed...), pre(format)...), n, base), suf(format)...)
		if string(want) != string(got) {
			t.Fatalf("uint %q n=%d: appendf=%q chain=%q", format, n, want, got)
		}
	}
	ints := []int64{0, 1, -1, 9, -9, 10, -10, 255, -256, math.MinInt64, math.MaxInt64}
	uints := []uint64{0, 1, 9, 255, 65535, math.MaxUint64}
	for _, f := range []struct {
		s string
		b int
	}{{"id=%d;", 10}, {"%d!", 10}, {"[%o]", 8}, {"0x%x", 16}, {"%b\n", 2}} {
		for _, n := range ints {
			ci(f.s, f.b, n)
		}
		for _, n := range uints {
			cu(f.s, f.b, n)
		}
	}
}

// pre/suf split "<prefix>%<verb><suffix>" — the single-verb shape PS5059 matches.
func pre(format string) string {
	for i := 0; i+1 < len(format); i++ {
		if format[i] == '%' {
			return format[:i]
		}
	}
	return format
}

func suf(format string) string {
	for i := 0; i+1 < len(format); i++ {
		if format[i] == '%' {
			return format[i+2:]
		}
	}
	return ""
}

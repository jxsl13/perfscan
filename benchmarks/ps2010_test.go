package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS2010 — bytes.Equal([]byte(s1), []byte(s2)) vs the direct s1 == s2.
// On current gc this demonstrates PARITY empirically (same policy as the
// gc-parity pairs PS2119/PS2125): escape analysis turns both conversions
// into zero-copy views because bytes.Equal neither retains nor mutates
// its arguments, so the Before allocates nothing either. The rewrite's
// win is that the After is allocation-free by construction — on
// toolchains without the zero-copy conversion (gccgo, tinygo, older gc)
// the Before pays one allocation and one full copy per conversion. One
// pair shares a long common prefix so the comparison does real work, and
// one pair differs in length so both forms short-circuit in O(1).
var (
	ps2010A = strings.Repeat("alpha beta gamma delta ", 128) + "x"
	ps2010B = strings.Repeat("alpha beta gamma delta ", 128) + "y"
	ps2010C = strings.Repeat("alpha beta gamma delta ", 128)
)

func BenchmarkPS2010_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		hits := 0
		if bytes.Equal([]byte(ps2010A), []byte(ps2010B)) {
			hits++
		}
		if bytes.Equal([]byte(ps2010A), []byte(ps2010C)) {
			hits++
		}
		sinkI = hits
	}
}

func BenchmarkPS2010_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		hits := 0
		if ps2010A == ps2010B {
			hits++
		}
		if ps2010A == ps2010C {
			hits++
		}
		sinkI = hits
	}
}

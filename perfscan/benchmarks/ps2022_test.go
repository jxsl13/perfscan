package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS2022 — bytes.Equal(b, []byte(s)) vs the direct string(b) == s.
// On current gc this demonstrates PARITY empirically (same policy as the
// gc-parity pairs PS2010/PS2119/PS2125): escape analysis turns []byte(s)
// into a zero-copy view because bytes.Equal neither retains nor mutates
// its arguments, so the Before allocates nothing either. The rewrite's
// win is that the After is copy-free by CONSTRUCTION — string(b) beside
// == compiles to an unconditional no-copy view — while the Before's
// elision is escape-analysis-dependent and never holds on toolchains
// without the zero-copy conversion (gccgo, tinygo, older gc), where it
// pays one allocation and a full copy of s per call. One pair shares a
// long common prefix so the comparison does real work, and one pair
// differs in length so both forms short-circuit in O(1).
var (
	ps2022B = []byte(strings.Repeat("alpha beta gamma delta ", 128) + "x")
	ps2022S = strings.Repeat("alpha beta gamma delta ", 128) + "y"
	ps2022T = strings.Repeat("alpha beta gamma delta ", 128)
)

func BenchmarkPS2022_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		hits := 0
		if bytes.Equal(ps2022B, []byte(ps2022S)) {
			hits++
		}
		if bytes.Equal(ps2022B, []byte(ps2022T)) {
			hits++
		}
		sinkI = hits
	}
}

func BenchmarkPS2022_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		hits := 0
		if string(ps2022B) == ps2022S {
			hits++
		}
		if string(ps2022B) == ps2022T {
			hits++
		}
		sinkI = hits
	}
}

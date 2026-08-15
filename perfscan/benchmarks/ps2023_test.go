package benchmarks

import (
	"strconv"
	"strings"
	"testing"
)

// PS2023 — strings.SplitAfter(s, sep)[i] vs strings.SplitAfterN(s, sep,
// i+2)[i] for a small constant i (here i = 2, the Before/After of the
// check's doc). SplitAfter scans the whole input and allocates a
// []string header for every piece only for [2] to discard all but one
// field; SplitAfterN(..., 4) stops after the third separator and
// allocates at most a four-element slice. The line is the same realistic
// ~1.3KB CSV-ish record of 64 fields as PS2009/PS2014/PS2121, so Before
// pays for 64 pieces and After for the prefix through field 2 only.

var ps2023Line = func() string {
	fields := make([]string, 64)
	for i := range fields {
		fields[i] = strings.Repeat("v", 16) + "-" + strconv.Itoa(i)
	}
	return strings.Join(fields, ",")
}()

func BenchmarkPS2023_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.SplitAfter(ps2023Line, ",")[2]
	}
}

func BenchmarkPS2023_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.SplitAfterN(ps2023Line, ",", 4)[2]
	}
}

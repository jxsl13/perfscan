package benchmarks

import (
	"strconv"
	"strings"
	"testing"
)

// PS2014 — strings.Split(s, sep)[i] vs strings.SplitN(s, sep, i+2)[i]
// for a small constant i (here i = 2, the Before/After of the check's
// doc). Split scans the whole input and allocates a []string header for
// every piece only for [2] to discard all but one field; SplitN(..., 4)
// stops after the third separator and allocates at most a four-element
// slice. The line is the same realistic ~1.3KB CSV-ish record of 64
// fields as PS2009/PS2121, so Before pays for 64 pieces and After for
// the prefix through field 2 only.

var ps2014Line = func() string {
	fields := make([]string, 64)
	for i := range fields {
		fields[i] = strings.Repeat("v", 16) + "-" + strconv.Itoa(i)
	}
	return strings.Join(fields, ",")
}()

func BenchmarkPS2014_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.Split(ps2014Line, ",")[2]
	}
}

func BenchmarkPS2014_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.SplitN(ps2014Line, ",", 4)[2]
	}
}

package benchmarks

import (
	"strconv"
	"strings"
	"testing"
)

// PS2009 — strings.Split(s, sep)[0] vs strings.SplitN(s, sep, 2)[0].
// Split scans the whole input and allocates a []string header for every
// piece only for [0] to discard all but the head; SplitN(..., 2) stops
// at the first separator and allocates at most a two-element slice. The
// line is a realistic ~1.3KB CSV-ish record of 64 fields (same shape as
// PS2121), so Before pays for 64 pieces and After for one prefix.

var ps2009Line = func() string {
	fields := make([]string, 64)
	for i := range fields {
		fields[i] = strings.Repeat("v", 16) + "-" + strconv.Itoa(i)
	}
	return strings.Join(fields, ",")
}()

func BenchmarkPS2009_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.Split(ps2009Line, ",")[0]
	}
}

func BenchmarkPS2009_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.SplitN(ps2009Line, ",", 2)[0]
	}
}

package benchmarks

import (
	"strconv"
	"strings"
	"testing"
)

// PS2015 — strings.Join(strings.Split(s, sep), new) vs
// strings.ReplaceAll(s, sep, new) (the Before/After of the check's doc).
// Split materializes a []string of every fragment (slice header plus
// backing array sized Count+1) purely as an intermediate and Join walks
// it to allocate the result; ReplaceAll produces the identical string in
// one left-to-right pass with only the result allocation. The line is
// the same realistic ~1.3KB CSV-ish record of 64 fields as
// PS2009/PS2014/PS2121, so Before pays for 64 fragment headers plus the
// result and After for the result alone.

var ps2015Line = func() string {
	fields := make([]string, 64)
	for i := range fields {
		fields[i] = strings.Repeat("v", 16) + "-" + strconv.Itoa(i)
	}
	return strings.Join(fields, ",")
}()

func BenchmarkPS2015_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.Join(strings.Split(ps2015Line, ","), "; ")
	}
}

func BenchmarkPS2015_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.ReplaceAll(ps2015Line, ",", "; ")
	}
}

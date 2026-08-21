package benchmarks

import (
	"strconv"
	"strings"
	"testing"
)

// PS2121 — len(strings.Split(s, sep)) vs strings.Count(s, sep) + 1.
// Split allocates the whole []string (one header per piece) only for
// its length to be taken and the slice dropped; Count computes the
// identical integer with one scan and zero allocation. The line is a
// realistic ~1.3KB CSV-ish record of 64 fields (same shape as PS2119).

var ps2121Line = func() string {
	fields := make([]string, 64)
	for i := range fields {
		fields[i] = strings.Repeat("v", 16) + "-" + strconv.Itoa(i)
	}
	return strings.Join(fields, ",")
}()

func BenchmarkPS2121_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = len(strings.Split(ps2121Line, ","))
	}
}

func BenchmarkPS2121_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = strings.Count(ps2121Line, ",") + 1
	}
}

package benchmarks

import (
	"strconv"
	"strings"
	"testing"
)

// PS2021 — parts := strings.Split(s, ","); parts[len(parts)-1] vs
// s[strings.LastIndexByte(s, ',')+1:]. Split scans the whole input and
// allocates a []string of every field only for the final one to be kept;
// LastIndexByte is a single right-to-left byte scan with zero allocation
// and the reslice is O(1) on the immutable string. The line is a
// realistic ~1.3KB CSV-ish record of 64 fields (same shape as PS2009),
// so Before pays for 64 string headers plus the slice and After pays for
// one reverse scan of the final field.

var ps2021Line = func() string {
	fields := make([]string, 64)
	for i := range fields {
		fields[i] = strings.Repeat("v", 16) + "-" + strconv.Itoa(i)
	}
	return strings.Join(fields, ",")
}()

func BenchmarkPS2021_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		parts := strings.Split(ps2021Line, ",")
		sinkS = parts[len(parts)-1]
	}
}

func BenchmarkPS2021_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = ps2021Line[strings.LastIndexByte(ps2021Line, ',')+1:]
	}
}

package benchmarks

import (
	"strconv"
	"strings"
	"testing"
)

// PS2119 — for _, v := range strings.Split(s, sep) vs
// for v := range strings.SplitSeq(s, sep). Split allocates the whole
// []string up front (plus one string header slot per piece); SplitSeq
// yields the identical pieces lazily with no backing slice. The line is
// a realistic ~1.3KB CSV-ish record of 64 fields.

var ps2119Line = func() string {
	fields := make([]string, 64)
	for i := range fields {
		fields[i] = strings.Repeat("v", 16) + "-" + strconv.Itoa(i)
	}
	return strings.Join(fields, ",")
}()

var ps2119Spaced = strings.ReplaceAll(ps2119Line, ",", " ")

func BenchmarkPS2119Split_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n := 0
		for _, part := range strings.Split(ps2119Line, ",") {
			n += len(part)
		}
		sinkI = n
	}
}

func BenchmarkPS2119Split_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n := 0
		for part := range strings.SplitSeq(ps2119Line, ",") {
			n += len(part)
		}
		sinkI = n
	}
}

func BenchmarkPS2119Fields_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n := 0
		for _, f := range strings.Fields(ps2119Spaced) {
			n += len(f)
		}
		sinkI = n
	}
}

func BenchmarkPS2119Fields_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n := 0
		for f := range strings.FieldsSeq(ps2119Spaced) {
			n += len(f)
		}
		sinkI = n
	}
}

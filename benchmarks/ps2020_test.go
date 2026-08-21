package benchmarks

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

// PS2020 — bytes.Join(bytes.Split(b, sep), new) vs
// bytes.ReplaceAll(b, sep, new) (the Before/After of the check's doc; the
// bytes twin of PS2015). Split materializes a [][]byte of every fragment
// (one slice header plus THREE words of subslice descriptor per fragment,
// Count+1 of them) purely as an intermediate and Join walks it to
// allocate the result; ReplaceAll produces the identical bytes in one
// left-to-right pass with only the result allocation. The line is the
// same realistic ~1.3KB CSV-ish record of 64 fields as PS2015, so Before
// pays for the 64-entry [][]byte plus the result and After for the
// result alone.

var (
	ps2020Line = func() []byte {
		fields := make([]string, 64)
		for i := range fields {
			fields[i] = strings.Repeat("v", 16) + "-" + strconv.Itoa(i)
		}
		return []byte(strings.Join(fields, ","))
	}()
	ps2020Sep  = []byte(",")
	ps2020New  = []byte("; ")
	ps2020Sink []byte
)

func BenchmarkPS2020_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2020Sink = bytes.Join(bytes.Split(ps2020Line, ps2020Sep), ps2020New)
	}
}

func BenchmarkPS2020_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2020Sink = bytes.ReplaceAll(ps2020Line, ps2020Sep, ps2020New)
	}
}

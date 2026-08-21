package benchmarks

import (
	"fmt"
	"strconv"
	"testing"
)

// PS5015 — fmt.Appendf(buf, "%d", i) vs strconv.AppendInt(buf, int64(i), 10)
// (and the %g float pair vs strconv.AppendFloat). For a single bare scalar
// verb the two append the identical bytes, but Appendf parses the format,
// BOXES the operand into an interface (a heap allocation) and walks fmt's
// formatter state machine through a pooled pp buffer, while strconv.Append*
// formats the scalar straight into the buffer's existing capacity. Both
// sides reuse a preallocated destination, so the remaining allocations are
// exactly fmt's boxing, which the strconv form removes entirely.
var (
	ps5015Buf  = make([]byte, 0, 64)
	ps5015Int  = 123456
	ps5015F    = 3.141592653589793
	ps5015Sink []byte
)

func BenchmarkPS5015_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5015Sink = fmt.Appendf(ps5015Buf[:0], "%d", ps5015Int)
	}
}

func BenchmarkPS5015_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5015Sink = strconv.AppendInt(ps5015Buf[:0], int64(ps5015Int), 10)
	}
}

func BenchmarkPS5015Float_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5015Sink = fmt.Appendf(ps5015Buf[:0], "%g", ps5015F)
	}
}

func BenchmarkPS5015Float_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5015Sink = strconv.AppendFloat(ps5015Buf[:0], ps5015F, 'g', -1, 64)
	}
}

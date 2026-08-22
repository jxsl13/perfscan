package benchmarks

import (
	"fmt"
	"strconv"
	"testing"
)

// PS2035 — fmt.Appendf(buf, "%v", n) vs strconv.AppendInt(buf, int64(n), 10)
// (and the float pair vs strconv.AppendFloat). For an unnamed integer, bool
// or float operand %v appends the identical bytes, but Appendf parses the
// format, BOXES the operand into an interface (a heap allocation) and walks
// fmt's formatter state machine through a pooled pp buffer, while
// strconv.Append* formats the scalar straight into the buffer's existing
// capacity. Both sides reuse a preallocated destination, so the remaining
// allocations are exactly fmt's boxing, which the strconv form removes
// entirely.
var (
	ps2035Buf  = make([]byte, 0, 64)
	ps2035Int  = 123456
	ps2035F    = 3.141592653589793
	ps2035Sink []byte
)

func BenchmarkPS2035_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2035Sink = fmt.Appendf(ps2035Buf[:0], "%v", ps2035Int)
	}
}

func BenchmarkPS2035_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2035Sink = strconv.AppendInt(ps2035Buf[:0], int64(ps2035Int), 10)
	}
}

func BenchmarkPS2035Float_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2035Sink = fmt.Appendf(ps2035Buf[:0], "%v", ps2035F)
	}
}

func BenchmarkPS2035Float_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2035Sink = strconv.AppendFloat(ps2035Buf[:0], ps2035F, 'g', -1, 64)
	}
}

package benchmarks

import (
	"fmt"
	"strconv"
	"testing"
)

// PS2036 — fmt.Append(buf, x) vs strconv.Append* for a single unnamed
// scalar operand. fmt.Append BOXES the operand into an interface (a heap
// allocation), acquires a pp printer from fmt's sync.Pool, walks the %v
// printer into the pooled buffer and copies it onto buf; strconv.Append*
// formats the value straight into buf's existing capacity with no boxing
// and no reflection. Both sides reuse a preallocated destination, so the
// only allocation is Append's interface box, which the strconv form
// removes entirely. The no-format fmt.Append twin of PS2137's
// Sprint-scalar pair and the scalar sibling of PS5033.
var (
	ps2036Buf  = make([]byte, 0, 64)
	ps2036N    = 123456
	ps2036F    = 123456.789
	ps2036Sink []byte
)

func BenchmarkPS2036_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2036Sink = fmt.Append(ps2036Buf[:0], ps2036N)
	}
}

func BenchmarkPS2036_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2036Sink = strconv.AppendInt(ps2036Buf[:0], int64(ps2036N), 10)
	}
}

func BenchmarkPS2036Float_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2036Sink = fmt.Append(ps2036Buf[:0], ps2036F)
	}
}

func BenchmarkPS2036Float_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2036Sink = strconv.AppendFloat(ps2036Buf[:0], ps2036F, 'g', -1, 64)
	}
}

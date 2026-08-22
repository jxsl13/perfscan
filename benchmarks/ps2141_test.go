package benchmarks

import (
	"fmt"
	"testing"
)

// PS2141 — fmt.Appendf(buf, "%s", s) vs append(buf, s...). For a lone %s over a
// plain string the two append the identical bytes, but Appendf parses the
// format, BOXES s into an interface (a heap allocation) and walks fmt's pp
// buffer, while append is a single builtin copy into buf's existing capacity.
// Both reuse a preallocated destination, so the only allocation is Appendf's
// interface box, which append removes entirely.
var (
	ps2141Buf  = make([]byte, 0, 256)
	ps2141S    = "the quick brown fox jumps"
	ps2141Sink []byte
)

func BenchmarkPS2141_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2141Sink = fmt.Appendf(ps2141Buf[:0], "%s", ps2141S)
	}
}

func BenchmarkPS2141_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2141Sink = append(ps2141Buf[:0], ps2141S...)
	}
}

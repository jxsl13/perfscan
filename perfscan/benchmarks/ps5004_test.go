package benchmarks

import (
	"bytes"
	"testing"
)

// PS5004 — WriteString of a one-byte string constant vs WriteByte. The
// one-byte string takes the generic string-append path (availability check,
// grow bookkeeping, copy loop); WriteByte is the specialized single-byte
// append: one bounds check, one store. Zero allocations either way — the
// win is pure per-call instruction overhead, which is what delimiter
// loops pay n times.
func BenchmarkPS5004_Before(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	buf.Grow(n)
	for range b.N {
		buf.Reset()
		for range n {
			buf.WriteString(",")
		}
		sinkI = buf.Len()
	}
}

func BenchmarkPS5004_After(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	buf.Grow(n)
	for range b.N {
		buf.Reset()
		for range n {
			buf.WriteByte(',')
		}
		sinkI = buf.Len()
	}
}

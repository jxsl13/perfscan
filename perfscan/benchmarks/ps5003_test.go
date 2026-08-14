package benchmarks

import (
	"bytes"
	"testing"
)

// PS5003 — Write of a one-element []byte composite literal vs WriteByte.
// The literal costs a slice header + backing array construction and Write's
// length bookkeeping and copy; WriteByte appends the byte directly. (Here
// the non-escaping literal stays on the stack, so both sides report zero
// allocations; behind an io.Writer interface the Before form would also
// heap-allocate the throwaway slice.)
func BenchmarkPS5003_Before(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	buf.Grow(n)
	for range b.N {
		buf.Reset()
		for range n {
			buf.Write([]byte{','})
		}
		sinkI = buf.Len()
	}
}

func BenchmarkPS5003_After(b *testing.B) {
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

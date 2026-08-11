package benchmarks

import (
	"bytes"
	"fmt"
	"testing"
)

// PS2113 — w.Write([]byte(fmt.Sprintf(...))) allocates the formatted
// string plus a copied []byte before the writer sees a single byte;
// fmt.Fprintf(w, ...) formats straight into the writer.

func BenchmarkPS2113_Before(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	for i := range b.N {
		buf.Reset()
		buf.Write([]byte(fmt.Sprintf("word=%s idx=%d", words[i%n], i)))
		sinkI = buf.Len()
	}
}

func BenchmarkPS2113_After(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	for i := range b.N {
		buf.Reset()
		fmt.Fprintf(&buf, "word=%s idx=%d", words[i%n], i)
		sinkI = buf.Len()
	}
}

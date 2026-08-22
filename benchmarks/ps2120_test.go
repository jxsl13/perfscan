package benchmarks

import (
	"bytes"
	"fmt"
	"testing"
)

// PS2120 — w.WriteString(fmt.Sprintf(...)) allocates the formatted
// string before the writer sees a single byte; fmt.Fprintf(w, ...)
// formats straight into the writer.

func BenchmarkPS2120_Before(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	for i := range b.N {
		buf.Reset()
		buf.WriteString(fmt.Sprintf("word=%s idx=%d", words[i%n], i))
		sinkI = buf.Len()
	}
}

func BenchmarkPS2120_After(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	for i := range b.N {
		buf.Reset()
		fmt.Fprintf(&buf, "word=%s idx=%d", words[i%n], i)
		sinkI = buf.Len()
	}
}

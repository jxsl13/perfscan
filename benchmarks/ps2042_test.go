package benchmarks

import (
	"fmt"
	"io"
	"testing"
)

// PS2042 — fmt.Fprintf(w, "%s", b) vs w.Write(b) for a value that already
// IS an unnamed []byte. The Fprintf side parses the one-verb format, boxes
// b's slice header into an interface (the heap allocation), copies the
// bytes into fmt's pooled pp buffer and only then calls w.Write on that
// buffer; w.Write(b) hands the same bytes straight to the writer. Output
// is identical (%s writes a []byte operand verbatim), as are the (n, err)
// results (fmt's last step is literally w.Write(p.buf)).
var ps2042B = []byte("the quick brown fox jumps over the lazy dog..") // 45 bytes

func BenchmarkPS2042_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n, _ := fmt.Fprintf(io.Discard, "%s", ps2042B)
		sinkI = n
	}
}

func BenchmarkPS2042_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n, _ := io.Discard.Write(ps2042B)
		sinkI = n
	}
}

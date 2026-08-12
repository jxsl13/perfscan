package benchmarks

import (
	"fmt"
	"io"
	"testing"
)

// PS2129 — fmt.Fprintf(w, "%s", s) vs io.WriteString(w, s) for a value
// that already IS a plain string. The Fprintf side parses the one-verb
// format, boxes s into an interface (the single heap allocation) and
// dispatches through fmt's reflection printer; io.WriteString hands the
// same bytes straight to the writer. Output is identical (%s writes a
// string operand verbatim), as are the (n, err) results.
var ps2129S = "the quick brown fox jumps over the lazy dog.." // 45 bytes

func BenchmarkPS2129_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n, _ := fmt.Fprintf(io.Discard, "%s", ps2129S)
		sinkI = n
	}
}

func BenchmarkPS2129_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n, _ := io.WriteString(io.Discard, ps2129S)
		sinkI = n
	}
}

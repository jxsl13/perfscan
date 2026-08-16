package benchmarks

import (
	"fmt"
	"io"
	"testing"
)

// PS2048 — fmt.Fprint(w, a, b, c) over only plain strings vs the
// io.WriteString(w, a+b+c) rewrite. Fprint boxes each operand into an
// interface (one heap allocation apiece), walks fmt's reflection-based
// default printer through a pooled pp buffer and performs one w.Write;
// the rewrite concatenates once (one result allocation) and performs
// one write of the identical bytes with the same (n, err). No spacing
// ever appears between string operands, so the output is byte-identical.
var (
	ps2048Host = "internal.example.com/api" // 24 bytes
	ps2048Sep  = ":"                        // 1 byte
	ps2048Port = "8080\n"                   // 5 bytes
)

func BenchmarkPS2048_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n, _ := fmt.Fprint(io.Discard, ps2048Host, ps2048Sep, ps2048Port)
		sinkI = n
	}
}

func BenchmarkPS2048_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n, _ := io.WriteString(io.Discard, ps2048Host+ps2048Sep+ps2048Port)
		sinkI = n
	}
}

package benchmarks

import (
	"fmt"
	"io"
	"testing"
)

// PS5028 — fmt.Fprintf(w, "constant") with a verbless literal and no
// operands vs io.WriteString(w, "constant"). The Fprintf side takes a pp
// printer from fmt's sync.Pool, scans the format byte-by-byte for verbs
// (finding none), copies the whole run into the pooled pp.buf, calls
// w.Write(pp.buf) and returns the printer to the pool; io.WriteString
// hands the same bytes straight to the writer. Output and (n, err) are
// identical — with no operands nothing is ever formatted. Neither side
// allocates in steady state (fmt pools its printer), so the win is pure
// instruction count: the pool get/put, the format scan and the
// intermediate copy.
const ps5028S = "shutdown: all shards flushed\n" // 29 bytes

func BenchmarkPS5028_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		//lint:ignore SA1006 this benchmark deliberately measures the verbless-constant fmt.Fprintf anti-pattern that PS5028 fixes
		n, _ := fmt.Fprintf(io.Discard, ps5028S)
		sinkI = n
	}
}

func BenchmarkPS5028_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n, _ := io.WriteString(io.Discard, ps5028S)
		sinkI = n
	}
}

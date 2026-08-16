package benchmarks

import (
	"fmt"
	"io"
	"testing"
)

// PS5038 — fmt.Fprintln(w, s) with a single plain-string operand vs
// io.WriteString(w, s+"\n"). The Fprintln side takes a pp printer from
// fmt's sync.Pool, boxes s into an interface (a heap allocation for the
// string header), walks doPrintln's default formatter through the
// pooled buffer and performs one w.Write; the rewrite forms s+"\n" in a
// single runtime.concatstrings allocation and hands it straight to the
// writer. Output and (n, err) are identical — one operand means no
// separating spaces and exactly one trailing newline, and %v of a plain
// string is the verbatim bytes. For a VARIABLE s both sides allocate
// once (the boxed operand vs the joined string); for a constant s the
// +"\n" folds at compile time and the After side is allocation-free.
var ps5038S = "shutdown: all shards flushed" // 28 bytes

func BenchmarkPS5038_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n, _ := fmt.Fprintln(io.Discard, ps5038S)
		sinkI = n
	}
}

func BenchmarkPS5038_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n, _ := io.WriteString(io.Discard, ps5038S+"\n")
		sinkI = n
	}
}

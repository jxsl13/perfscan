package benchmarks

import (
	"fmt"
	"testing"
)

// PS5027 — fmt.Sprintf("constant") vs the literal itself. With no verbs
// and no extra operands fmt.Sprintf returns its format string verbatim,
// but only after a printer is fetched from the sync.Pool, doPrintf walks
// the format, the bytes are copied into the pooled pp buffer and a fresh
// heap string is materialized from it. The literal is a compile-time
// constant: zero pool traffic, zero scan, zero copy, zero allocation.

var sinkPS5027 string

func BenchmarkPS5027_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		//lint:ignore S1039 the verbless fmt.Sprintf call IS the measured Before shape
		sinkPS5027 = fmt.Sprintf("database connection failed")
	}
}

func BenchmarkPS5027_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkPS5027 = "database connection failed"
	}
}

package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS5011 — string(bytes.ReplaceAll([]byte(s), []byte(old), []byte(new)))
// vs strings.ReplaceAll(s, old, new). The Before round-trips all three
// operands through []byte and the replaced result back through
// string(...): measured cost is 3 allocs — the []byte(s) operand copy
// (the input outgrows the 32-byte conversion stack buffer; the short
// old/new conversions stay on the stack), the replaced-result []byte,
// and its heap copy into a fresh string. The After allocates the result
// string once. The input is a typical path-normalization line with
// several matches; the no-match case (strings returns s itself, zero
// allocations, where bytes pays two full copies of s) is pinned in
// checks/equiv_PS5011_test.go.
var (
	ps5011Line = "user//profile//settings//theme = dark; cache//ttl = 300s; ok"
	ps5011Old  = "//"
	ps5011New  = "/"
)

func BenchmarkPS5011_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = string(bytes.ReplaceAll([]byte(ps5011Line), []byte(ps5011Old), []byte(ps5011New)))
	}
}

func BenchmarkPS5011_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.ReplaceAll(ps5011Line, ps5011Old, ps5011New)
	}
}

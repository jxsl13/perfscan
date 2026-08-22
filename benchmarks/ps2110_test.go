// PS2110 — hand-rolled clone via append onto an empty slice vs
// slices.Clone. Both reach the same runtime growslice+copy: this pair
// documents parity (the check's win is intent and the exact
// nil-preservation contract, not cycles).
package benchmarks

import (
	"slices"
	"testing"
)

func BenchmarkPS2110_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkSS = append([]string(nil), words...)
	}
}

func BenchmarkPS2110_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkSS = slices.Clone(words)
	}
}

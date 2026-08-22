package benchmarks

import "testing"

// PS2108 — string([]byte(s)) round-trip vs using the string directly. The
// round-trip copies every string into a fresh byte slice and back into a
// fresh string; the After is the plain assignment the fix produces.
func BenchmarkPS2108_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		for _, w := range words {
			sinkS = string([]byte(w))
		}
	}
}

func BenchmarkPS2108_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		for _, w := range words {
			sinkS = w
		}
	}
}

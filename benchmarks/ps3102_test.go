package benchmarks

import "testing"

// PS3102 — emptying a map with a range-delete loop vs the clear builtin.
// Each iteration refills the map with the 1024 distinct int keys and then
// empties it. Parity is the EXPECTED result for this NaN-free key type: gc
// has pattern-matched the exact range-delete loop into runtime.mapclear
// since Go 1.11, the very call clear(m) compiles to. The check's value is
// directness (and robustness: any drift from the exact loop shape silently
// loses the compiler rewrite, clear cannot).
func BenchmarkPS3102_Before(b *testing.B) {
	b.ReportAllocs()
	m := make(map[int]int, n)
	for range b.N {
		for _, v := range ints {
			m[v] = v
		}
		for k := range m {
			delete(m, k)
		}
		sinkI = len(m)
	}
}

func BenchmarkPS3102_After(b *testing.B) {
	b.ReportAllocs()
	m := make(map[int]int, n)
	for range b.N {
		for _, v := range ints {
			m[v] = v
		}
		clear(m)
		sinkI = len(m)
	}
}

package benchmarks

import (
	"strconv"
	"testing"
)

func ps2053Map() map[string]int {
	m := make(map[string]int, 4096)
	for i := 0; i < 4096; i++ {
		m[strconv.Itoa(i)] = i
	}
	return m
}

var ps2053Sink int

// BenchmarkPS2053Before is the key-only range with a body re-index the check
// flags: m[k] re-hashes the key the range already walked.
func BenchmarkPS2053Before(b *testing.B) {
	m := ps2053Map()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := 0
		for k := range m {
			s += m[k]
		}
		ps2053Sink = s
	}
}

// BenchmarkPS2053After binds the value in the range: one bucket walk, no
// per-iteration re-hash.
func BenchmarkPS2053After(b *testing.B) {
	m := ps2053Map()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := 0
		for _, v := range m {
			s += v
		}
		ps2053Sink = s
	}
}

package benchmarks

import (
	"strconv"
	"testing"
)

func ps2052Map() map[string]int {
	m := make(map[string]int, 1024)
	for i := 0; i < 1024; i++ {
		m[strconv.Itoa(i)] = i % 3
	}
	return m
}

func ps2052Keys() []string {
	ks := make([]string, 1024)
	for i := range ks {
		ks[i] = strconv.Itoa(i)
	}
	return ks
}

var ps2052Sink int

// BenchmarkPS2052Before is the plain double-lookup the check flags: m[k] is
// hashed in the condition and again in the body.
func BenchmarkPS2052Before(b *testing.B) {
	m, keys := ps2052Map(), ps2052Keys()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := 0
		for _, k := range keys {
			if m[k] > 0 {
				s += m[k]
			}
		}
		ps2052Sink = s
	}
}

// BenchmarkPS2052After binds the value in the if-init: one hash per key.
func BenchmarkPS2052After(b *testing.B) {
	m, keys := ps2052Map(), ps2052Keys()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := 0
		for _, k := range keys {
			if v := m[k]; v > 0 {
				s += v
			}
		}
		ps2052Sink = s
	}
}

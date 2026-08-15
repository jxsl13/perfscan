package benchmarks

import (
	"strconv"
	"testing"
)

// PS3021 — the map double-lookup. `if _, ok := m[k]; ok { use(m[k]) }` hashes k
// twice: the comma-ok guard reads and discards the value, then the body reads it
// again. Binding the value in the guard (`v, ok := m[k]`) keeps the single
// existing lookup and turns the body read into a plain local load.

var ps3021Map = func() map[string]int {
	m := make(map[string]int, 1024)
	for i := 0; i < 1024; i++ {
		m[strconv.Itoa(i)] = i
	}
	return m
}()

var ps3021Keys = func() []string {
	k := make([]string, 1024)
	for i := range k {
		k[i] = strconv.Itoa(i)
	}
	return k
}()

var sinkPS3021 int

func BenchmarkPS3021_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		s := 0
		for _, k := range ps3021Keys {
			if _, ok := ps3021Map[k]; ok {
				s += ps3021Map[k]
			}
		}
		sinkPS3021 = s
	}
}

func BenchmarkPS3021_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		s := 0
		for _, k := range ps3021Keys {
			if v, ok := ps3021Map[k]; ok {
				s += v
			}
		}
		sinkPS3021 = s
	}
}

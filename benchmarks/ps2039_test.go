package benchmarks

import (
	"maps"
	"testing"
)

// PS2039 — dst := make(map[int]int); maps.Copy(dst, src) versus
// dst := make(map[int]int, len(src)); maps.Copy(dst, src). maps.Copy
// inserts len(src) entries one by one; the unhinted destination starts at
// minimum bucket capacity and re-buckets every key inserted so far each
// time the table crosses its load factor, while the hinted one allocates
// the buckets once up front and never rehashes during the copy.

var ps2039Src = func() map[int]int {
	m := make(map[int]int, 1024)
	for i := range 1024 {
		m[i*i+7] = i
	}
	return m
}()

var ps2039Sink int

func BenchmarkPS2039_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		dst := make(map[int]int)
		maps.Copy(dst, ps2039Src)
		ps2039Sink = len(dst)
	}
}

func BenchmarkPS2039_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		dst := make(map[int]int, len(ps2039Src))
		maps.Copy(dst, ps2039Src)
		ps2039Sink = len(dst)
	}
}

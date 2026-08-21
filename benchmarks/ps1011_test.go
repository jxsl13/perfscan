package benchmarks

import "testing"

// PS1011 — counting a map's entries with `for range m { n++ }` vs
// len(m). The Before side sets up a hash iterator and walks every
// bucket and overflow bucket, scanning slot metadata and skipping
// empty slots, purely to re-derive the entry count the runtime
// maintains on every insert and delete; len(m) is a single header
// field load. The map holds 4096 entries (the shape from the check's
// MeasuredWin); the Before cost grows linearly with the entry count
// while the After side stays O(1).
var (
	ps1011M    = make(map[int]int, 4096)
	ps1011Sink int
)

func init() {
	for i := 0; i < 4096; i++ {
		ps1011M[i*2654435761] = i
	}
}

func BenchmarkPS1011_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n := 0
		for range ps1011M { //perfscan:ignore PS1011 the Before shape this benchmark exists to measure
			n++
		}
		ps1011Sink = n
	}
}

func BenchmarkPS1011_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n := len(ps1011M)
		ps1011Sink = n
	}
}

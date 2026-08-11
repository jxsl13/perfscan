package benchmarks

import (
	"strings"
	"testing"
)

// PS5104 — strings.Count(s, sub) > 0 vs strings.Contains(s, sub) for a
// membership question. Count scans the entire haystack tallying every
// occurrence; Contains returns at the first match. One needle matches
// early and often (Contains short-circuits almost immediately, Count
// still walks all ~11.5 KB); one needle is absent (both must scan
// everything — the honest lower bound of the win).
var ps5104Text = strings.Repeat("alpha beta gamma delta ", 512)

func BenchmarkPS5104_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		hits := 0
		if strings.Count(ps5104Text, "beta") > 0 {
			hits++
		}
		if strings.Count(ps5104Text, "omega") == 0 {
			hits++
		}
		sinkI = hits
	}
}

func BenchmarkPS5104_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		hits := 0
		if strings.Contains(ps5104Text, "beta") {
			hits++
		}
		if !strings.Contains(ps5104Text, "omega") {
			hits++
		}
		sinkI = hits
	}
}

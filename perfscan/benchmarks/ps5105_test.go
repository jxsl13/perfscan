package benchmarks

import (
	"strings"
	"testing"
)

// PS5105 — strings.Index(s, sub) == 0 vs strings.HasPrefix(s, sub) for a
// prefix question. The haystack's only occurrence of the needle sits at
// the very end, so Index scans all ~11.5 KB to locate it before == 0
// discards the answer; HasPrefix fails at the first mismatched byte.
var ps5105Text = strings.Repeat("alpha beta gamma delta ", 512) + "needle"

func BenchmarkPS5105_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		hits := 0
		if strings.Index(ps5105Text, "needle") == 0 {
			hits++
		}
		sinkI = hits
	}
}

func BenchmarkPS5105_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		hits := 0
		if strings.HasPrefix(ps5105Text, "needle") {
			hits++
		}
		sinkI = hits
	}
}

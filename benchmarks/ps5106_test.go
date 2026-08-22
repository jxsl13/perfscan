package benchmarks

import (
	"strings"
	"testing"
)

// PS5106 — strings.Compare(a, b) == 0 vs the direct a == b operator. Compare
// computes a full three-way ordering through a function call; the operator is a
// direct comparison. Two long strings sharing a long common prefix so both do
// real work before differing.
var (
	ps5106A = strings.Repeat("alpha beta gamma delta ", 256) + "x"
	ps5106B = strings.Repeat("alpha beta gamma delta ", 256) + "y"
)

func BenchmarkPS5106_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		hits := 0
		if strings.Compare(ps5106A, ps5106B) == 0 {
			hits++
		}
		sinkI = hits
	}
}

func BenchmarkPS5106_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		hits := 0
		if ps5106A == ps5106B {
			hits++
		}
		sinkI = hits
	}
}

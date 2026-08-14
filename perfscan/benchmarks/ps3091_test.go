package benchmarks

import "testing"

// PS3091 — a single-slot compiled-resource cache vs a bounded, signature-keyed
// one, under a workload that ALTERNATES among a small set of shapes. The single
// slot holds one artifact + one signature and recompiles on every miss, so
// cycling k>1 shapes recompiles on nearly every dispatch; a bounded map keyed by
// the full signature hits after warmup. `compileGraph` stands in for an
// expensive compile with a fixed lump of work.

func ps3091CompileGraph(sig int) int {
	acc := 0
	for i := 0; i < 20000; i++ { // stand-in for a costly graph/pipeline compile
		acc += (i ^ sig) & 0x3
	}
	return acc
}

var ps3091Shapes = []int{1, 2, 3, 4, 5} // the bounded working set, cycled

func BenchmarkPS3091_Before(b *testing.B) {
	lastSig, lastGraph := -1, 0
	b.ResetTimer()
	for n := range b.N {
		sig := ps3091Shapes[n%len(ps3091Shapes)]
		if sig != lastSig { // single slot: a miss on every alternation
			lastGraph = ps3091CompileGraph(sig)
			lastSig = sig
		}
		sinkI = lastGraph
	}
}

func BenchmarkPS3091_After(b *testing.B) {
	cache := make(map[int]int, 16) // bounded cache keyed by the full signature
	b.ResetTimer()
	for n := range b.N {
		sig := ps3091Shapes[n%len(ps3091Shapes)]
		g, ok := cache[sig]
		if !ok {
			g = ps3091CompileGraph(sig)
			cache[sig] = g
		}
		sinkI = g
	}
}

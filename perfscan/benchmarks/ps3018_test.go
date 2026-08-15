package benchmarks

import (
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// PS3018 — slices.SortedStableFunc with a bare strings.Compare(a, b)
// comparator vs slices.Sorted. Both collect the same 4096-element tie-heavy
// iter.Seq[string] (identical slices.Collect growth allocations) and sort
// it; SortedStableFunc pays an indirect comparator call (plus the
// strings.Compare hop inside the literal) on every one of the O(n log n)
// comparisons AND runs the stable insertion-run/symMerge algorithm, both of
// which the monomorphized unstable slices.Sort drops. The delta is larger
// than PS3015's (the SortedFunc sibling on the same input shape) because it
// stacks the stability overhead on top of the comparator indirection — the
// same union-of-costs PS3016 measures for the cmp.Compare spelling.
var ps3018Words = func() []string {
	out := make([]string, 4096)
	for i := range out {
		out[i] = "w-" + strconv.Itoa(i%1024) // duplicates -> real ties
	}
	rng := rand.New(rand.NewSource(1))
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}()

var ps3018Sink []string

func BenchmarkPS3018_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3018Sink = slices.SortedStableFunc(slices.Values(ps3018Words), func(x, y string) int { return strings.Compare(x, y) })
	}
}

func BenchmarkPS3018_BeforeFuncValue(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3018Sink = slices.SortedStableFunc(slices.Values(ps3018Words), strings.Compare)
	}
}

func BenchmarkPS3018_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3018Sink = slices.Sorted(slices.Values(ps3018Words))
	}
}

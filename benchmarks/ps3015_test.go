package benchmarks

import (
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// PS3015 — slices.SortedFunc with a bare strings.Compare(a, b) comparator vs
// slices.Sorted. Both collect the same 4096-element tie-heavy iter.Seq[string]
// (identical slices.Collect growth allocations) and sort it; SortedFunc pays
// an indirect comparator call (plus the strings.Compare hop inside the
// literal) on every one of the O(n log n) comparisons, which the
// monomorphized slices.Sort inlines. The delta is smaller than the []int
// sibling's (PS3012) because the byte comparison is a runtime cmpstring call
// on BOTH sides — what the rewrite removes is the comparator indirection,
// the same win PS3009 measures for the eager SortFunc form.
var ps3015Words = func() []string {
	out := make([]string, 4096)
	for i := range out {
		out[i] = "w-" + strconv.Itoa(i%1024) // duplicates -> real ties
	}
	rng := rand.New(rand.NewSource(1))
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}()

var ps3015Sink []string

func BenchmarkPS3015_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3015Sink = slices.SortedFunc(slices.Values(ps3015Words), func(x, y string) int { return strings.Compare(x, y) })
	}
}

func BenchmarkPS3015_BeforeFuncValue(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3015Sink = slices.SortedFunc(slices.Values(ps3015Words), strings.Compare)
	}
}

func BenchmarkPS3015_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3015Sink = slices.Sorted(slices.Values(ps3015Words))
	}
}

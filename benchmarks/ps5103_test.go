package benchmarks

import (
	"strings"
	"testing"
)

// ps5103Mixed pairs each package word with an upper-cased copy so every
// comparison is a real case-insensitive match (equal under folding,
// unequal byte-wise).
var ps5103Mixed = func() []string {
	out := make([]string, len(words))
	for i, s := range words {
		out[i] = strings.ToUpper(s)
	}
	return out
}()

// PS5103 — case-insensitive compare via ToLower equality vs EqualFold.
// The manual remedy is not bit-identical for all Unicode inputs (see the
// check doc); these inputs are ASCII, where the two spellings agree.
func BenchmarkPS5103_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		hits := 0
		for i, s := range words {
			//lint:ignore SA6005 this is the Before side of the PS5103 pair
			if strings.ToLower(s) == strings.ToLower(ps5103Mixed[i]) {
				hits++
			}
		}
		sinkI = hits
	}
}

func BenchmarkPS5103_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		hits := 0
		for i, s := range words {
			if strings.EqualFold(s, ps5103Mixed[i]) {
				hits++
			}
		}
		sinkI = hits
	}
}

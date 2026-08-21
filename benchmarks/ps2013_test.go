package benchmarks

import (
	"strings"
	"testing"
)

// PS2013 — inline single-pair strings.NewReplacer(old, new).Replace(s) vs
// strings.ReplaceAll(s, old, new). The Before constructs a full replacer
// machine (generic trie for a multi-byte key), uses it once, and discards
// it; the After does one Count-sized allocation and one linear scan. The
// hoisted-replacer reference is the PS2132 alternative: time-parity with
// ReplaceAll but 3 allocs vs 1 (genericReplacer.Replace routes its output
// through an append-writer buffer), so the ReplaceAll rewrite never loses
// to it either. The input is a sentence with several matches so the
// substitution does real work.
var ps2013Sentence = "the quick brown fox jumps over the lazy dog while the cat watches the wall"

func BenchmarkPS2013_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.NewReplacer("the", "THE").Replace(ps2013Sentence)
	}
}

func BenchmarkPS2013_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.ReplaceAll(ps2013Sentence, "the", "THE")
	}
}

// BenchmarkPS2013_HoistedReference is not a Before/After leg: it measures
// the PS2132-style hoisted single-pair replacer, pinning the doc's claim
// that ReplaceAll is time-parity with fewer allocations even against the
// amortized alternative.
var ps2013Hoisted = strings.NewReplacer("the", "THE")

func BenchmarkPS2013_HoistedReference(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = ps2013Hoisted.Replace(ps2013Sentence)
	}
}

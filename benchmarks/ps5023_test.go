package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS5023 — strings/bytes IndexRune with a constant ASCII rune vs the
// IndexByte form the fix's rename produces. For 0 <= r < utf8.RuneSelf,
// IndexRune's entire body is "return IndexByte(s, byte(r))" behind a
// multi-branch switch that keeps it above the inliner's cost budget, so
// each call pays a non-inlined wrapper frame plus the range check before
// reaching the assembly-optimized IndexByte intrinsic; the rename removes
// that wrapper entirely. Zero allocations on every side; the entire win
// is instruction count. The haystack is a typical 61-byte log line with
// the needle in the last field, the shape from the check's MeasuredWin
// (and the PS5007/PS5013/PS5022 benchmarks — the byte/substring/cutset
// needle siblings of this rewrite).
var (
	ps5023HaystackStr = "service=checkout region=eu-west-1 shard=07 status=ok final=z"
	ps5023HaystackByt = []byte(ps5023HaystackStr)
)

var ps5023Sink int

func BenchmarkPS5023StringsIndexRune_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5023Sink = strings.IndexRune(ps5023HaystackStr, 'z')
	}
}

func BenchmarkPS5023StringsIndexRune_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5023Sink = strings.IndexByte(ps5023HaystackStr, 'z')
	}
}

func BenchmarkPS5023BytesIndexRune_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5023Sink = bytes.IndexRune(ps5023HaystackByt, 'z')
	}
}

func BenchmarkPS5023BytesIndexRune_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5023Sink = bytes.IndexByte(ps5023HaystackByt, 'z')
	}
}

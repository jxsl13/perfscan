package benchmarks

import (
	"strings"
	"testing"
)

// PS5007 — strings.Index(s, "z") / strings.LastIndex(s, "z") vs
// strings.IndexByte(s, "z"[0]) / strings.LastIndexByte(s, "z"[0]). For a
// one-byte needle, Index pays a per-call needle-length dispatch before it
// delegates to the byte scan, and LastIndex has NO single-byte fast path
// at all — it runs its full Rabin-Karp reverse substring search. The
// byte forms skip straight to the direct scan. Zero allocations on every
// side; the entire win is instruction count. The "z"[0] spelling the fix
// emits is constant-folded by the compiler — measured identical to a
// hand-written 'z'. The haystack is a typical 61-byte log line with the
// needle in the last field, the shape from the check's MeasuredWin.
var ps5007Haystack = "service=checkout region=eu-west-1 shard=07 status=ok final=z"

func BenchmarkPS5007_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = strings.Index(ps5007Haystack, "z")
	}
}

func BenchmarkPS5007_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = strings.IndexByte(ps5007Haystack, "z"[0])
	}
}

func BenchmarkPS5007LastIndex_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = strings.LastIndex(ps5007Haystack, "z")
	}
}

func BenchmarkPS5007LastIndex_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = strings.LastIndexByte(ps5007Haystack, "z"[0])
	}
}

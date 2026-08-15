package benchmarks

import (
	"strings"
	"testing"
)

// PS5024 — strings.ContainsRune with a constant ASCII rune vs the
// IndexByte comparison the fix emits. ContainsRune is IndexRune >= 0, and
// for an ASCII rune IndexRune immediately delegates to IndexByte — but
// neither wrapper is inlined, so every probe pays two call frames plus
// IndexRune's rune-class dispatch before the SIMD byte scan. IndexByte
// jumps straight to the scan. Zero allocations on every side (the needle
// is a constant rune, the haystack passes through untouched); the entire
// win is instruction count. The untyped rune literal the fix passes to
// IndexByte verbatim is constant-folded by the compiler. The haystack is
// a typical 61-byte log line with the needle in the last field, the shape
// from the check's MeasuredWin (and the PS5013/PS5014/PS5016/PS5022
// benchmarks — the substring- and cutset-needle siblings of this
// rewrite).
var ps5024Haystack = "service=checkout region=eu-west-1 shard=07 status=ok final=z"

var ps5024Sink bool

func BenchmarkPS5024Contains_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5024Sink = strings.ContainsRune(ps5024Haystack, 'z')
	}
}

func BenchmarkPS5024Contains_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5024Sink = strings.IndexByte(ps5024Haystack, 'z') >= 0
	}
}

func BenchmarkPS5024ContainsNot_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5024Sink = !strings.ContainsRune(ps5024Haystack, '=')
	}
}

func BenchmarkPS5024ContainsNot_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5024Sink = strings.IndexByte(ps5024Haystack, '=') < 0
	}
}

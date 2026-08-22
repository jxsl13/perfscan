package benchmarks

import (
	"bytes"
	"testing"
)

// PS5026 — bytes.ContainsRune with a constant ASCII rune vs the
// IndexByte comparison the fix emits. ContainsRune is IndexRune >= 0, and
// for an ASCII rune IndexRune immediately delegates to IndexByte — but
// neither wrapper is inlined, so every probe pays two call frames plus
// IndexRune's rune-class dispatch before the SIMD byte scan. IndexByte
// jumps straight to the scan. Zero allocations on every side (the needle
// is a constant rune, the haystack passes through untouched); the entire
// win is instruction count. The untyped rune literal the fix passes to
// IndexByte verbatim is constant-folded by the compiler. The haystack is
// a typical 61-byte log line with the needle in the last field, the shape
// from the check's MeasuredWin (and the PS5024 benchmark — the strings
// twin of this rewrite).
var ps5026Haystack = []byte("service=checkout region=eu-west-1 shard=07 status=ok final=z")

var ps5026Sink bool

func BenchmarkPS5026Contains_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5026Sink = bytes.ContainsRune(ps5026Haystack, 'z')
	}
}

func BenchmarkPS5026Contains_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5026Sink = bytes.IndexByte(ps5026Haystack, 'z') >= 0
	}
}

func BenchmarkPS5026ContainsNot_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5026Sink = !bytes.ContainsRune(ps5026Haystack, '=')
	}
}

func BenchmarkPS5026ContainsNot_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5026Sink = bytes.IndexByte(ps5026Haystack, '=') < 0
	}
}

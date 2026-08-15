package benchmarks

import (
	"bytes"
	"testing"
)

// PS5014 — bytes.Contains(b, []byte{'z'}) vs bytes.IndexByte(b, 'z') >= 0
// (and the !Contains vs < 0 pair the fix's !-absorption emits). For a
// one-byte needle, Contains delegates to Index, which pays a per-call
// needle-length dispatch and a sep[0] load before delegating to the byte
// scan, and the CALLER constructs a throwaway one-element []byte on top;
// the IndexByte comparison skips straight to the direct scan with no
// needle at all. Zero allocations on every side (escape analysis keeps
// the needle slice on the stack); the entire win is instruction count.
// The "="[0] spelling the fix emits for the conversion form is
// constant-folded by the compiler. The haystack is a typical 61-byte log
// line with the needle in the last field, the shape from the check's
// MeasuredWin (and PS5013's benchmark — the index form of this rewrite).
var ps5014Haystack = []byte("service=checkout region=eu-west-1 shard=07 status=ok final=z")

var ps5014Sink bool

func BenchmarkPS5014_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5014Sink = bytes.Contains(ps5014Haystack, []byte{'z'})
	}
}

func BenchmarkPS5014_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5014Sink = bytes.IndexByte(ps5014Haystack, 'z') >= 0
	}
}

func BenchmarkPS5014Not_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5014Sink = !bytes.Contains(ps5014Haystack, []byte("="))
	}
}

func BenchmarkPS5014Not_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5014Sink = bytes.IndexByte(ps5014Haystack, "="[0]) < 0
	}
}

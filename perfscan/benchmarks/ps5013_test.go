package benchmarks

import (
	"bytes"
	"testing"
)

// PS5013 — bytes.Index(b, []byte{'z'}) / bytes.LastIndex(b, []byte("z"))
// vs bytes.IndexByte(b, 'z') / bytes.LastIndexByte(b, "z"[0]). For a
// one-byte needle, Index and LastIndex pay a per-call needle-length
// dispatch and a sep[0] load before delegating to the byte scan, and the
// CALLER constructs a throwaway one-element []byte on top; the byte forms
// skip straight to the direct scan with no needle at all. Zero
// allocations on every side (escape analysis keeps the needle slice on
// the stack); the entire win is instruction count. The "z"[0] spelling
// the fix emits for the conversion form is constant-folded by the
// compiler. The haystack is a typical 61-byte log line with the needle
// in the last field, the shape from the check's MeasuredWin (and the
// bytes twin of PS5007's benchmark).
var ps5013Haystack = []byte("service=checkout region=eu-west-1 shard=07 status=ok final=z")

func BenchmarkPS5013_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = bytes.Index(ps5013Haystack, []byte{'z'})
	}
}

func BenchmarkPS5013_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = bytes.IndexByte(ps5013Haystack, 'z')
	}
}

func BenchmarkPS5013LastIndex_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = bytes.LastIndex(ps5013Haystack, []byte("z"))
	}
}

func BenchmarkPS5013LastIndex_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = bytes.LastIndexByte(ps5013Haystack, "z"[0])
	}
}

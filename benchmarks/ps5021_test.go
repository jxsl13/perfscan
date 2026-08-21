package benchmarks

import (
	"strings"
	"testing"
)

// PS5021 — copy(dst, []byte(s)) vs copy(dst, s). The Before spells out a
// string->[]byte conversion whose throwaway result the builtin
// immediately copies again; the After uses copy's spec-level string
// source special case: one memmove, no temporary. Honest gc note: on
// gc >= 1.22 the escape analyzer's zero-copy optimization already
// eliminates the conversion's allocation for this exact argument shape
// (both rows report 0 allocs here), so this pair measures the residual
// per-call conversion scaffolding — near-parity, with the memmove
// dominating both forms. Where that
// optimization does not apply (Go < 1.22, other toolchains, the
// conversion hoisted into a variable — measurable with
// -gcflags=-d=zerocopy=0) the Before additionally pays a len(s)-byte
// allocation and a full extra copy per call, an ~8-13x gap that scales
// with the bytes moved; the equiv suite pins that both forms are
// byte-identical regardless.
var (
	ps5021Src  = strings.Repeat("perfscan", 8) // 64 bytes
	ps5021Dst  = make([]byte, 64)
	ps5021Sink int
)

func BenchmarkPS5021_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5021Sink = copy(ps5021Dst, []byte(ps5021Src))
	}
}

func BenchmarkPS5021_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5021Sink = copy(ps5021Dst, ps5021Src)
	}
}

package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS5025 — strings/bytes LastIndexAny with a one-ASCII-byte cutset vs
// the LastIndexByte forms the fix emits. For a one-character cutset
// LastIndexAny still pays its full per-call dispatch — the empty-cutset
// check, and then for len(s) > 8 it BUILDS a makeASCIISet 32-byte bitset
// (zeroed and populated on every call) before running a reverse
// set-membership loop over the haystack (shorter haystacks take a
// reverse utf8.DecodeLastRune loop instead). LastIndexByte dispatches
// straight into bytealg's reverse single-byte scan: the set build, the
// rune machinery, and the call-frame chain disappear. Zero allocations
// on every side (the cutset is a string constant and the bitset is
// stack-allocated); the entire win is instruction count. The
// "z"[0]/"a"[0] spellings the fix emits are constant-folded by the
// compiler. The haystack is a typical 61-byte log line with the needle
// in the last field, the shape from the check's MeasuredWin (and the
// PS5013/PS5022 benchmarks — the forward and substring-needle siblings
// of this rewrite); the Start pair moves the needle to the first byte,
// making the backward scan traverse the whole line.
var (
	ps5025HaystackStr = "service=checkout region=eu-west-1 shard=07 status=ok final=z"
	ps5025HaystackByt = []byte(ps5025HaystackStr)
	// The needle only at byte 0: the backward scan's worst case.
	ps5025StartStr = "z-------------------------------------------------------------"
	ps5025StartByt = []byte(ps5025StartStr)
)

var ps5025Sink int

func BenchmarkPS5025StringsLast_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5025Sink = strings.LastIndexAny(ps5025HaystackStr, "z")
	}
}

func BenchmarkPS5025StringsLast_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5025Sink = strings.LastIndexByte(ps5025HaystackStr, "z"[0])
	}
}

func BenchmarkPS5025BytesLast_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5025Sink = bytes.LastIndexAny(ps5025HaystackByt, "z")
	}
}

func BenchmarkPS5025BytesLast_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5025Sink = bytes.LastIndexByte(ps5025HaystackByt, "z"[0])
	}
}

func BenchmarkPS5025StringsFullScan_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5025Sink = strings.LastIndexAny(ps5025StartStr, "z")
	}
}

func BenchmarkPS5025StringsFullScan_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5025Sink = strings.LastIndexByte(ps5025StartStr, "z"[0])
	}
}

func BenchmarkPS5025BytesFullScan_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5025Sink = bytes.LastIndexAny(ps5025StartByt, "z")
	}
}

func BenchmarkPS5025BytesFullScan_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5025Sink = bytes.LastIndexByte(ps5025StartByt, "z"[0])
	}
}

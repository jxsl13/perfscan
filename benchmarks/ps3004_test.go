package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS3004 — bytes.Index([]byte(s), []byte(sub)) vs strings.Index(s, sub)
// over plain strings. The bytes side materializes two throwaway []byte
// copies of the operands (two allocations + two memmoves) before the
// scan even starts; the strings side runs the identical algorithm over
// the identical bytes straight off the string headers. Results are
// bit-identical (pinned by TestEquiv_PS3004BytesPredToStrings).
var (
	ps3004Hay    = "the quick brown fox jumps over the lazy dog near a riverbank" // 60 bytes
	ps3004Needle = "riverbank"                                                    // near the end: a real scan
)

func BenchmarkPS3004_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = bytes.Index([]byte(ps3004Hay), []byte(ps3004Needle))
	}
}

func BenchmarkPS3004_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = strings.Index(ps3004Hay, ps3004Needle)
	}
}

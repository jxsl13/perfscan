package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS2019 — strings.Index(string(b), string(sub)) vs bytes.Index(b, sub)
// over plain []byte operands (the exact mirror of PS3004's pair). The
// strings side materializes two throwaway string copies of the operands
// (two allocations + two memmoves) before the scan even starts; the bytes
// side runs the identical algorithm over the identical bytes straight off
// the slice headers. Results are bit-identical (pinned by
// TestEquiv_PS2019StringsPredToBytes).
var (
	ps2019Hay    = []byte("the quick brown fox jumps over the lazy dog near a riverbank") // 60 bytes
	ps2019Needle = []byte("riverbank")                                                    // near the end: a real scan
)

func BenchmarkPS2019_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = strings.Index(string(ps2019Hay), string(ps2019Needle))
	}
}

func BenchmarkPS2019_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = bytes.Index(ps2019Hay, ps2019Needle)
	}
}

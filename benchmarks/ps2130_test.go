package benchmarks

import (
	"fmt"
	"testing"
)

// PS2130 — fmt.Sprintf("%s", s) vs s itself for a value that already IS
// a plain string. The Sprintf side parses the one-verb format, boxes s
// into an interface (one heap allocation) and copies the bytes into a
// fresh result string (the second); the identity side just hands the
// same immutable string over. Output is bit-identical (%s writes a
// string operand verbatim).
var ps2130S = "the quick brown fox jumps over the lazy dog." // 44 bytes

func BenchmarkPS2130_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		//lint:ignore S1025 this benchmark deliberately measures the fmt.Sprintf("%s", s) identity anti-pattern that PS2130 fixes
		sinkS = fmt.Sprintf("%s", ps2130S)
	}
}

func BenchmarkPS2130_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = ps2130S
	}
}

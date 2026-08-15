package benchmarks

import (
	"strconv"
	"testing"
)

// PS2032 — string(strconv.AppendInt(nil, x, 10)) vs
// strconv.FormatInt(x, 10) on a 6-digit int64 (PS2136's shape, mirrored).
// The Before side appends to the nil slice (one heap allocation for the
// digit buffer) and then string(...) copies those bytes into a second
// heap allocation; the After side formats straight into the single
// string result. For small integers (0 <= i < 100 in base 10) FormatInt
// returns a substring of a package constant, so the same rewrite is two
// allocations to ZERO — the small-int pair pins that path.
var (
	ps2032X    int64 = 123456
	ps2032S    int64 = 7
	ps2032Sink string
)

func BenchmarkPS2032_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2032Sink = string(strconv.AppendInt(nil, ps2032X, 10))
	}
}

func BenchmarkPS2032_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2032Sink = strconv.FormatInt(ps2032X, 10)
	}
}

func BenchmarkPS2032_SmallInt_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2032Sink = string(strconv.AppendInt(nil, ps2032S, 10))
	}
}

func BenchmarkPS2032_SmallInt_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2032Sink = strconv.FormatInt(ps2032S, 10)
	}
}

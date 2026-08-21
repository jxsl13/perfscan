package benchmarks

import (
	"strconv"
	"testing"
)

// PS2047 — append(dst, strconv.FormatInt(x, 10)...) vs
// strconv.AppendInt(dst, x, 10) into a reset preallocated []byte: the
// Before spelling allocates the formatted string (any value outside
// strconv's tiny small-int cache) and append then copies those bytes a
// second time into dst's tail; AppendInt runs the identical formatter
// straight into dst's backing array — the intermediate string and its
// copy both disappear, 1 alloc -> 0 when dst has spare capacity.

var (
	ps2047Dst  = make([]byte, 0, 64)
	ps2047Val  = int64(1<<40 + 7) // 13 digits, far outside the small-int cache
	ps2047Sink []byte
)

func BenchmarkPS2047_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2047Sink = append(ps2047Dst[:0], strconv.FormatInt(ps2047Val, 10)...) //perfscan:ignore PS2047 the Before shape this benchmark exists to measure
	}
}

func BenchmarkPS2047_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2047Sink = strconv.AppendInt(ps2047Dst[:0], ps2047Val, 10)
	}
}

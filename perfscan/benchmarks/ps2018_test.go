package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS2018 — string(bytes.Repeat([]byte(s), n)) vs strings.Repeat(s, n).
// The Before round-trips through []byte: the []byte(s) seed conversion
// (stack-allocated here, a third heap allocation in shapes escape
// analysis cannot prove), the len(s)*n repeat buffer bytes.Repeat
// allocates and fills, and the string(...) conversion that copies the
// whole buffer into a fresh immutable string — the full-length result
// is written twice. The After fills a single len(s)*n buffer and
// returns it zero-copy: one allocation, every byte written once. The
// seed deliberately avoids strings.Repeat's lookup-table fast-path
// seeds (' ', '-', '0', '=', '\t') so the two sides run the same
// general fill loop.
var ps2018Seed = "ab"

func BenchmarkPS2018_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = string(bytes.Repeat([]byte(ps2018Seed), 64))
	}
}

func BenchmarkPS2018_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.Repeat(ps2018Seed, 64)
	}
}

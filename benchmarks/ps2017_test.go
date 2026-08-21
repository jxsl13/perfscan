package benchmarks

import (
	"bytes"
	"strings"
	"testing"
	"unicode"
)

// PS2017 — string(bytes.Map(f, []byte(s))) vs strings.Map(f, s). The
// Before round-trips s through []byte: []byte(s) copies s in, bytes.Map
// allocates and fills a fresh result slice, and string(...) copies that
// result out. The After reads straight off the string header and
// allocates only the result. Two pairs bracket the win: mapping
// unicode.ToUpper over a mixed-case line containing ß (the mapping must
// allocate — the After saves the round-trip copy) and an identity
// mapping that changes nothing (the After's no-change path returns s
// itself: zero allocations against the Before's unconditional
// round-trip).
var (
	ps2017Mixed = "mixed-Case straße payload with Fifty-five Bytes total!"
	ps2017Ident = func(r rune) rune { return r }
)

func BenchmarkPS2017_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = string(bytes.Map(unicode.ToUpper, []byte(ps2017Mixed)))
	}
}

func BenchmarkPS2017_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.Map(unicode.ToUpper, ps2017Mixed)
	}
}

func BenchmarkPS2017_NoChange_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = string(bytes.Map(ps2017Ident, []byte(ps2017Mixed)))
	}
}

func BenchmarkPS2017_NoChange_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.Map(ps2017Ident, ps2017Mixed)
	}
}

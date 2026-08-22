package benchmarks

import (
	"strings"
	"testing"
)

// PS5020 — append(dst, []byte(s)...) vs append(dst, s...). Honest
// framing (mirroring the gc-parity checks PS2119/PS2125): since go1.22
// cmd/compile's zero-copy escape analysis rewrites the Before's
// conversion — non-escaping and never mutated, which the direct spread
// argument always satisfies — to alias the string's memory, so on
// current gc both shapes compile to the same growslice+memmove and the
// pair measures as parity with zero allocations. The rewrite moves that
// guarantee from a compiler special case into the source: on gc <=1.21,
// gccgo and tinygo the Before pays a full extra allocation+copy of
// len(s) bytes per call, and any refactor that hoists []byte(s) out of
// the spread position re-introduces the copy on every toolchain. The
// After does a single copy everywhere, so the rewrite never loses. The
// destination is a reused 2 KiB buffer so neither shape ever grows —
// the measurement isolates the conversion itself.
var (
	ps5020S    = strings.Repeat("level=info msg=served path=/api/v1/items dur=1ms; ", 20) + "tail" // 1004 bytes
	ps5020Dst  = make([]byte, 0, 2048)
	ps5020Sink []byte
)

func BenchmarkPS5020_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5020Sink = append(ps5020Dst[:0], []byte(ps5020S)...)
	}
}

func BenchmarkPS5020_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5020Sink = append(ps5020Dst[:0], ps5020S...)
	}
}

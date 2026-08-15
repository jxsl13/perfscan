package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS5022 — strings/bytes IndexAny/ContainsAny with a one-ASCII-byte
// cutset vs the IndexByte forms the fix emits. For a one-character cutset
// IndexAny still pays its full per-call dispatch — the empty-cutset
// check, the len(chars) == 1 branch, a rune conversion and RuneSelf
// comparison — and then CALLS IndexRune, which repeats its own range
// check and CALLS IndexByte; ContainsAny is IndexAny >= 0 on top.
// IndexByte jumps straight to the SIMD byte scan: two non-inlined call
// frames and several branches disappear. Zero allocations on every side
// (the cutset is a string constant, never a constructed slice); the
// entire win is instruction count. The "z"[0]/"="[0] spellings the fix
// emits are constant-folded by the compiler. The haystack is a typical
// 61-byte log line with the needle in the last field, the shape from the
// check's MeasuredWin (and the PS5013/PS5014/PS5016 benchmarks — the
// substring-needle siblings of this rewrite).
var (
	ps5022HaystackStr = "service=checkout region=eu-west-1 shard=07 status=ok final=z"
	ps5022HaystackByt = []byte(ps5022HaystackStr)
)

var (
	ps5022SinkInt  int
	ps5022SinkBool bool
)

func BenchmarkPS5022StringsIndex_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5022SinkInt = strings.IndexAny(ps5022HaystackStr, "z")
	}
}

func BenchmarkPS5022StringsIndex_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5022SinkInt = strings.IndexByte(ps5022HaystackStr, "z"[0])
	}
}

func BenchmarkPS5022BytesIndex_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5022SinkInt = bytes.IndexAny(ps5022HaystackByt, "z")
	}
}

func BenchmarkPS5022BytesIndex_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5022SinkInt = bytes.IndexByte(ps5022HaystackByt, "z"[0])
	}
}

func BenchmarkPS5022StringsContains_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5022SinkBool = strings.ContainsAny(ps5022HaystackStr, "=")
	}
}

func BenchmarkPS5022StringsContains_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5022SinkBool = strings.IndexByte(ps5022HaystackStr, "="[0]) >= 0
	}
}

func BenchmarkPS5022BytesContainsNot_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5022SinkBool = !bytes.ContainsAny(ps5022HaystackByt, "=")
	}
}

func BenchmarkPS5022BytesContainsNot_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5022SinkBool = bytes.IndexByte(ps5022HaystackByt, "="[0]) < 0
	}
}

package benchmarks

import (
	"strings"
	"testing"
)

// PS5016 — strings.Contains(s, "z") vs strings.IndexByte(s, "z"[0]) >= 0
// (and the !Contains vs < 0 pair the fix's !-absorption emits). For a
// one-byte needle, Contains delegates to Index, which pays a per-call
// needle-length dispatch and a substr[0] load before delegating to the
// byte scan; the IndexByte comparison skips straight to the direct scan.
// Zero allocations on every side (the needle is a string constant — no
// slice is ever constructed); the entire win is instruction count. The
// "z"[0] spelling the fix emits is constant-folded by the compiler. The
// haystack is a typical 61-byte log line with the needle in the last
// field, the shape from the check's MeasuredWin (and PS5007's and
// PS5014's benchmarks — the index form and the bytes twin of this
// rewrite).
var ps5016Haystack = "service=checkout region=eu-west-1 shard=07 status=ok final=z"

var ps5016Sink bool

func BenchmarkPS5016_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5016Sink = strings.Contains(ps5016Haystack, "z")
	}
}

func BenchmarkPS5016_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5016Sink = strings.IndexByte(ps5016Haystack, "z"[0]) >= 0
	}
}

func BenchmarkPS5016Not_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5016Sink = !strings.Contains(ps5016Haystack, "=")
	}
}

func BenchmarkPS5016Not_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5016Sink = strings.IndexByte(ps5016Haystack, "="[0]) < 0
	}
}

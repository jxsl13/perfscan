package benchmarks

import (
	"testing"
	"unicode/utf8"
)

// PS2025 — utf8.Valid([]byte(s)) vs utf8.ValidString(s), and the reverse
// utf8.ValidString(string(b)) vs utf8.Valid(b). The two functions run the
// identical validation state machine over the same bytes; the Before
// sides additionally copy the whole input through a throwaway conversion.
// The reverse direction pays that copy for real on current gc (string(b)
// must copy — b may be mutated afterwards — heap-allocating past the
// 32-byte stack temporary); the forward direction demonstrates gc's
// elision of the non-escaping, read-only []byte(s) conversion
// empirically (near-parity, the honest caveat in the check's
// MeasuredWin). The input is a 64-byte mixed ASCII/multi-byte line, the
// shape from the check's MeasuredWin.
var (
	ps2025Str   = "service=checkout región=eu-wést-1 status=ok body=héllo, 世界!"
	ps2025Bytes = []byte(ps2025Str)
	ps2025Sink  bool
)

func BenchmarkPS2025_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2025Sink = utf8.Valid([]byte(ps2025Str))
	}
}

func BenchmarkPS2025_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2025Sink = utf8.ValidString(ps2025Str)
	}
}

func BenchmarkPS2025Reverse_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2025Sink = utf8.ValidString(string(ps2025Bytes))
	}
}

func BenchmarkPS2025Reverse_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2025Sink = utf8.Valid(ps2025Bytes)
	}
}

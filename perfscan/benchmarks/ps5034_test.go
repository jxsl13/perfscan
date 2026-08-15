package benchmarks

import (
	"strings"
	"testing"
	"unicode"
)

// PS5034 — strings.TrimFunc(s, unicode.IsSpace) vs strings.TrimSpace(s).
// The Before pays, per boundary rune examined, a utf8 decode plus an
// INDIRECT call through the predicate function pointer into
// unicode.IsSpace (never inlinable, never devirtualized). The After
// classifies boundary bytes with a 256-entry table lookup (asciiSpace)
// and only falls back to the very same TrimFunc call when it meets a
// byte >= utf8.RuneSelf. Three inputs pin the honest profile: ASCII
// input with white space to trim (the common log/config line), an
// already-trimmed ASCII input (the hot no-op path — TrimSpace inspects
// one byte per end, TrimFunc still decodes and indirect-calls at both
// ends), and an NBSP-padded input where TrimSpace immediately delegates
// to the user's exact TrimFunc call — parity by construction. Both
// spellings return a substring: 0 B/op, 0 allocs/op everywhere.
var (
	ps5034Sink    string
	ps5034Padded  = " \t\r\n  status=ok path=/api/v1/items latency=42ms region=eu \t\r\n "
	ps5034Trimmed = "status=ok path=/api/v1/items latency=42ms region=eu"
	ps5034NBSP    = "\u00a0\u00a0status=ok path=/api/v1/items latency=42ms\u00a0\u00a0"
)

func BenchmarkPS5034Padded_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5034Sink = strings.TrimFunc(ps5034Padded, unicode.IsSpace)
	}
}

func BenchmarkPS5034Padded_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5034Sink = strings.TrimSpace(ps5034Padded)
	}
}

func BenchmarkPS5034Trimmed_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5034Sink = strings.TrimFunc(ps5034Trimmed, unicode.IsSpace)
	}
}

func BenchmarkPS5034Trimmed_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5034Sink = strings.TrimSpace(ps5034Trimmed)
	}
}

func BenchmarkPS5034NBSP_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5034Sink = strings.TrimFunc(ps5034NBSP, unicode.IsSpace)
	}
}

func BenchmarkPS5034NBSP_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5034Sink = strings.TrimSpace(ps5034NBSP)
	}
}

package benchmarks

import (
	"bytes"
	"testing"
	"unicode"
)

// PS3031 — bytes.TrimFunc(b, unicode.IsSpace) vs bytes.TrimSpace(b).
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
// spellings return a subslice: 0 B/op, 0 allocs/op everywhere.
var (
	ps3031Sink    []byte
	ps3031Padded  = []byte(" \t\r\n  status=ok path=/api/v1/items latency=42ms region=eu \t\r\n ")
	ps3031Trimmed = []byte("status=ok path=/api/v1/items latency=42ms region=eu")
	ps3031NBSP    = []byte("\u00a0\u00a0status=ok path=/api/v1/items latency=42ms\u00a0\u00a0")
)

func BenchmarkPS3031Padded_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3031Sink = bytes.TrimFunc(ps3031Padded, unicode.IsSpace)
	}
}

func BenchmarkPS3031Padded_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3031Sink = bytes.TrimSpace(ps3031Padded)
	}
}

func BenchmarkPS3031Trimmed_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3031Sink = bytes.TrimFunc(ps3031Trimmed, unicode.IsSpace)
	}
}

func BenchmarkPS3031Trimmed_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3031Sink = bytes.TrimSpace(ps3031Trimmed)
	}
}

func BenchmarkPS3031NBSP_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3031Sink = bytes.TrimFunc(ps3031NBSP, unicode.IsSpace)
	}
}

func BenchmarkPS3031NBSP_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3031Sink = bytes.TrimSpace(ps3031NBSP)
	}
}

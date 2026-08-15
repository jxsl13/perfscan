package benchmarks

import (
	"bytes"
	"testing"
)

// PS2026 — len(buf.String()) vs buf.Len() on a bytes.Buffer.
// Buffer.String heap-allocates a fresh string and byte-copies the ENTIRE
// unread contents (O(n) time, O(n) allocation) which len() then throws
// away; Buffer.Len is a single integer subtraction. The buffer holds
// 64 KiB of log-line content, so the Before side is one 64 KiB
// malloc+memcpy per iteration and the gap scales linearly with buffer
// size.
var (
	ps2026Buf = func() *bytes.Buffer {
		var b bytes.Buffer
		for b.Len() < 64*1024 {
			b.WriteString("service=checkout región=eu-wést-1 status=ok body=héllo, 世界!\n")
		}
		return &b
	}()
	ps2026Sink int
)

func BenchmarkPS2026_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2026Sink = len(ps2026Buf.String())
	}
}

func BenchmarkPS2026_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2026Sink = ps2026Buf.Len()
	}
}

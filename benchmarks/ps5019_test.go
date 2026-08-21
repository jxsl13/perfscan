package benchmarks

import (
	"bytes"
	"testing"
)

// PS5019 — bytes.Replace(b, old, new, bytes.Count(b, old)) vs
// bytes.ReplaceAll(b, old, new), the bytes twin of PS5012. The explicit
// Count is a full O(len(b)) scan whose result Replace immediately
// re-derives with its own internal Count, so the Before walks the input
// twice; the After walks it once. Allocations are identical by
// construction (the equiv suite pins that — bytes.Replace always
// returns one fresh copy, match or not), so the whole delta is the
// redundant scan. The input is a 4 KiB log payload with a match per
// line: every match forces the substring searcher to stop and restart,
// which is exactly the work the explicit Count duplicates in full
// before Replace re-does it.
var (
	ps5019Input = bytes.Repeat([]byte("level=info msg=\"served request\" path=/api/v1/items dur=1ms; "), 68)
	ps5019Old   = []byte("msg=")
	ps5019New   = []byte("message=")
	ps5019Sink  []byte
)

func BenchmarkPS5019_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5019Sink = bytes.Replace(ps5019Input, ps5019Old, ps5019New, bytes.Count(ps5019Input, ps5019Old))
	}
}

func BenchmarkPS5019_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5019Sink = bytes.ReplaceAll(ps5019Input, ps5019Old, ps5019New)
	}
}

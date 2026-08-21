package benchmarks

import (
	"strings"
	"testing"
)

// PS5012 — strings.Replace(s, old, new, strings.Count(s, old)) vs
// strings.ReplaceAll(s, old, new). The explicit Count is a full
// O(len(s)) scan whose result Replace immediately re-derives with its
// own internal Count, so the Before walks the input twice; the After
// walks it once. Allocations are identical by construction (the equiv
// suite pins that), so the whole delta is the redundant scan. The input
// is a 4 KiB log payload with a match per line: every match forces the
// substring searcher to stop and restart, which is exactly the work the
// explicit Count duplicates in full before Replace re-does it.
var ps5012Input = strings.Repeat("level=info msg=\"served request\" path=/api/v1/items dur=1ms; ", 68)

func BenchmarkPS5012_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.Replace(ps5012Input, "msg=", "message=", strings.Count(ps5012Input, "msg="))
	}
}

func BenchmarkPS5012_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.ReplaceAll(ps5012Input, "msg=", "message=")
	}
}

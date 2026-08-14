package benchmarks

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

// PS2135 — WriteString(string(b)) vs Write(b), through an interface the
// way writers are typically held (io.Writer + io.StringWriter). Behind a
// genuinely indirect call the argument escapes, so every string(b)
// heap-allocates and copies the slice before WriteString copies it again;
// Write copies straight from the slice. (The mirror of PS2111: there the
// wasted copy was []byte(s) before Write, here it is string(b) before
// WriteString.) The constructor is kept out of sight of the inliner so
// the call cannot devirtualize.
type ps2135Writer interface {
	Write(p []byte) (int, error)
	WriteString(s string) (int, error)
}

//go:noinline
func ps2135NewWriter(buf *bytes.Buffer) ps2135Writer { return buf }

var ps2135Lines = func() [][]byte {
	out := make([][]byte, 64)
	for i := range out {
		out[i] = []byte(strings.Repeat("x", 80) + "-" + strconv.Itoa(i))
	}
	return out
}()

func BenchmarkPS2135_Before(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	w := ps2135NewWriter(&buf)
	for range b.N {
		buf.Reset()
		for _, p := range ps2135Lines {
			w.WriteString(string(p))
		}
		sinkI = buf.Len()
	}
}

func BenchmarkPS2135_After(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	w := ps2135NewWriter(&buf)
	for range b.N {
		buf.Reset()
		for _, p := range ps2135Lines {
			w.Write(p)
		}
		sinkI = buf.Len()
	}
}

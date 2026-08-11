package benchmarks

import (
	"bytes"
	"io"
	"strconv"
	"strings"
	"testing"
)

// PS2118 — io.WriteString(w, string(b)) vs w.Write(b), with the writer
// held as a plain io.Writer the way it is typically passed around. The
// string(b) conversion heap-allocates and copies the slice on every call
// (its result escapes into io.WriteString); w.Write(b) hands the writer
// the original bytes with no copy. The constructor is kept out of sight
// of the inliner so the interface call stays genuinely indirect.

//go:noinline
func ps2118NewWriter(buf *bytes.Buffer) io.Writer { return buf }

var ps2118Lines = func() [][]byte {
	out := make([][]byte, 64)
	for i := range out {
		out[i] = []byte(strings.Repeat("x", 80) + "-" + strconv.Itoa(i))
	}
	return out
}()

func BenchmarkPS2118_Before(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	w := ps2118NewWriter(&buf)
	for range b.N {
		buf.Reset()
		for _, line := range ps2118Lines {
			//lint:ignore SA6006 this benchmark deliberately measures the io.WriteString(w, string(b)) anti-pattern that PS2118 fixes
			io.WriteString(w, string(line))
		}
		sinkI = buf.Len()
	}
}

func BenchmarkPS2118_After(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	w := ps2118NewWriter(&buf)
	for range b.N {
		buf.Reset()
		for _, line := range ps2118Lines {
			w.Write(line)
		}
		sinkI = buf.Len()
	}
}

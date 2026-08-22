package benchmarks

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

// PS2111 — Write([]byte(s)) vs WriteString(s), through an interface the
// way writers are typically held (io.Writer + io.StringWriter). Behind a
// genuinely indirect call the argument escapes, so every []byte(s)
// heap-allocates and copies the string before Write copies it again;
// WriteString copies straight from the string. (When the compiler can
// devirtualize to a concrete *bytes.Buffer it already elides the copy
// for non-escaping arguments — the opaque-writer case is where the
// allocation really bites, so the constructor is kept out of sight of
// the inliner.)
type ps2111Writer interface {
	Write(p []byte) (int, error)
	WriteString(s string) (int, error)
}

//go:noinline
func ps2111NewWriter(buf *bytes.Buffer) ps2111Writer { return buf }

var ps2111Lines = func() []string {
	out := make([]string, 64)
	for i := range out {
		out[i] = strings.Repeat("x", 80) + "-" + strconv.Itoa(i)
	}
	return out
}()

func BenchmarkPS2111_Before(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	w := ps2111NewWriter(&buf)
	for range b.N {
		buf.Reset()
		for _, s := range ps2111Lines {
			w.Write([]byte(s))
		}
		sinkI = buf.Len()
	}
}

func BenchmarkPS2111_After(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	w := ps2111NewWriter(&buf)
	for range b.N {
		buf.Reset()
		for _, s := range ps2111Lines {
			w.WriteString(s)
		}
		sinkI = buf.Len()
	}
}

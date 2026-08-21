package checks

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

// Runtime differential for PS5070: w.WriteString(string(b)) versus
// w.Write(b). The fix's safety argument is that the io.StringWriter
// contract requires WriteString to write the same bytes as Write, and
// string(b) holds exactly b's bytes, so the two forms write the identical
// output and return the identical (int, error) — the byte count is the
// length either way. This suite pins that over an adversarial []byte set
// (empty, nil, single byte, embedded NUL, invalid UTF-8, unicode) against
// each standard receiver the check accepts — bytes.Buffer, strings.Builder,
// bufio.Writer — confirming both the written bytes and the returned count
// agree.
func TestEquiv_PS5070WriteStringOfByteConv(t *testing.T) {
	inputs := [][]byte{
		nil, {}, []byte("a"), []byte("hello world"),
		{0x00, 0x01, 0x02}, {0xff, 0xfe, 0x00, 'a'},
		[]byte("日本語テスト"), bytes.Repeat([]byte("x"), 4096),
	}

	for _, b := range inputs {
		// bytes.Buffer
		var b1, b2 bytes.Buffer
		n1, e1 := b1.WriteString(string(b))
		n2, e2 := b2.Write(b)
		if n1 != n2 || e1 != e2 || b1.String() != b2.String() {
			t.Fatalf("bytes.Buffer diverged for %q: (%d,%v)=%q vs (%d,%v)=%q", b, n1, e1, b1.String(), n2, e2, b2.String())
		}

		// strings.Builder
		var s1, s2 strings.Builder
		m1, f1 := s1.WriteString(string(b))
		m2, f2 := s2.Write(b)
		if m1 != m2 || f1 != f2 || s1.String() != s2.String() {
			t.Fatalf("strings.Builder diverged for %q", b)
		}

		// bufio.Writer over a bytes.Buffer sink
		var w1, w2 bytes.Buffer
		bw1 := bufio.NewWriter(&w1)
		bw2 := bufio.NewWriter(&w2)
		k1, g1 := bw1.WriteString(string(b))
		k2, g2 := bw2.Write(b)
		if err := bw1.Flush(); err != nil {
			t.Fatal(err)
		}
		if err := bw2.Flush(); err != nil {
			t.Fatal(err)
		}
		if k1 != k2 || g1 != g2 || w1.String() != w2.String() {
			t.Fatalf("bufio.Writer diverged for %q", b)
		}
	}
}

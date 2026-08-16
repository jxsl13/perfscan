package checks

import (
	"bytes"
	"slices"
	"testing"
)

// TestEquivPS5050 proves the rewrite is byte-identical: slices.Index(b, c) and
// bytes.IndexByte(b, c) return the same index, and slices.Contains(b, c) and
// bytes.IndexByte(b, c) >= 0 the same membership, for every byte value across
// adversarial haystacks (empty, nil, match at either end, repeats, NUL bytes,
// invalid UTF-8).
func TestEquivPS5050(t *testing.T) {
	haystacks := [][]byte{
		nil,
		{},
		[]byte("hello world"),
		[]byte("aaaa"),
		{0x00, 0xff, 0x7f, 0x80},
		{0xff, 0xfe, 0xfd}, // invalid UTF-8
		bytes.Repeat([]byte("xy"), 500),
		append([]byte{'z'}, bytes.Repeat([]byte("q"), 300)...), // match at head
		append(bytes.Repeat([]byte("q"), 300), 'z'),            // match at tail
	}
	for _, h := range haystacks {
		for c := 0; c < 256; c++ {
			b := byte(c)
			if got, want := bytes.IndexByte(h, b), slices.Index(h, b); got != want {
				t.Fatalf("Index mismatch h=%q c=%d: bytes.IndexByte=%d slices.Index=%d", h, b, got, want)
			}
			if got, want := bytes.IndexByte(h, b) >= 0, slices.Contains(h, b); got != want {
				t.Fatalf("Contains mismatch h=%q c=%d: (IndexByte>=0)=%v slices.Contains=%v", h, b, got, want)
			}
		}
	}
}

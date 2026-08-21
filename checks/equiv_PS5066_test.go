package checks

import (
	"bytes"
	"math/rand"
	"testing"
)

// TestEquivPS5066 proves the rewrite reads the identical byte: buf.String()[i]
// and buf.Bytes()[i] expose the same unread region, so every in-bounds index is
// the same byte value — including after reads have advanced the buffer's read
// offset (String and Bytes both report only the unread portion).
func TestEquivPS5066(t *testing.T) {
	chk := func(content []byte, consume int) {
		var buf bytes.Buffer
		buf.Write(content)
		tmp := make([]byte, consume)
		buf.Read(tmp) // advance the read offset
		s := buf.String()
		b := buf.Bytes()
		if len(s) != len(b) {
			t.Fatalf("len mismatch: String=%d Bytes=%d", len(s), len(b))
		}
		for i := range b {
			if s[i] != b[i] {
				t.Fatalf("byte %d: String=%d Bytes=%d", i, s[i], b[i])
			}
		}
	}
	for i := 0; i < 20000; i++ {
		r := rand.New(rand.NewSource(int64(i)))
		n := r.Intn(40)
		content := make([]byte, n)
		for j := range content {
			content[j] = byte(r.Intn(256))
		}
		consume := 0
		if n > 0 {
			consume = r.Intn(n + 1)
		}
		chk(content, consume)
	}
}

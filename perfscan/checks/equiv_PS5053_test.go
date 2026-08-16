package checks

import (
	"math/rand"
	"strconv"
	"testing"
)

// TestEquivPS5053 proves the rewrite is byte-identical: strconv.Quote is a
// bijection, so Quote(a) == Quote(b) equals a == b (and != likewise) for every
// pair of strings, including embedded quotes, backslashes, control bytes, and
// invalid UTF-8.
func TestEquivPS5053(t *testing.T) {
	corpus := []string{
		"", "a", "ab", "a\"b", "a\\b", "tab\there", "nl\n", "é", "😀",
		"\x00\x01\x7f", "\xff\xfe", "quote\"quote", "back`tick", "  ", "A",
	}
	chk := func(a, b string) {
		if (strconv.Quote(a) == strconv.Quote(b)) != (a == b) {
			t.Fatalf("== a=%q b=%q", a, b)
		}
		if (strconv.Quote(a) != strconv.Quote(b)) != (a != b) {
			t.Fatalf("!= a=%q b=%q", a, b)
		}
	}
	for _, a := range corpus {
		for _, b := range corpus {
			chk(a, b)
		}
	}
	rnd := func(seed int) string {
		r := rand.New(rand.NewSource(int64(seed)))
		bs := make([]byte, r.Intn(8))
		for i := range bs {
			bs[i] = byte(r.Intn(256))
		}
		return string(bs)
	}
	for i := 0; i < 20000; i++ {
		chk(rnd(i), rnd(i*2654435761+1))
	}
}

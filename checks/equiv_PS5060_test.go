package checks

import (
	"fmt"
	"strconv"
	"testing"
)

// TestEquivPS5060 proves the rewrite is byte-identical: fmt.Appendf of one %t
// or %q verb spliced into literal text produces exactly what the nested
// append/strconv.AppendBool|AppendQuote chain writes, over bools and adversarial
// strings.
func TestEquivPS5060(t *testing.T) {
	seed := []byte("seed>")
	for _, b := range []bool{true, false} {
		want := fmt.Appendf(append([]byte(nil), seed...), "ok=%t;", b)
		got := append(strconv.AppendBool(append(append([]byte(nil), seed...), "ok="...), b), ";"...)
		if string(want) != string(got) {
			t.Fatalf("%%t b=%v: appendf=%q chain=%q", b, want, got)
		}
	}
	for _, s := range []string{"", "a", "a\"b", "a\\b", "tab\there", "nl\n", "é", "😀", "\x00\xff", "quote\"q"} {
		want := fmt.Appendf(append([]byte(nil), seed...), "s=%q!", s)
		got := append(strconv.AppendQuote(append(append([]byte(nil), seed...), "s="...), s), "!"...)
		if string(want) != string(got) {
			t.Fatalf("%%q s=%q: appendf=%q chain=%q", s, want, got)
		}
	}
}

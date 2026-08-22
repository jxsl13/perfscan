package checks

import (
	"fmt"
	"strconv"
	"testing"
)

// TestEquivPS5041 proves the rewrite is byte-identical: fmt.Appendf(dst,
// "%q", s) and strconv.AppendQuote(dst, s) append the same double-quoted Go
// string literal. fmt's "%q" over a string is defined as strconv.AppendQuote,
// so the two agree for every input — adversarial strings, every single byte,
// and every rune.
func TestEquivPS5041(t *testing.T) {
	check := func(s string) {
		a := fmt.Appendf([]byte("seed"), "%q", s)
		b := strconv.AppendQuote([]byte("seed"), s)
		if string(a) != string(b) {
			t.Fatalf("s=%#v: fmt=%s strconv=%s", s, a, b)
		}
	}
	for _, s := range []string{
		"", "a", "hello world", "tab\tnl\nret\r", "quote\"backslash\\",
		"unicode 世界 € 😀", "\x00\x01\x7f", "\xff\xfe\x80 invalid", "\a\b\f\v",
		"'single'", "mixed 世\xffz", `raw`,
	} {
		check(s)
	}
	for i := 0; i < 256; i++ {
		check(string([]byte{byte(i)}))
		check("x" + string([]byte{byte(i)}) + "y")
	}
	for r := rune(0); r <= 0x10FFFF; r++ {
		check(string(r))
	}
}

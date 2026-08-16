package checks

import (
	"fmt"
	"testing"
)

// TestEquivPS5044 proves the rewrite is byte-identical: for a plain string,
// fmt.Appendf(buf, "%v", s) and append(buf, s...) append the same bytes. "%v"
// over a predeclared string writes its bytes verbatim (no quoting, no spacing),
// exactly what append's string-to-[]byte fast path does.
func TestEquivPS5044(t *testing.T) {
	check := func(s string) {
		a := fmt.Appendf([]byte("seed"), "%v", s)
		b := append([]byte("seed"), s...)
		if string(a) != string(b) {
			t.Fatalf("s=%#v: appendf=%q append=%q", s, a, b)
		}
	}
	for _, s := range []string{
		"", "a", "hello world", "tab\tnl\n", "quote\"back\\", "unicode 世界 € 😀",
		"\x00\x01\x7f", "\xff\xfe\x80 invalid", "percent %s %v %d literal", "'x'",
	} {
		check(s)
	}
	for i := 0; i < 256; i++ {
		check(string([]byte{byte(i)}))
		check("x" + string([]byte{byte(i)}) + "y")
	}
	for r := rune(0); r <= 0x2FFFF; r++ {
		check(string(r))
	}
}

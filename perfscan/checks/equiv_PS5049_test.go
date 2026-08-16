package checks

import (
	"fmt"
	"testing"
)

// TestEquivPS5049 proves the rewrite is byte-identical: appending a spread
// fmt.Sprint{f,,ln} string to dst produces exactly what fmt.Append{f,,ln}
// writes into dst — across the format, verbless, and newline forms, the empty
// case, and even when a formatted operand aliases dst's contents.
func TestEquivPS5049(t *testing.T) {
	seeds := [][]byte{nil, {}, []byte("x="), []byte("a longer existing prefix ")}

	sprintf := func(seed []byte, format string, a ...any) {
		want := append(append([]byte(nil), seed...), fmt.Sprintf(format, a...)...)
		got := fmt.Appendf(append([]byte(nil), seed...), format, a...)
		if string(want) != string(got) {
			t.Fatalf("Sprintf %q%v: append=%q Appendf=%q", format, a, want, got)
		}
	}
	sprint := func(seed []byte, a ...any) {
		want := append(append([]byte(nil), seed...), fmt.Sprint(a...)...)
		got := fmt.Append(append([]byte(nil), seed...), a...)
		if string(want) != string(got) {
			t.Fatalf("Sprint %v: append=%q Append=%q", a, want, got)
		}
	}
	sprintln := func(seed []byte, a ...any) {
		want := append(append([]byte(nil), seed...), fmt.Sprintln(a...)...)
		got := fmt.Appendln(append([]byte(nil), seed...), a...)
		if string(want) != string(got) {
			t.Fatalf("Sprintln %v: append=%q Appendln=%q", a, want, got)
		}
	}

	for _, s := range seeds {
		sprintf(s, "literal")
		sprintf(s, "user=%d", 42)
		sprintf(s, "%s/%d/%v", "path", -7, []int{1, 2})
		sprintf(s, "%q %x %08b", "q\"", 255, 5)
		sprint(s)
		sprint(s, 1, 2, "z")
		sprint(s, "a", "b", 3.5, true, nil)
		sprintln(s)
		sprintln(s, "a", "b")
		sprintln(s, 1, 2, 3)
	}

	// Self-aliasing operand: %s of dst's own contents, formatted directly into
	// dst's tail, must still read the original bytes.
	for _, s := range seeds {
		want := append(append([]byte(nil), s...), fmt.Sprintf("[%s]", s)...)
		got := fmt.Appendf(append([]byte(nil), s...), "[%s]", append([]byte(nil), s...))
		if string(want) != string(got) {
			t.Fatalf("self-alias %q: append=%q Appendf=%q", s, want, got)
		}
	}
}

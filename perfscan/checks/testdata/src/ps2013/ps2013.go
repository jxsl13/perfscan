package ps2013

import (
	"fmt"
	"strings"
)

const oldKey = "the"

// Basic form: constant literals, plain identifier argument.
func basic(s string) string {
	return strings.NewReplacer("the", "THE").Replace(s) // want `strings\.NewReplacer with a single constant pair`
}

// The s argument is kept byte-verbatim, however it is spelled: a field
// selector and a compound expression with a call (evaluated exactly once
// in both forms).
func verbatim(w struct{ line string }, f func() string) {
	fmt.Println(strings.NewReplacer("a", "b").Replace(w.line))    // want `strings\.NewReplacer with a single constant pair`
	fmt.Println(strings.NewReplacer("a", "b").Replace("x" + f())) // want `strings\.NewReplacer with a single constant pair`
}

// A named constant and a constant concatenation are compile-time constant
// strings; the pair is re-rendered after s.
func constPair(s string) string {
	out := strings.NewReplacer(oldKey, "THE").Replace(s) // want `strings\.NewReplacer with a single constant pair`
	return strings.NewReplacer("t"+"he", "T").Replace(out) // want `strings\.NewReplacer with a single constant pair`
}

// A raw-string pair keeps its backtick spelling.
func rawPair(s string) string {
	return strings.NewReplacer(`th`, `TH`).Replace(s) // want `strings\.NewReplacer with a single constant pair`
}

// A parenthesized receiver is unwrapped by the match and swallowed by the
// rewrite.
func parenthesized(s string) string {
	return (strings.NewReplacer("a", "b")).Replace(s) // want `strings\.NewReplacer with a single constant pair`
}

// Nested matches: the outer site's edits surround the verbatim s span, the
// inner site's edits sit wholly inside it — both rewrite.
func nested(s string) string {
	return strings.NewReplacer("a", "b").Replace(strings.NewReplacer("c", "d").Replace(s)) // want `strings\.NewReplacer with a single constant pair` `strings\.NewReplacer with a single constant pair`
}

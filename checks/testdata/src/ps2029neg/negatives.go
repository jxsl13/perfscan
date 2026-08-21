package ps2029neg

import (
	"bytes"
	"strings"
)

// A variable separator is never matched: the empty separator is the
// one input where the identity breaks (SplitN rune-explodes and its
// length tracks the rune count while Contains(s, "") is always true),
// and a variable cannot be proven non-empty.
func variableSep(s, sep string, b, bsep []byte) {
	_ = len(strings.SplitN(s, sep, 2)) == 2
	_ = len(bytes.SplitN(b, bsep, 2)) == 2
}

// The provably EMPTY separator is the divergence shape itself and is
// excluded, in every spelling.
const emptyConst = ""

func emptySep(s string, b []byte) {
	_ = len(strings.SplitN(s, "", 2)) == 2
	_ = len(strings.SplitN(s, emptyConst, 2)) == 2
	_ = len(bytes.SplitN(b, []byte(""), 2)) == 2
	_ = len(bytes.SplitN(b, []byte{}, 2)) == 2
	_ = len(bytes.SplitN(b, nil, 2)) == 2
}

// The limit must be the LITERAL 2: any other limit changes the length
// algebra, and a variable or named constant holding 2 is not matched.
const two = 2

func limits(s string, n int) {
	_ = len(strings.SplitN(s, ",", 3)) == 2
	_ = len(strings.SplitN(s, ",", -1)) == 2
	_ = len(strings.SplitN(s, ",", 1)) == 1
	_ = len(strings.SplitN(s, ",", n)) == 2
	_ = len(strings.SplitN(s, ",", two)) == 2
}

// Comparisons that are not one of the eight membership shapes stay
// silent — they are either constant-foldable trivia (the length is
// always 1 or 2) or genuinely inspect the count.
func counting(s string) {
	_ = len(strings.SplitN(s, ",", 2)) == 0
	_ = len(strings.SplitN(s, ",", 2)) >= 1
	_ = len(strings.SplitN(s, ",", 2)) <= 2
	_ = len(strings.SplitN(s, ",", 2)) > 2
	_ = len(strings.SplitN(s, ",", 2)) < 1
	_ = len(strings.SplitN(s, ",", 2)) > 0
	_ = 3 == len(strings.SplitN(s, ",", 2))
	_ = 0 != len(strings.SplitN(s, ",", 2))
}

// A comparison against a non-literal (a variable or a named constant
// holding 2) is not matched, and a bare len(SplitN(...)) value that is
// not compared stays silent.
func nonLiteral(s string, n int) int {
	_ = len(strings.SplitN(s, ",", 2)) == n
	_ = len(strings.SplitN(s, ",", 2)) == two
	_ = n >= len(strings.SplitN(s, ",", 2))
	return len(strings.SplitN(s, ",", 2))
}

// A SplitN slice stored in a variable first may have other consumers.
func stored(s string) []string {
	parts := strings.SplitN(s, ",", 2)
	if len(parts) == 2 {
		return parts
	}
	return nil
}

// Split and SplitAfter have no limit (PS2121's territory), and
// SplitAfterN is out of scope here.
func otherFuncs(s string) {
	_ = len(strings.SplitAfterN(s, ",", 2)) == 2
}

// The comparison is an untyped bool and adopts whatever boolean type
// its context demands; Contains returns the basic type bool, so a
// context that materialized a named bool type is skipped entirely —
// the rewrite would not compile there.
type myBool bool

func namedBoolContext(s string) myBool {
	var b myBool = len(strings.SplitN(s, ",", 2)) == 2
	return b
}

// A local function, a method, or a field named SplitN — and a shadowed
// strings identifier — do not resolve to the stdlib strings.SplitN.
type splitter struct{}

func (splitter) SplitN(s, sep string, n int) []string { return nil }

func notStdlib(sp splitter, s string) {
	splitN := func(string, string, int) []string { return nil }
	_ = len(splitN(s, ",", 2)) == 2
	_ = len(sp.SplitN(s, ",", 2)) == 2
	strings := sp
	_ = len(strings.SplitN(s, ",", 2)) == 2
}

// A shadowed len is not the builtin.
func shadowedLen(s string) {
	len := func([]string) int { return 0 }
	_ = len(strings.SplitN(s, ",", 2)) == 2
}

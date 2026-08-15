package ps2028neg

import (
	"bytes"
	"strings"
)

// Comparisons that genuinely need the field count stay silent — there
// is no cheap TrimSpace equivalent for "exactly one field" or "at
// least two fields".
func counting(s string) {
	_ = len(strings.Fields(s)) == 1
	_ = len(strings.Fields(s)) != 1
	_ = len(strings.Fields(s)) > 1
	_ = len(strings.Fields(s)) >= 2
	_ = len(strings.Fields(s)) <= 1
	_ = len(strings.Fields(s)) < 2
	_ = 2 == len(strings.Fields(s))
	_ = 1 < len(strings.Fields(s))
}

// A Fields slice stored in a variable first may have other consumers.
func stored(s string) []string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// A bare len(Fields(...)) value that is not compared, or compared to a
// non-literal (a variable or a named constant holding 0), stays silent:
// the check only rewrites the spelling that is provably a blank test
// against the untyped literal.
const zero = 0

func nonLiteral(s string, n int) int {
	_ = len(strings.Fields(s)) == n
	_ = len(strings.Fields(s)) == zero
	_ = n >= len(strings.Fields(s))
	return len(strings.Fields(s))
}

// bytes.Fields is out of scope: []byte has no == "" comparison, so its
// blank test has no drop-in TrimSpace spelling.
func bytesFields(b []byte) {
	_ = len(bytes.Fields(b)) == 0
	_ = len(bytes.Fields(b)) > 0
}

// FieldsFunc takes a predicate with an arbitrary rune class — TrimSpace
// answers a different question — and FieldsSeq has no len at all.
func fieldsFunc(s string) {
	_ = len(strings.FieldsFunc(s, func(r rune) bool { return r == ',' })) == 0
}

// A local function, a method, or a field named Fields — and a shadowed
// strings identifier — do not resolve to the stdlib strings.Fields.
type form struct{}

func (form) Fields(s string) []string { return nil }

func notStdlib(f form, s string) {
	fields := func(string) []string { return nil }
	_ = len(fields(s)) == 0
	_ = len(f.Fields(s)) == 0
	strings := f
	_ = len(strings.Fields(s)) == 0
}

// A shadowed len is not the builtin.
func shadowedLen(s string) {
	len := func([]string) int { return 0 }
	_ = len(strings.Fields(s)) == 0
}

// len of something other than a direct Fields call stays silent.
func otherLen(s string) {
	_ = len(s) == 0
	_ = len([]rune(s)) == 0
}

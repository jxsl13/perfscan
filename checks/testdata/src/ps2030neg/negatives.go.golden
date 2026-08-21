package ps2030neg

import (
	"bytes"
	"strings"
)

// Comparisons that genuinely need the field count stay silent — after
// the rename len would count BYTES, not fields, so "exactly one
// field" or "at least two fields" has no TrimSpace equivalent.
func counting(b []byte) {
	_ = len(bytes.Fields(b)) == 1
	_ = len(bytes.Fields(b)) != 1
	_ = len(bytes.Fields(b)) > 1
	_ = len(bytes.Fields(b)) >= 2
	_ = len(bytes.Fields(b)) <= 1
	_ = len(bytes.Fields(b)) < 2
	_ = 2 == len(bytes.Fields(b))
	_ = 1 < len(bytes.Fields(b))
}

// A Fields slice stored in a variable first may have other consumers.
func stored(b []byte) [][]byte {
	fields := bytes.Fields(b)
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

func nonLiteral(b []byte, n int) int {
	_ = len(bytes.Fields(b)) == n
	_ = len(bytes.Fields(b)) == zero
	_ = n >= len(bytes.Fields(b))
	return len(bytes.Fields(b))
}

// strings.Fields is PS2028's territory: there the whole len
// scaffolding collapses to a == "" comparison instead.
func stringsFields(s string) {
	_ = len(strings.Fields(s)) == 0
	_ = len(strings.Fields(s)) > 0
}

// FieldsFunc takes a predicate with an arbitrary rune class — TrimSpace
// answers a different question — and FieldsSeq has no len at all.
func fieldsFunc(b []byte) {
	_ = len(bytes.FieldsFunc(b, func(r rune) bool { return r == ',' })) == 0
}

// A local function, a method, or a field named Fields — and a shadowed
// bytes identifier — do not resolve to the stdlib bytes.Fields.
type form struct{}

func (form) Fields(b []byte) [][]byte { return nil }

func notStdlib(f form, b []byte) {
	fields := func([]byte) [][]byte { return nil }
	_ = len(fields(b)) == 0
	_ = len(f.Fields(b)) == 0
	bytes := f
	_ = len(bytes.Fields(b)) == 0
}

// A shadowed len is not the builtin.
func shadowedLen(b []byte) {
	len := func([][]byte) int { return 0 }
	_ = len(bytes.Fields(b)) == 0
}

// len of something other than a direct Fields call stays silent.
func otherLen(b []byte) {
	_ = len(b) == 0
	_ = len(bytes.TrimSpace(b)) == 0
}

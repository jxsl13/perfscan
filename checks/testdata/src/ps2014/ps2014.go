package ps2014

import (
	"bytes"
	"strings"
)

// Literal separator, constant index 2: the classic early-field read.
func third(s string) string {
	f := strings.Split(s, ",")[2] // want `strings\.Split\(\.\.\.\)\[2\] scans the whole input and allocates every piece just to read one field; strings\.SplitN\(\.\.\., 4\)\[2\] stops after that field and is bit-identical`
	return f
}

// Index 1: the smallest index in scope (0 is PS2009's domain).
func second(s string) string {
	return strings.Split(s, "/")[1] // want `strings\.Split\(\.\.\.\)\[1\] scans the whole input and allocates every piece just to read one field; strings\.SplitN\(\.\.\., 3\)\[1\] stops after that field and is bit-identical`
}

// A VARIABLE separator is fine — the identity holds for every separator
// value, including the empty string.
func varSep(s, sep string) string {
	return strings.Split(s, sep)[3] // want `strings\.Split\(\.\.\.\)\[3\] scans the whole input and allocates every piece just to read one field; strings\.SplitN\(\.\.\., 5\)\[3\] stops after that field and is bit-identical`
}

// The EMPTY separator is fine too: both forms rune-explode and piece i
// is the i-th rune in both (the remainder is piece i+1, never read).
func emptySep(s string) string {
	return strings.Split(s, "")[2] // want `strings\.Split\(\.\.\.\)\[2\] scans the whole input and allocates every piece just to read one field; strings\.SplitN\(\.\.\., 4\)\[2\] stops after that field and is bit-identical`
}

// Parenthesized operand: still a direct index of the call.
func parenthesized(s string) string {
	return (strings.Split(s, ","))[2] // want `strings\.Split\(\.\.\.\)\[2\] scans the whole input and allocates every piece just to read one field; strings\.SplitN\(\.\.\., 4\)\[2\] stops after that field and is bit-identical`
}

// A named constant that evaluates to a small positive index is the same
// index; the fix inserts the computed literal n = i+2.
const fieldIdx = 5

func namedIdx(s string) string {
	return strings.Split(s, ",")[fieldIdx] // want `strings\.Split\(\.\.\.\)\[5\] scans the whole input and allocates every piece just to read one field; strings\.SplitN\(\.\.\., 7\)\[5\] stops after that field and is bit-identical`
}

// A comment after the separator argument survives: the fix INSERTS
// ", 4" directly after the argument instead of rewriting the span up
// to the closing parenthesis.
func commented(s string) string {
	return strings.Split(s, "," /* sep */)[2] // want `strings\.Split\(\.\.\.\)\[2\] scans the whole input and allocates every piece just to read one field; strings\.SplitN\(\.\.\., 4\)\[2\] stops after that field and is bit-identical`
}

// The upper bound of the scope: index 16 rewrites to SplitN(..., 18).
func atCap(s string) string {
	return strings.Split(s, ",")[16] // want `strings\.Split\(\.\.\.\)\[16\] scans the whole input and allocates every piece just to read one field; strings\.SplitN\(\.\.\., 18\)\[16\] stops after that field and is bit-identical`
}

// bytes twin: piece i is the same capped subslice of the same backing
// array (identical len, cap, aliasing, nil-ness).
func bytesThird(b []byte) []byte {
	return bytes.Split(b, []byte(","))[2] // want `bytes\.Split\(\.\.\.\)\[2\] scans the whole input and allocates every piece just to read one field; bytes\.SplitN\(\.\.\., 4\)\[2\] stops after that field and is bit-identical`
}

// --- guards: none of the following may be reported or rewritten ---

// [0] is PS2009's domain — this check never matches it, so the two
// checks are disjoint and never double-report.
func head(s string) string {
	return strings.Split(s, ",")[0]
}

// A variable index may take any value at run time.
func varIndex(s string, i int) string {
	return strings.Split(s, ",")[i]
}

// Beyond the cap: a huge constant index shrinks the win and suggests
// the code wants most pieces anyway — out of scope.
func beyondCap(s string) string {
	return strings.Split(s, ",")[17]
}

// The result stored first may have other consumers — out of scope.
func storedFirst(s string) string {
	parts := strings.Split(s, ",")
	return parts[2]
}

// Already limited: nothing to gain.
func alreadyN(s string) string {
	return strings.SplitN(s, ",", 4)[2]
}

// SplitAfter keeps the separator attached — a different function, out
// of scope for this check.
func splitAfter(s string) string {
	return strings.SplitAfter(s, ",")[2]
}

// A shadowed strings is not the strings package.
type fakeStrings struct{}

func (fakeStrings) Split(s, sep string) []string { return []string{s, s, s} }

func shadowedPkg(s string) string {
	strings := fakeStrings{}
	return strings.Split(s, ",")[2]
}

// Slicing (not indexing) the result is out of scope.
func sliced(s string) []string {
	return strings.Split(s, ",")[1:3]
}

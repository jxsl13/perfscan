package ps2029

import (
	"bytes"
	"strings"
	str "strings"
)

// Every membership-equivalent comparison shape is rewritten, with the
// literal on either side of the operator. The two-byte separator keeps
// the plain Contains form.
func forms(s string) {
	_ = len(strings.SplitN(s, "=>", 2)) == 2 // want `len\(strings\.SplitN\(s, "=>", 2\)\) == 2 allocates the up-to-two-piece slice just to test for the separator; strings\.Contains\(s, "=>"\) is the identical boolean with zero allocation`
	_ = len(strings.SplitN(s, "=>", 2)) >= 2 // want `len\(strings\.SplitN\(s, "=>", 2\)\) >= 2 allocates the up-to-two-piece slice just to test for the separator; strings\.Contains\(s, "=>"\)`
	_ = len(strings.SplitN(s, "=>", 2)) > 1  // want `len\(strings\.SplitN\(s, "=>", 2\)\) > 1 allocates the up-to-two-piece slice just to test for the separator; strings\.Contains\(s, "=>"\)`
	_ = len(strings.SplitN(s, "=>", 2)) != 1 // want `len\(strings\.SplitN\(s, "=>", 2\)\) != 1 allocates the up-to-two-piece slice just to test for the separator; strings\.Contains\(s, "=>"\)`
	_ = len(strings.SplitN(s, "=>", 2)) == 1 // want `len\(strings\.SplitN\(s, "=>", 2\)\) == 1 allocates the up-to-two-piece slice just to test for the separator; !strings\.Contains\(s, "=>"\)`
	_ = len(strings.SplitN(s, "=>", 2)) <= 1 // want `len\(strings\.SplitN\(s, "=>", 2\)\) <= 1 allocates the up-to-two-piece slice just to test for the separator; !strings\.Contains\(s, "=>"\)`
	_ = len(strings.SplitN(s, "=>", 2)) < 2  // want `len\(strings\.SplitN\(s, "=>", 2\)\) < 2 allocates the up-to-two-piece slice just to test for the separator; !strings\.Contains\(s, "=>"\)`
	_ = len(strings.SplitN(s, "=>", 2)) != 2 // want `len\(strings\.SplitN\(s, "=>", 2\)\) != 2 allocates the up-to-two-piece slice just to test for the separator; !strings\.Contains\(s, "=>"\)`
	_ = 2 == len(strings.SplitN(s, "=>", 2)) // want `2 == len\(strings\.SplitN\(s, "=>", 2\)\) allocates the up-to-two-piece slice just to test for the separator; strings\.Contains\(s, "=>"\)`
	_ = 2 <= len(strings.SplitN(s, "=>", 2)) // want `2 <= len\(strings\.SplitN\(s, "=>", 2\)\) allocates the up-to-two-piece slice just to test for the separator; strings\.Contains\(s, "=>"\)`
	_ = 1 >= len(strings.SplitN(s, "=>", 2)) // want `1 >= len\(strings\.SplitN\(s, "=>", 2\)\) allocates the up-to-two-piece slice just to test for the separator; !strings\.Contains\(s, "=>"\)`
	_ = 2 > len(strings.SplitN(s, "=>", 2))  // want `2 > len\(strings\.SplitN\(s, "=>", 2\)\) allocates the up-to-two-piece slice just to test for the separator; !strings\.Contains\(s, "=>"\)`
}

// An aliased import keeps its qualifier verbatim; Contains lives in
// the same package, so the import is never orphaned.
func aliased(s string) bool {
	return len(str.SplitN(s, "=>", 2)) == 2 // want `len\(str\.SplitN\(s, "=>", 2\)\) == 2 allocates the up-to-two-piece slice just to test for the separator; str\.Contains\(s, "=>"\)`
}

// A one-byte string literal separator chains to the IndexByte fixed
// point directly (emitting Contains would hand PS5016 its Before-shape
// and churn the next -fix pass). A multi-byte rune that is one RUNE but
// two BYTES stays a plain Contains, as does a one-byte NAMED constant
// (advisory-only in PS5016, so no churn).
const arrow = "=>"

const eq = "="

func oneByte(line string) {
	if len(strings.SplitN(line, "=", 2)) == 2 { // want `len\(strings\.SplitN\(line, "=", 2\)\) == 2 allocates the up-to-two-piece slice just to test for the separator; strings\.IndexByte\(line, "="\[0\]\) >= 0`
		return
	}
	_ = len(strings.SplitN(line, "=", 2)) == 1    // want `strings\.IndexByte\(line, "="\[0\]\) < 0`
	_ = len(strings.SplitN(line, "\xff", 2)) == 2 // want `strings\.IndexByte\(line, "\\xff"\[0\]\) >= 0`
	_ = len(strings.SplitN(line, "é", 2)) == 2    // want `strings\.Contains\(line, "é"\)`
	_ = len(strings.SplitN(line, arrow, 2)) == 2  // want `strings\.Contains\(line, arrow\)`
	_ = len(strings.SplitN(line, eq, 2)) == 2     // want `strings\.Contains\(line, eq\)`
}

// The bytes twin: multi-byte needles keep the plain Contains form; a
// one-element []byte{X} composite and a []byte("z") conversion of a
// one-byte literal chain to bytes.IndexByte (PS5014's fixed point),
// with X carried over verbatim and evaluated exactly once.
func bytesTwin(b []byte, sepByte byte, next func() byte) {
	_ = len(bytes.SplitN(b, []byte("=>"), 2)) == 2     // want `bytes\.Contains\(b, \[\]byte\("=>"\)\)`
	_ = len(bytes.SplitN(b, []byte{'a', 'b'}, 2)) != 1 // want `bytes\.Contains\(b, \[\]byte\{'a', 'b'\}\)`
	_ = len(bytes.SplitN(b, []byte{0: '\n'}, 2)) == 2  // want `bytes\.Contains\(b, \[\]byte\{0: '\\n'\}\)`
	_ = len(bytes.SplitN(b, []byte{'\n'}, 2)) == 2     // want `bytes\.IndexByte\(b, '\\n'\) >= 0`
	_ = len(bytes.SplitN(b, []byte("="), 2)) == 1      // want `bytes\.IndexByte\(b, "="\[0\]\) < 0`
	_ = len(bytes.SplitN(b, []byte{sepByte}, 2)) == 2  // want `bytes\.IndexByte\(b, sepByte\) >= 0`
	_ = len(bytes.SplitN(b, []byte{next()}, 2)) == 2   // want `bytes\.IndexByte\(b, next\(\)\) >= 0`
	_ = 1 != len(bytes.SplitN(b, []byte("=>"), 2))     // want `bytes\.Contains\(b, \[\]byte\("=>"\)\)`
}

// The argument expressions carry over byte-verbatim: struct fields,
// index expressions, calls, and redundant parens all stay exactly as
// written and are evaluated exactly once either way. A comment BETWEEN
// the arguments sits in kept text and survives the rewrite.
type payload struct{ text string }

func args(p payload, parts []string, get func() string) {
	if len(strings.SplitN(p.text, "=>", 2)) == 2 { // want `strings\.Contains\(p\.text, "=>"\)`
		return
	}
	for len(strings.SplitN(parts[0], "=>", 2)) == 1 { // want `!strings\.Contains\(parts\[0\], "=>"\)`
		break
	}
	_ = len(strings.SplitN(get(), "=>", 2)) == 2            // want `strings\.Contains\(get\(\), "=>"\)`
	_ = len(strings.SplitN(p.text /* mid */, "=>", 2)) == 2 // want `allocates the up-to-two-piece slice`
}

// Redundant parens around the len call, the SplitN call, the SplitN
// selector, the separator, or the limit are scaffolding and are
// consumed with it (the kept separator keeps its own parens).
func parens(s string) {
	_ = (len(strings.SplitN(s, "=>", 2))) == 2 // want `allocates the up-to-two-piece slice`
	_ = len((strings.SplitN(s, "=>", 2))) > 1  // want `allocates the up-to-two-piece slice`
	_ = len((strings.SplitN)(s, "=>", 2)) == 2 // want `allocates the up-to-two-piece slice`
	_ = len(strings.SplitN(s, "=>", (2))) == 2 // want `allocates the up-to-two-piece slice`
	_ = len(strings.SplitN(s, ("=>"), 2)) == 2 // want `allocates the up-to-two-piece slice`
	_ = len(strings.SplitN(s, ("="), 2)) == 2  // want `allocates the up-to-two-piece slice`
	_ = (2) == len(strings.SplitN(s, "=>", 2)) // want `allocates the up-to-two-piece slice`
}

// A call (or a !-prefixed call) binds tighter than any binary
// operator, so the rewrite needs no parentheses inside a larger
// condition; the chained IndexByte comparison stands where a
// comparison already stood.
func condition(ok bool, s string) bool {
	_ = ok && len(strings.SplitN(s, "=", 2)) == 1     // want `allocates the up-to-two-piece slice`
	return ok && len(strings.SplitN(s, "=>", 2)) == 1 // want `allocates the up-to-two-piece slice`
}

// A comment inside the scaffolding the fix would delete withholds the
// automatic fix: the report stays advisory and the comment survives.
func commentAdvisory(s string) {
	_ = len( /* keep me */ strings.SplitN(s, "=>", 2)) == 2 // want `a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand`
	_ = len(strings.SplitN(s, "=>", 2)) == /* two */ 2      // want `a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand`
	_ = len(strings.SplitN(s, "=>" /* tail */, 2)) == 2     // want `a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand`
}

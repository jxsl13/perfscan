package ps2038neg

import "unicode/utf8"

// Named operand types are out of scope: the conversion also changes the
// static type, and the semantics belong to the named type.
type Body string

type Raw []byte

func namedOperands(t Body, r Raw) {
	_, _ = utf8.DecodeRune([]byte(t))
	_, _ = utf8.DecodeRuneInString(string(r))
	_, _ = utf8.DecodeLastRuneInString(string(r))
}

// A conversion to a DEFINED byte-slice type is not the throwaway
// predeclared conversion this check targets.
type Blob []byte

func definedConvTarget(s string) {
	_, _ = utf8.DecodeRune(Blob(s))
}

// []byte(nil) is a valid conversion, but utf8.DecodeRuneInString(nil)
// would not compile — untyped nil is out of scope.
func nilOperand() {
	_, _ = utf8.DecodeRune([]byte(nil))
}

// The already-direct spellings are the check's AFTER shapes.
func alreadyDirect(s string, b []byte) {
	_, _ = utf8.DecodeRune(b)
	_, _ = utf8.DecodeRuneInString(s)
	_, _ = utf8.DecodeLastRune(b)
	_, _ = utf8.DecodeLastRuneInString(s)
}

// string -> string and []byte -> []byte "conversions" are no-op copies
// of an operand that already has the right type — a different pattern,
// not a cross-kind decode round-trip.
func sameKindConversions(s string, b []byte) {
	_, _ = utf8.DecodeRuneInString(string(s))
	_, _ = utf8.DecodeRune([]byte(b))
}

// string([]rune) UTF-8-ENCODES its runes — a different operation, not a
// byte-verbatim copy — and string(r) encodes one rune. Neither matches.
func runeSources(rs []rune, is []int32, r rune) {
	_, _ = utf8.DecodeRuneInString(string(rs))
	_, _ = utf8.DecodeLastRuneInString(string(is))
	_, _ = utf8.DecodeRuneInString(string(r))
}

// A conversion result stored in a variable first may have other
// consumers and is out of scope.
func stored(b []byte) (rune, int) {
	s := string(b)
	return utf8.DecodeRuneInString(s)
}

// A same-named local function value is not unicode/utf8's (and a bare,
// non-selector callee is out of scope regardless).
func localFunc(b []byte) (rune, int) {
	DecodeRuneInString := func(s string) (rune, int) { return 0, len(s) }
	return DecodeRuneInString(string(b))
}

// A shadowed utf8 identifier with same-named methods never matches.
type fake struct{}

func (fake) DecodeRuneInString(s string) (rune, int) { return 0, len(s) }

func shadowed(b []byte) (rune, int) {
	utf8 := fake{}
	return utf8.DecodeRuneInString(string(b))
}

// Other unicode/utf8 members are not this check's pattern.
func otherMembers(b []byte) {
	_ = utf8.RuneLen('x')
	_ = utf8.FullRune(b)
}

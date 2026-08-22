package ps2025neg

import "unicode/utf8"

// Named operand types are out of scope: the conversion also changes the
// static type, and the semantics belong to the named type.
type Body string

type Raw []byte

func namedOperands(t Body, r Raw) {
	_ = utf8.Valid([]byte(t))
	_ = utf8.ValidString(string(r))
}

// A conversion to a DEFINED byte-slice type is not the throwaway
// predeclared conversion this check targets.
type Blob []byte

func definedConvTarget(s string) {
	_ = utf8.Valid(Blob(s))
}

// []byte(nil) is a valid conversion, but utf8.ValidString(nil) would not
// compile — untyped nil is out of scope.
func nilOperand() {
	_ = utf8.Valid([]byte(nil))
}

// The already-direct spellings are the check's AFTER shapes.
func alreadyDirect(s string, b []byte) {
	_ = utf8.ValidString(s)
	_ = utf8.Valid(b)
}

// string -> string and []byte -> []byte "conversions" are no-op copies of
// an operand that already has the right type — a different pattern, not a
// cross-kind validation round-trip.
func sameKindConversions(s string, b []byte) {
	_ = utf8.ValidString(string(s))
	_ = utf8.Valid([]byte(b))
}

// A type parameter is not the plain predeclared operand type, even when
// its type set is only strings.
func generic[T ~string](v T) bool {
	return utf8.Valid([]byte(v))
}

// A shadowed utf8 identifier never matches — the callee is pinned by type
// information to the standard library's package-level functions.
type fakeUTF8 struct{}

func (fakeUTF8) Valid(b []byte) bool       { return len(b) >= 0 }
func (fakeUTF8) ValidString(s string) bool { return len(s) >= 0 }

func shadowed(s string, b []byte) {
	utf8 := fakeUTF8{}
	_ = utf8.Valid([]byte(s))
	_ = utf8.ValidString(string(b))
}

// Other members of the package are different patterns.
func otherMembers(b []byte, r rune) {
	_ = utf8.ValidRune(r)
	_ = utf8.RuneCount(b)
}

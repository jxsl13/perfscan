package ps2045adv

import "bytes"

// shared is read from noimport.go, whose file has no bytes import.
var shared, shared2 bytes.Buffer

// A *bytes.Buffer receiver that is not provably non-nil — on EITHER
// side — keeps the report advisory: (*bytes.Buffer)(nil).String() is
// "<nil>" — two nil receivers even compare EQUAL — while Bytes()
// panics, so the rewrite could change behavior.
func pointerAdvisory(p, q *bytes.Buffer) {
	var a bytes.Buffer
	_ = p.String() == q.String()                                    // want `a \*bytes\.Buffer receiver is not provably non-nil .* the automatic fix is withheld`
	_ = a.String() != p.String()                                    // want `a \*bytes\.Buffer receiver is not provably non-nil .* the automatic fix is withheld`
	_ = p.String() == a.String()                                    // want `a \*bytes\.Buffer receiver is not provably non-nil .* the automatic fix is withheld`
	_ = bytes.NewBuffer(nil).String() == new(bytes.Buffer).String() // want `a \*bytes\.Buffer receiver is not provably non-nil .* the automatic fix is withheld`
}

// A comparison yields an UNTYPED bool; bytes.Equal returns the typed
// bool, which does not compile where the result flows into a named
// bool type — those sites stay advisory.
type flag bool

func take(flag) {}

func namedBoolAdvisory(a, b bytes.Buffer) flag {
	take(a.String() == b.String())        // want `flows into a named bool type,.* the automatic fix is withheld`
	var f flag = a.String() != b.String() // want `flows into a named bool type,.* the automatic fix is withheld`
	_ = f
	return a.String() == b.String() // want `flows into a named bool type,.* the automatic fix is withheld`
}

// The identifier bytes is shadowed at the comparison, so the file has
// no usable way to name the package there — advisory.
func shadowed(a, b bytes.Buffer) {
	bytes := "shadow"
	_ = bytes
	_ = a.String() == b.String() // want `no usable import of package bytes at this position .* the automatic fix is withheld`
}

// A comment inside the syntax the fix would replace withholds the
// automatic fix: the report stays advisory and the comment survives.
func commentAdvisory() {
	var a, b bytes.Buffer
	_ = a.String() == /* why */ b.String() // want `a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand`
}

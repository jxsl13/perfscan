package ps2031adv

import "bytes"

// shared is read from noimport.go, whose file has no bytes import.
var shared bytes.Buffer

// A *bytes.Buffer receiver that is not provably non-nil keeps the
// report advisory: (*bytes.Buffer)(nil).String() is "<nil>" — a
// comparison against "<nil>" is even TRUE on the nil receiver — while
// Bytes() panics, so the rewrite could change behavior.
func pointerAdvisory(p *bytes.Buffer) {
	_ = p.String() == "OK"                    // want `the \*bytes\.Buffer receiver is not provably non-nil .* the automatic fix is withheld`
	_ = p.String() == "<nil>"                 // want `the \*bytes\.Buffer receiver is not provably non-nil .* the automatic fix is withheld`
	_ = bytes.NewBuffer(nil).String() != "OK" // want `the \*bytes\.Buffer receiver is not provably non-nil .* the automatic fix is withheld`
}

// A comparison yields an UNTYPED bool; bytes.Equal returns the typed
// bool, which does not compile where the result flows into a named
// bool type — those sites stay advisory.
type flag bool

func take(flag) {}

func namedBoolAdvisory(buf bytes.Buffer) flag {
	take(buf.String() == "OK")  // want `flows into a named bool type,.* the automatic fix is withheld`
	var f flag = buf.String() != "OK" // want `flows into a named bool type,.* the automatic fix is withheld`
	_ = f
	return buf.String() == "OK" // want `flows into a named bool type,.* the automatic fix is withheld`
}

// The identifier bytes is shadowed at the comparison, so the file has
// no usable way to name the package there — advisory.
func shadowed(buf bytes.Buffer) {
	bytes := "shadow"
	_ = bytes
	_ = buf.String() == "OK" // want `no usable import of package bytes at this position .* the automatic fix is withheld`
}

// A comment inside the syntax the fix would replace withholds the
// automatic fix: the report stays advisory and the comment survives.
func commentAdvisory() {
	var buf bytes.Buffer
	_ = buf.String() == /* why */ "OK" // want `a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand`
}

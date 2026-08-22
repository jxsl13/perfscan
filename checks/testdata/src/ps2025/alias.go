package ps2025

import enc "unicode/utf8"

// An aliased import keeps its qualifier verbatim — only the member
// identifier is renamed.
func aliased(s string, b []byte) {
	_ = enc.Valid([]byte(s))       // want `utf8\.ValidString\(s\) runs the identical validation in place`
	_ = enc.ValidString(string(b)) // want `utf8\.Valid\(b\) runs the identical validation in place`
}

package ps5069adv

import (
	"bytes"
	"strings"
)

// A *bytes.Buffer receiver that is not provably non-nil keeps the report
// advisory: (*bytes.Buffer)(nil).String() is "<nil>" while Bytes() panics.
func pointerAdvisory(p *bytes.Buffer) bool {
	return strings.HasPrefix(p.String(), "GET ") // want `the \*bytes\.Buffer receiver is not provably non-nil .* the automatic fix is withheld`
}

// The identifier bytes is shadowed at the call, so the file has no usable
// way to name the package there — advisory.
func shadowed(buf bytes.Buffer) bool {
	bytes := "shadow"
	_ = bytes
	return strings.HasPrefix(buf.String(), "GET ") // want `no usable import of package bytes at this position .* the automatic fix is withheld`
}

// A comment inside the syntax the fix would replace withholds the fix.
func commented(buf bytes.Buffer) bool {
	return strings.Contains(buf.String() /* inline */, "sep") // want `a comment inside the rewritten call withholds the automatic fix`
}

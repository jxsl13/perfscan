package ps2031

import bb "bytes"

// An aliased import of package bytes is reused under its alias — the
// fix must not hardcode the canonical name.
func aliasedImport(buf bb.Buffer) {
	if buf.String() == "hit" { // want `buf\.String\(\) == "hit" copies the whole bytes\.Buffer just to compare it against a constant string; bb\.Equal\(buf\.Bytes\(\), \[\]byte\("hit"\)\) tests`
		return
	}
}

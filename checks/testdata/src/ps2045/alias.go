package ps2045

import bb "bytes"

// An aliased import of package bytes is reused under its alias — the
// fix must not hardcode the canonical name.
func aliasedImport(x, y bb.Buffer) {
	if x.String() == y.String() { // want `x\.String\(\) == y\.String\(\) copies both whole bytes\.Buffers just to compare them; bb\.Equal\(x\.Bytes\(\), y\.Bytes\(\)\) tests`
		return
	}
}

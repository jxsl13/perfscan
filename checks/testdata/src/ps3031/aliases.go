package ps3031

import (
	by "bytes"
	u "unicode"
)

// Aliased imports: the bytes qualifier is kept verbatim (by stays
// by), and the aliased unicode spec is dropped when its only reference
// is the deleted predicate argument.
func aliased(b []byte) []byte {
	return by.TrimFunc(b, u.IsSpace) // want `bytes\.TrimFunc\(b, unicode\.IsSpace\) decodes`
}

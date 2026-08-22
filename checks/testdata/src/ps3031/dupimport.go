package ps3031

import (
	by "bytes"
	u1 "unicode"
	u2 "unicode"
)

// The unicode path is imported under two specs and the fixes would
// orphan BOTH (dropping just one would still leave a compile error) —
// vanishingly rare, so every report in this file stays advisory.
func dupImport(b []byte) []byte {
	b = by.TrimFunc(b, u1.IsSpace)    // want `bytes\.TrimFunc\(b, unicode\.IsSpace\) decodes`
	return by.TrimFunc(b, u2.IsSpace) // want `bytes\.TrimFunc\(b, unicode\.IsSpace\) decodes`
}

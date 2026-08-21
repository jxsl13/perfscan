package ps5035

import (
	st "strings"
	u1 "unicode"
	u2 "unicode"
)

// The unicode path is imported under two specs and the fixes would
// orphan BOTH (dropping just one would still leave a compile error) —
// vanishingly rare, so every report in this file stays advisory.
func dupImport(s string) string {
	s = st.TrimFunc(s, u1.IsSpace) // want `strings\.TrimFunc\(s, unicode\.IsSpace\) decodes`
	return st.TrimFunc(s, u2.IsSpace) // want `strings\.TrimFunc\(s, unicode\.IsSpace\) decodes`
}

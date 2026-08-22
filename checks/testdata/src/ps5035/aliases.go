package ps5035

import (
	st "strings"
	u "unicode"
)

// Aliased imports: the strings qualifier is kept verbatim (st stays
// st), and the aliased unicode spec is dropped when its only reference
// is the deleted predicate argument.
func aliased(s string) string {
	return st.TrimFunc(s, u.IsSpace) // want `strings\.TrimFunc\(s, unicode\.IsSpace\) decodes`
}

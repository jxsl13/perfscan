package ps5018

import (
	st "strings"
	u "unicode"
)

// Aliased imports: the strings qualifier is kept verbatim (st stays
// st), and the aliased unicode spec is dropped when its only reference
// is the deleted mapping argument.
func aliased(s string) string {
	return st.Map(u.ToLower, s) // want `strings\.Map\(unicode\.ToLower, s\) pays`
}

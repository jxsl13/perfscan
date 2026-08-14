package ps3004

import (
	"bytes"
	str "strings"
)

// strings is imported under an alias: the fix emits the bare strings
// qualifier, which would not resolve here — advisory only.
func aliasedStrings(s, sub string) bool {
	if str.ToUpper(s) == sub {
		return true
	}
	return bytes.Contains([]byte(s), []byte(sub)) // want `bytes\.Contains\(\[\]byte\(s\), \[\]byte\(sub\)\) allocates two throwaway \[\]byte copies just to scan them; strings\.Contains\(s, sub\) runs the same scan on the string bytes directly with zero allocations`
}

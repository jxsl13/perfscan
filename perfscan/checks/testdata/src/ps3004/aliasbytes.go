package ps3004

import (
	bs "bytes"
	"strings"
)

// An aliased bytes import still matches — the callee is resolved by type
// information — and the rewrite removes the alias's only reference, so
// the whole spec (alias included) is dropped; strings is already imported
// and stays used.
func aliasedBytes(s, sub string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	return bs.EqualFold([]byte(s), []byte(sub)) // want `bytes\.EqualFold\(\[\]byte\(s\), \[\]byte\(sub\)\) allocates two throwaway \[\]byte copies just to scan them; strings\.EqualFold\(s, sub\) runs the same scan on the string bytes directly with zero allocations`
}

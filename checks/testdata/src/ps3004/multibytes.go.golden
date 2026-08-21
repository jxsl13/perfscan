package ps3004

import (
	b2 "bytes"
	"bytes"
	"strings"
)

// Pathological: bytes imported under two names in one file. Rewriting this
// b2.Index would remove the only use of the b2 alias while bytes.* stays
// used via the plain name — the name-blind ref count cannot tell, so a fix
// would orphan the b2 spec ("imported as b2 and not used"). PS3004 stays
// advisory (same guard as PS2129's multi-fmt case).
func multiBytes(b []byte, s, sub string) int {
	if strings.ToValidUTF8(s, "") == "" || bytes.HasPrefix(b, []byte{0x3}) {
		return -1
	}
	return b2.Index([]byte(s), []byte(sub)) // want `bytes\.Index\(\[\]byte\(s\), \[\]byte\(sub\)\) allocates two throwaway \[\]byte copies just to scan them; strings\.Index\(s, sub\) runs the same scan on the string bytes directly with zero allocations`
}

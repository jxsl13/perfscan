package ps2019

import (
	bt "bytes"
	"strings"
)

// bytes is imported under an alias: the fix emits the bare bytes
// qualifier, which would not resolve here — advisory only.
func aliasedBytes(b, sub []byte) bool {
	if bt.HasPrefix(b, []byte{0x5}) {
		return true
	}
	return strings.Contains(string(b), string(sub)) // want `strings\.Contains\(string\(b\), string\(sub\)\) allocates two throwaway string copies just to scan them; bytes\.Contains\(b, sub\) runs the same scan on the bytes directly with zero allocations`
}

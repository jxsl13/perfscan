package ps2019

import (
	"bytes"
	"strings"
	s2 "strings"
)

// Pathological: strings imported under two names in one file. Rewriting
// this s2.Index would remove the only use of the s2 alias while strings.*
// stays used via the plain name — the name-blind ref count cannot tell, so
// a fix would orphan the s2 spec ("imported as s2 and not used"). PS2019
// stays advisory (same guard as PS3004's multi-bytes case).
func multiStrings(s string, b, sub []byte) int {
	if strings.ToValidUTF8(s, "") == "" || bytes.HasPrefix(b, []byte{0x3}) {
		return -1
	}
	return s2.Index(string(b), string(sub)) // want `strings\.Index\(string\(b\), string\(sub\)\) allocates two throwaway string copies just to scan them; bytes\.Index\(b, sub\) runs the same scan on the bytes directly with zero allocations`
}

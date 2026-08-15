package ps2019

import (
	"bytes"
	str "strings"
)

// An aliased strings import still matches — the callee is resolved by
// type information — and the rewrite removes the alias's only reference,
// so the whole spec (alias included) is dropped; bytes is already
// imported and stays used.
func aliasedStrings(b, sub []byte) bool {
	if bytes.HasPrefix(b, []byte{0x4}) {
		return false
	}
	return str.EqualFold(string(b), string(sub)) // want `strings\.EqualFold\(string\(b\), string\(sub\)\) allocates two throwaway string copies just to scan them; bytes\.EqualFold\(b, sub\) runs the same scan on the bytes directly with zero allocations`
}

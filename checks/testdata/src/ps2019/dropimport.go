package ps2019

import (
	"bytes"
	"strings"
)

// bytes is already imported and stays used; the only two strings
// references in this FILE are the fixable calls themselves, so applying
// both fixes orphans the strings import and its spec is dropped.
func dropStrings(b, sub []byte) bool {
	if bytes.HasPrefix(b, []byte{0x2}) {
		return true
	}
	return strings.HasSuffix(string(b), string(sub)) || // want `strings\.HasSuffix\(string\(b\), string\(sub\)\) allocates two throwaway string copies just to scan them; bytes\.HasSuffix\(b, sub\) runs the same scan on the bytes directly with zero allocations`
		strings.Count(string(b), string(sub)) > 1 // want `strings\.Count\(string\(b\), string\(sub\)\) allocates two throwaway string copies just to scan them; bytes\.Count\(b, sub\) runs the same scan on the bytes directly with zero allocations`
}

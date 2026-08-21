package ps2019

import (
	"strings"
)

// The only strings reference in this FILE is the fixable call itself and
// bytes is not imported: applying the fix orphans the strings import, so
// its spec is swapped for "bytes" in place.
func orphanSwap(b []byte, chars string) int {
	return strings.IndexAny(string(b), chars) // want `strings\.IndexAny\(string\(b\), chars\) allocates a throwaway string copy just to scan it; bytes\.IndexAny\(b, chars\) runs the same scan on the bytes directly with zero allocations`
}

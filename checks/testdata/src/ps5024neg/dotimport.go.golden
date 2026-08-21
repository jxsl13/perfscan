package ps5024neg

import . "strings"

// A dot-imported ContainsRune call has no selector to rewrite; the shape
// is out of scope and never reported.
func dotted(s string) bool {
	return ContainsRune(s, 'z')
}

package ps5023neg

import . "strings"

// A dot-imported IndexRune call has no selector to rewrite; the shape is
// out of scope and never reported.
func dotted(s string) int {
	return IndexRune(s, 'z')
}

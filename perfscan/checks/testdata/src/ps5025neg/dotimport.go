package ps5025neg

import . "strings"

// A dot-imported LastIndexAny call has no selector to rewrite; the shape
// is out of scope and never reported.
func dotted(s string) int {
	return LastIndexAny(s, "z")
}

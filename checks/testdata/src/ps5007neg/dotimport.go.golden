package ps5007neg

import . "strings"

// A dot-imported Index call has no selector to rewrite; the shape is out
// of scope and never reported.
func dotted(s string) int {
	return Index(s, "z") + LastIndex(s, "z")
}

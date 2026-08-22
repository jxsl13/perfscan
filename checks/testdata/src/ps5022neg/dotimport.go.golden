package ps5022neg

import . "strings"

// A dot-imported IndexAny/ContainsAny call has no selector to rewrite;
// the shape is out of scope and never reported.
func dotted(s string) bool {
	return IndexAny(s, "z") >= 0 || ContainsAny(s, "z")
}

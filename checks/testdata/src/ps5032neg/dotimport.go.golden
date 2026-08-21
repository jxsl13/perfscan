package ps5032neg

import . "bytes"

// A dot-imported IndexAny/ContainsAny call has no selector to rewrite;
// the shape is out of scope and never reported.
func dotted(b []byte) bool {
	return IndexAny(b, "—") >= 0 || ContainsAny(b, "—")
}

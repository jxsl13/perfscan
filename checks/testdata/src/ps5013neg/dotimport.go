package ps5013neg

import . "bytes"

// A dot-imported Index call has no selector to rewrite; the shape is out
// of scope and never reported.
func dotted(b []byte) int {
	return Index(b, []byte{'z'}) + LastIndex(b, []byte("z"))
}

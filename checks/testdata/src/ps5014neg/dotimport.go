package ps5014neg

import . "bytes"

// A dot-imported Contains call has no selector to rewrite; the shape is
// out of scope and never reported.
func dotted(b []byte) bool {
	return Contains(b, []byte{'z'})
}

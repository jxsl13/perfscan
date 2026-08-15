package ps5026neg

import . "bytes"

// A dot-imported ContainsRune call has no selector to rewrite; the shape
// is out of scope and never reported.
func dotted(b []byte) bool {
	return ContainsRune(b, 'z')
}

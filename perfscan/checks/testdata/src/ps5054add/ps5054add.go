package ps5054add

import "encoding/hex"

// A non-compare hex use keeps hex alive after the compare is rewritten, so the
// fix fires and must ADD the bytes import.
var _ = hex.EncodedLen

func addBytes(a, b []byte) bool {
	return hex.EncodeToString(a) == hex.EncodeToString(b) // want `hex\.EncodeToString\(a\) == hex\.EncodeToString\(b\) hex-encodes two slices just to compare them; bytes\.Equal\(a, b\) compares the bytes directly`
}

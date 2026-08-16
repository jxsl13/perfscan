package ps5054orphan

import "encoding/hex"

// These hex.EncodeToString calls are the file's only hex references: fixing
// would orphan the import, so the report stays advisory.
func onlyRef(a, b []byte) bool {
	return hex.EncodeToString(a) == hex.EncodeToString(b) // want `hex\.EncodeToString\(a\) == hex\.EncodeToString\(b\) hex-encodes two slices just to compare them; bytes\.Equal\(a, b\) compares the bytes directly`
}

package ps5064

import (
	"bytes"
	"encoding/hex"
)

var _ = bytes.Contains
var _ = hex.DecodedLen // keeps hex alive so the rewrites never orphan it

// --- POSITIVES ---

func eq(x []byte) bool {
	return hex.EncodeToString(x) == "deadbeef" // want `hex\.EncodeToString\(x\) == a constant hex string`
}

func neq(x []byte) bool {
	return hex.EncodeToString(x) != "00ff" // want `hex\.EncodeToString\(x\) != a constant hex string`
}

// Constant on the left.
func constLeft(x []byte) bool {
	return "cafe" == hex.EncodeToString(x) // want `hex\.EncodeToString\(x\) == a constant hex string`
}

// The empty constant matches an empty slice.
func emptyConst(x []byte) bool {
	return hex.EncodeToString(x) == "" // want `hex\.EncodeToString\(x\) == a constant hex string`
}

// Decodes to printable ASCII.
func asciiConst(x []byte) bool {
	return hex.EncodeToString(x) == "4142" // want `hex\.EncodeToString\(x\) == a constant hex string`
}

// --- NEGATIVES: silent ---

// Uppercase: EncodeToString only emits lowercase, so this is unconditionally
// false while bytes.Equal of the decoded byte would be true.
func upperConst(x []byte) bool {
	return hex.EncodeToString(x) == "DEAD"
}

// Odd length is never produced by EncodeToString.
func oddConst(x []byte) bool {
	return hex.EncodeToString(x) == "abc"
}

// Non-hex characters.
func nonHexConst(x []byte) bool {
	return hex.EncodeToString(x) == "xyzt"
}

// Ordering, not equality.
func ordering(x []byte) bool {
	return hex.EncodeToString(x) < "deadbeef"
}

// Both sides are encodes (that is PS5054's).
func bothEncode(x, y []byte) bool {
	return hex.EncodeToString(x) == hex.EncodeToString(y)
}

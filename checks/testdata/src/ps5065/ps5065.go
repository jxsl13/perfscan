package ps5065

import (
	"bytes"
	"encoding/hex"
)

var _ = bytes.Contains
var _ = hex.DecodedLen // keeps hex alive so the rewrites never orphan it

// --- POSITIVES ---

func lt(a, b []byte) bool {
	return hex.EncodeToString(a) < hex.EncodeToString(b) // want `hex\.EncodeToString\(a\) < hex\.EncodeToString\(b\) hex-encodes two slices just to order them`
}

func leq(a, b []byte) bool {
	return hex.EncodeToString(a) <= hex.EncodeToString(b) // want `hex\.EncodeToString\(a\) <= hex\.EncodeToString\(b\) hex-encodes two slices just to order them`
}

func gt(a, b []byte) bool {
	return hex.EncodeToString(a[1:]) > hex.EncodeToString(b[:2]) // want `hex\.EncodeToString\(a\) > hex\.EncodeToString\(b\) hex-encodes two slices just to order them`
}

func geq(a, b []byte) bool {
	return hex.EncodeToString(a) >= hex.EncodeToString(b) // want `hex\.EncodeToString\(a\) >= hex\.EncodeToString\(b\) hex-encodes two slices just to order them`
}

// --- ADVISORY: reported, no fix ---

func commentInside(a, b []byte) bool {
	return hex.EncodeToString(a) < hex.EncodeToString( /* keep */ b) // want `hex\.EncodeToString\(a\) < hex\.EncodeToString\(b\) hex-encodes two slices just to order them`
}

// --- NEGATIVES: silent ---

// Equality is PS5054's domain.
func equality(a, b []byte) bool {
	return hex.EncodeToString(a) == hex.EncodeToString(b)
}

// Only one operand is a hex encode.
func oneEncode(a []byte) bool {
	return hex.EncodeToString(a) < "ff"
}

package ps5058

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
)

var _ = bytes.Contains
var _ = hex.DecodedLen

// keeps base64/base32 imports alive so the rewrites do not orphan them
var _ = base64.RawURLEncoding
var _ = base32.HexEncoding

// --- POSITIVES ---

func eqStd(a, b []byte) bool {
	return base64.StdEncoding.EncodeToString(a) == base64.StdEncoding.EncodeToString(b) // want `enc\.EncodeToString\(a\) == enc\.EncodeToString\(b\) encodes two slices just to compare them`
}

func neqURL(a, b []byte) bool {
	return base64.URLEncoding.EncodeToString(a) != base64.URLEncoding.EncodeToString(b) // want `enc\.EncodeToString\(a\) != enc\.EncodeToString\(b\) encodes two slices just to compare them`
}

func eqBase32(a, b []byte) bool {
	return base32.StdEncoding.EncodeToString(a) == base32.StdEncoding.EncodeToString(b) // want `enc\.EncodeToString\(a\) == enc\.EncodeToString\(b\) encodes two slices just to compare them`
}

// Same encoder held in a variable.
func eqVar(enc *base64.Encoding, a, b []byte) bool {
	return enc.EncodeToString(a) == enc.EncodeToString(b) // want `enc\.EncodeToString\(a\) == enc\.EncodeToString\(b\) encodes two slices just to compare them`
}

// --- ADVISORY: reported, no fix ---

func commentInside(a, b []byte) bool {
	return base64.StdEncoding.EncodeToString(a) == base64.StdEncoding.EncodeToString( /* keep */ b) // want `enc\.EncodeToString\(a\) == enc\.EncodeToString\(b\) encodes two slices just to compare them`
}

// --- NEGATIVES: silent ---

// Different encoders on the two sides (Std vs URL) are not equivalent.
func crossEncoder(a, b []byte) bool {
	return base64.StdEncoding.EncodeToString(a) == base64.URLEncoding.EncodeToString(b)
}

// Different receiver variables — not provably the same encoder.
func mismatchedVar(e1, e2 *base64.Encoding, a, b []byte) bool {
	return e1.EncodeToString(a) == e2.EncodeToString(b)
}

// Ordering does not carry over.
func ordering(a, b []byte) bool {
	return base64.StdEncoding.EncodeToString(a) < base64.StdEncoding.EncodeToString(b)
}

// Package-level hex is PS5054's domain.
func hexIsPS5054(a, b []byte) bool {
	return hex.EncodeToString(a) == hex.EncodeToString(b)
}

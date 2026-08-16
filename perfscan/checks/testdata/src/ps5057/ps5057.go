package ps5057

import (
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
)

// --- POSITIVES ---

func stdBase64(b []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(b)) // want `\[\]byte\(enc\.EncodeToString\(b\)\) allocates a throwaway encoded string and copies it into a \[\]byte`
}

func urlBase64(b []byte) []byte {
	return []byte(base64.URLEncoding.EncodeToString(b[1:])) // want `\[\]byte\(enc\.EncodeToString\(b\)\) allocates a throwaway encoded string and copies it into a \[\]byte`
}

func stdBase32(b []byte) []byte {
	return []byte(base32.StdEncoding.EncodeToString(b)) // want `\[\]byte\(enc\.EncodeToString\(b\)\) allocates a throwaway encoded string and copies it into a \[\]byte`
}

// --- ADVISORY: reported, no fix ---

func commentInside(b []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(b) /* keep */) // want `\[\]byte\(enc\.EncodeToString\(b\)\) allocates a throwaway encoded string and copies it into a \[\]byte`
}

// --- NEGATIVES: silent ---

// A string result, not []byte(...).
func stringResult(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// Package-level hex is PS5056's domain, not base64/base32.
func hexIsPS5056(b []byte) []byte {
	return []byte(hex.EncodeToString(b))
}

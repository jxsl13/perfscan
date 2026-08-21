package ps5058add

import "encoding/base64"

var _ = base64.URLEncoding

func addBytes(a, b []byte) bool {
	return base64.StdEncoding.EncodeToString(a) == base64.StdEncoding.EncodeToString(b) // want `enc\.EncodeToString\(a\) == enc\.EncodeToString\(b\) encodes two slices just to compare them`
}

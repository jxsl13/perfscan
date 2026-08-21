package ps5058orphan

import "encoding/base64"

// base64.StdEncoding here is the file's only base64 reference: fixing would
// orphan the import, so the report stays advisory.
func onlyRef(a, b []byte) bool {
	return base64.StdEncoding.EncodeToString(a) == base64.StdEncoding.EncodeToString(b) // want `enc\.EncodeToString\(a\) == enc\.EncodeToString\(b\) encodes two slices just to compare them`
}

package ps5039dot

import . "unicode/utf8"

// A dot import leaves no usable qualifier for the fix to spell
// utf8.AppendRune with, so the report stays advisory (the fix pipeline
// never rewrites a dot import into a qualified one).
func dotAdvisory(dst []byte, r rune) []byte {
	return append(dst, string(r)...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst`
}

var _ = UTFMax

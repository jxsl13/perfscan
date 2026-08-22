package ps2038neg

import . "unicode/utf8"

// Under a dot import the callee is a bare identifier, not a selector —
// deliberately out of scope (the fix rewrites a selector's member).
func dotImported(s string, b []byte) {
	_, _ = DecodeRune([]byte(s))
	_, _ = DecodeRuneInString(string(b))
	_, _ = DecodeLastRuneInString(string(b))
}

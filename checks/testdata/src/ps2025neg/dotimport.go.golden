package ps2025neg

import . "unicode/utf8"

// Under a dot import the callee is a bare identifier, not a selector —
// deliberately out of scope (the fix rewrites a selector's member).
func dotImported(s string, b []byte) {
	_ = Valid([]byte(s))
	_ = ValidString(string(b))
}

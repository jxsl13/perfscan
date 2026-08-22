package ps5037dot

import . "unicode/utf8"

// Under a dot import the callee is a bare ident. This file keeps
// another unicode/utf8 reference (RuneError below), so replacing the
// callee cannot orphan the dot import and the fix applies.
func dotFix(s string) bool {
	return RuneCountInString(s) == 0 // want `utf8\.RuneCountInString\(\.\.\.\) == 0 scans the entire string just to test emptiness; len\(s\) == 0 is the bit-identical O\(1\) test`
}

var _ = RuneError

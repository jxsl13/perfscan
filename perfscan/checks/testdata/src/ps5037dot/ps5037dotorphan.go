package ps5037dot

import . "unicode/utf8"

// The matched call is this file's ONLY unicode/utf8 reference. The fix
// pipeline never prunes a dot import, so replacing the callee would
// leave an "imported and not used" error — the fix is withheld and the
// report stays advisory.
func dotOrphan(s string) bool {
	return RuneCountInString(s) != 0 // want `utf8\.RuneCountInString\(\.\.\.\) != 0 scans the entire string just to test emptiness; len\(s\) != 0 is the bit-identical O\(1\) test`
}

package ps2005

import rx "regexp"

// Aliased regexp import: PS2005 detects regexp by its canonical name
// (astutil.PkgFuncCall is name-based), so an aliased `rx.MustCompile` in a loop
// is NOT currently flagged. This pins that boundary. It is the companion to the
// defensive qualifier rendering in hoistRegexpFix: were detection ever broadened
// to aliases, the hoist would correctly emit `psRe := rx.MustCompile(...)` (not a
// hardcoded `regexp.`), and this file's golden would then change — a conscious,
// reviewed update rather than a silent uncompilable rewrite.
func aliasedInLoop(lines []string) int {
	n := 0
	for _, s := range lines {
		if rx.MustCompile("^a+$").MatchString(s) {
			n++
		}
	}
	return n
}

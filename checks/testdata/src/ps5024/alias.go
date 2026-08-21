package ps5024

import str "strings"

// An aliased strings import keeps its qualifier verbatim; only the
// selected name and the appended comparison change — IndexByte lives in
// the same package, so no import surgery is ever needed.
func aliased(s string) bool {
	found := str.ContainsRune(s, '@')         // want `strings\.ContainsRune of the constant ASCII rune '@' chains two wrapper frames`
	return found || !str.ContainsRune(s, '#') // want `strings\.IndexByte\(s, '#'\) < 0 answers the same membership question`
}

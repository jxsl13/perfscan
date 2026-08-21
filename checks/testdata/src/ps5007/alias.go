package ps5007

import str "strings"

// An aliased strings import keeps its qualifier verbatim; only the
// selected name and the literal are rewritten — no import surgery.
func aliased(s string) int {
	return str.LastIndex(s, "@") // want `strings\.LastIndex of the single-byte string "@" runs the generic substring search`
}

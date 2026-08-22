package ps5016

import str "strings"

// An aliased strings import keeps its qualifier verbatim; only the
// selected name, the wrapped literal, and the appended comparison change
// — no import surgery.
func aliased(s string) bool {
	return str.Contains(s, "@") // want `strings\.Contains of the single-byte string "@"`
}

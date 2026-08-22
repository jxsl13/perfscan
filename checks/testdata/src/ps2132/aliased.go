package ps2132

import str "strings"

// ALIASED import regression: the hoisted var must reuse the source qualifier
// (str), never a hardcoded `strings` — that name is unbound here, so a hardcoded
// qualifier would hoist to uncompilable code. Sibling of the PS2107/2112/2102
// aliased-import class.
func aliasedEscape(s string) string {
	return str.NewReplacer("&", "&amp;", "<", "&lt;").Replace(s) // want `strings\.NewReplacer with constant pairs rebuilds its lookup structure on every call`
}

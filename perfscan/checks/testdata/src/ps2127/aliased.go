package ps2127

import rx "regexp"

// ALIASED import regression: the hoisted var must reuse the source qualifier
// (rx), never a hardcoded `regexp` — that name is unbound here, so a hardcoded
// qualifier would hoist to uncompilable code. Sibling of the PS2107/2112/2102
// aliased-import class.
func aliasedMatch(s string) bool {
	return rx.MustCompile("^a+$").MatchString(s) // want `regexp\.MustCompile of a constant pattern inside function aliasedMatch recompiles the same matcher on every call; hoist it to a package-level var compiled once at init`
}

package ps2131

import rx "regexp"

// The stdlib regexp imported under an alias still resolves to the same
// package: the check must fire through the alias exactly as it does
// through the canonical qualifier.
func aliasedMatchLit(s string) bool {
	ok, _ := rx.MatchString("^[0-9]+$", s) // want `regexp\.MatchString with a constant pattern recompiles the regexp on every call`
	return ok
}

func aliasedMatchBytes(b []byte) bool {
	ok, _ := rx.Match("x+", b) // want `regexp\.Match with a constant pattern recompiles the regexp on every call`
	return ok
}

// NEGATIVE: a runtime pattern through the alias is still dynamic: silent.
func aliasedMatchVar(pat, s string) bool {
	ok, _ := rx.MatchString(pat, s)
	return ok
}

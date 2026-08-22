package ps2131

import "regexp"

const patConst = "^[a-z]+$"

// Constant pattern (literal) -> flagged.
func matchLit(s string) bool {
	ok, _ := regexp.MatchString("^[0-9]+$", s) // want `regexp\.MatchString with a constant pattern recompiles the regexp on every call`
	return ok
}

// Constant pattern (const ident) -> flagged.
func matchConst(s string) bool {
	ok, _ := regexp.MatchString(patConst, s) // want `regexp\.MatchString with a constant pattern recompiles the regexp on every call`
	return ok
}

// regexp.Match over []byte -> flagged.
func matchBytes(b []byte) bool {
	ok, _ := regexp.Match("x+", b) // want `regexp\.Match with a constant pattern recompiles the regexp on every call`
	return ok
}

// NEGATIVE: a runtime-built (variable) pattern is genuinely dynamic: silent.
func matchVar(pat, s string) bool {
	ok, _ := regexp.MatchString(pat, s)
	return ok
}

// NEGATIVE: a method call on a precompiled *Regexp is already the good form.
var pre = regexp.MustCompile("^a$")

func matchPre(s string) bool {
	return pre.MatchString(s)
}

// NEGATIVE: a shadowed regexp does not resolve to the stdlib function.
type fakeRegexp struct{}

func (fakeRegexp) MatchString(p, s string) (bool, error) { return false, nil }

func shadowed(s string) bool {
	regexp := fakeRegexp{}
	ok, _ := regexp.MatchString("^x$", s)
	return ok
}

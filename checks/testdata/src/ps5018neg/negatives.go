package ps5018neg

import (
	"bytes"
	"strings"
	"unicode"
)

// None of these is strings.Map with the stdlib unicode.ToUpper/ToLower
// mapping: no diagnostics at all.
func negatives(b []byte, s string) {
	// unicode.ToTitle is deliberately not matched: strings.ToTitle IS
	// Map(unicode.ToTitle, s) with no fast path — the rewrite would be
	// readability-only, not a perf win.
	_ = strings.Map(unicode.ToTitle, s)

	// A unicode.SpecialCase method value uses DIFFERENT case tables
	// (Turkish dotless i); rewriting to strings.ToUpper would change
	// the output. Rejected by the receiver check.
	_ = strings.Map(unicode.TurkishCase.ToUpper, s)

	// A wrapper func is not the stdlib function object, even when it
	// only delegates: the mapping argument must be type-pinned.
	wrapper := func(r rune) rune { return unicode.ToUpper(r) }
	_ = strings.Map(wrapper, s)

	// A func VARIABLE holding unicode.ToUpper is a *types.Var, not the
	// package-level *types.Func — not matched (its value could change).
	f := unicode.ToUpper
	_ = strings.Map(f, s)

	// bytes.Map is a different package's Map — that is PS5017's site,
	// out of scope here.
	_ = bytes.Map(unicode.ToUpper, b)

	// Already the right spelling.
	_ = strings.ToUpper(s)
}

// A same-named method on a value that shadows either package identifier
// never matches: both callee and mapping are pinned by type information.
type fakeMapper struct{}

func (fakeMapper) Map(func(rune) rune, string) string { return "" }

type fakeCase struct{}

func (fakeCase) ToUpper(r rune) rune { return r }

func shadowedPkgs(s string) {
	strings := fakeMapper{}
	_ = strings.Map(func(r rune) rune { return r }, s)

	unicode := struct{ ToUpper func(rune) rune }{ToUpper: func(r rune) rune { return r }}
	_ = unicode.ToUpper('x')
}

func shadowedUnicodeArg(s string) string {
	unicode := fakeCase{}
	return strings.Map(unicode.ToUpper, s)
}

package ps5017neg

import (
	"bytes"
	"strings"
	"unicode"
)

// None of these is bytes.Map with the stdlib unicode.ToUpper/ToLower
// mapping: no diagnostics at all.
func negatives(b []byte, s string) {
	// unicode.ToTitle is deliberately not matched: bytes.ToTitle IS
	// Map(unicode.ToTitle, s) with no fast path — the rewrite would be
	// readability-only, not a perf win.
	_ = bytes.Map(unicode.ToTitle, b)

	// A unicode.SpecialCase method value uses DIFFERENT case tables
	// (Turkish dotless i); rewriting to bytes.ToUpper would change the
	// output. Rejected by the receiver check.
	_ = bytes.Map(unicode.TurkishCase.ToUpper, b)

	// A wrapper func is not the stdlib function object, even when it
	// only delegates: the mapping argument must be type-pinned.
	wrapper := func(r rune) rune { return unicode.ToUpper(r) }
	_ = bytes.Map(wrapper, b)

	// A func VARIABLE holding unicode.ToUpper is a *types.Var, not the
	// package-level *types.Func — not matched (its value could change).
	f := unicode.ToUpper
	_ = bytes.Map(f, b)

	// strings.Map is a different package's Map — out of scope here.
	_ = strings.Map(unicode.ToUpper, s)

	// Already the right spelling.
	_ = bytes.ToUpper(b)
}

// A same-named method on a value that shadows either package identifier
// never matches: both callee and mapping are pinned by type information.
type fakeMapper struct{}

func (fakeMapper) Map(func(rune) rune, []byte) []byte { return nil }

type fakeCase struct{}

func (fakeCase) ToUpper(r rune) rune { return r }

func shadowedPkgs(b []byte) {
	bytes := fakeMapper{}
	_ = bytes.Map(func(r rune) rune { return r }, b)

	unicode := struct{ ToUpper func(rune) rune }{ToUpper: func(r rune) rune { return r }}
	_ = unicode.ToUpper('x')
}

func shadowedUnicodeArg(b []byte) []byte {
	unicode := fakeCase{}
	return bytes.Map(unicode.ToUpper, b)
}

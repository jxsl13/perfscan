package ps2132

import "strings"

const amp = "&amp;"

// Inline NewReplacer with constant pairs, chained to Replace: flagged.
func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s) // want `strings\.NewReplacer with constant pairs rebuilds its lookup structure on every call`
}

// Const-ident args also count as constant: flagged.
func escapeConst(s string) string {
	return strings.NewReplacer("&", amp).Replace(s) // want `strings\.NewReplacer with constant pairs rebuilds its lookup structure on every call`
}

// Chained to WriteString: flagged (message names the method).
func writeEscaped(w interface{ WriteString(string) (int, error) }, s string) {
	_, _ = strings.NewReplacer("a", "b").WriteString(nil, s) // want `strings\.NewReplacer with constant pairs rebuilds its lookup structure on every call`
	_ = w
}

// NEGATIVE: package-level replacer reused — the good form, out of scope.
var repl = strings.NewReplacer("x", "y")

func good(s string) string { return repl.Replace(s) }

// NEGATIVE: runtime (variable) pairs — genuinely per-call: silent.
func dynamic(from, to, s string) string {
	return strings.NewReplacer(from, to).Replace(s)
}

// NEGATIVE: a shadowed strings does not resolve to the stdlib function.
type fakeStrings struct{}

func (fakeStrings) NewReplacer(_ ...string) fakeReplacer { return fakeReplacer{} }

type fakeReplacer struct{}

func (fakeReplacer) Replace(s string) string { return s }

func shadowed(s string) string {
	strings := fakeStrings{}
	return strings.NewReplacer("a", "b").Replace(s)
}

package ps2013neg

import (
	"strings"
)

var pair = []string{"a", "b"}

// None of these is the single-constant-pair Replace shape: no diagnostics
// at all.
func negatives(s, dyn string) {
	// old == "" is the ONE divergent input in the whole API: NewReplacer
	// inserts between every BYTE, ReplaceAll between every RUNE. Never
	// matched.
	_ = strings.NewReplacer("", "-").Replace(s)

	// A runtime-value pair: the rewrite's evaluation reorder would be
	// observable, and old could be "" at runtime. PS2132 already reports
	// the per-call construction.
	_ = strings.NewReplacer(dyn, "x").Replace(s)
	_ = strings.NewReplacer("x", dyn).Replace(s)

	// A multi-pair replacer has no single ReplaceAll counterpart —
	// PS2132's territory (hoist to package scope).
	_ = strings.NewReplacer("a", "b", "c", "d").Replace(s)

	// An ellipsis call: the pair count is not statically a single pair.
	_ = strings.NewReplacer(pair...).Replace(s)

	// WriteString has no single-call ReplaceAll counterpart.
	var sb strings.Builder
	_, _ = strings.NewReplacer("a", "b").WriteString(&sb, s)

	// A stored replacer is already amortized — out of scope.
	r := strings.NewReplacer("a", "b")
	_ = r.Replace(s)

	// The package-level strings.Replace is a different shape entirely.
	_ = strings.Replace(s, "a", "b", -1)
}

// A same-named METHOD never matches, even though it returns a real
// *strings.Replacer whose .Replace is the stdlib method: the constructor
// is pinned to the package-level strings.NewReplacer by type information.
type fakeStrings struct{}

func (fakeStrings) NewReplacer(oldnew ...string) *strings.Replacer {
	return strings.NewReplacer(oldnew...)
}

func fakeCtor(s string) string {
	f := fakeStrings{}
	return f.NewReplacer("a", "b").Replace(s)
}

// The same with the package identifier shadowed at the call site.
func shadowedPkg(s string) string {
	strings := fakeStrings{}
	return strings.NewReplacer("a", "b").Replace(s)
}

package ps2003

import s "strings"

// aliasedHoist pins the aliased-import behavior: PkgFuncCall matches the
// aliased strings import, and the hoisted binding must reuse the alias
// (s.ReplaceAll), not the literal "strings" qualifier.
func aliasedHoist(lines []string) {
	for range lines {
		emit(s.ReplaceAll("a-b", "-", "_")) // want `strings\.ReplaceAll in a loop allocates a fresh string per iteration; hoist the transform, build a strings\.Replacer once, or reuse a byte buffer`
	}
}

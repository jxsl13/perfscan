package ps2013neg

import . "strings"

// A dot import puts NewReplacer in scope unqualified: the constructor is
// no longer a package-selector expression, and a bare ReplaceAll rewrite
// would be too fragile — the shape is deliberately not matched.
func dotImported(s string) string {
	return NewReplacer("a", "b").Replace(s)
}

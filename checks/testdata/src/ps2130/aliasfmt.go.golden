package ps2130

import stdfmt "fmt"

// An aliased fmt import still matches — the callee is resolved by type
// information — but PS2130's import bookkeeping only handles the plain
// "fmt" spec (it never swaps a replacement in, unlike PS2129), so the
// report stays advisory: no fix.
func aliasedFmt(s string) string {
	return stdfmt.Sprintf("%v", s) // want `fmt\.Sprintf\("%v", s\) on a plain string pays fmt's format parse, interface boxing and a fresh string copy just to return the bytes s already holds; s itself is bit-identical`
}

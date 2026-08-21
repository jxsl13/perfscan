package ps5027

import stdfmt "fmt"

// An aliased fmt import still matches — the callee is resolved by type
// information — but PS5027's import bookkeeping only handles the plain
// "fmt" spec (same rule as PS2130), so the report stays advisory: no fix.
func aliasedFmt() string {
	return stdfmt.Sprintf("aliased constant") // want `fmt\.Sprintf on a verbless constant string`
}

package ps2130

import . "fmt"

// Dot-imported fmt: Sprintf is called unqualified — there is no fmt
// selector to match, so PS2130 stays silent (perfscan never reasons
// about, or edits, a dot import).
func dotted(s string) string {
	return Sprintf("%s", s)
}

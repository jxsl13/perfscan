package ps2129

import (
	. "fmt"
	"os"
)

// Dot-imported fmt: Fprintf is called unqualified — there is no fmt
// selector to match, so PS2129 stays silent (perfscan never reasons
// about, or edits, a dot import).
func dotted(s string) {
	Fprintf(os.Stdout, "%s", s)
}

package ps2048

import (
	. "fmt"
	"os"
)

// Dot-imported fmt: Fprint is called unqualified — there is no fmt
// selector to match, so PS2048 stays silent (perfscan never reasons
// about, or edits, a dot import).
func dotted(a, b string) {
	Fprint(os.Stdout, a, b)
}

package ps2049

import (
	. "fmt"
	"os"
)

// Dot-imported fmt: Fprintln is called unqualified — there is no fmt
// selector to match, so PS2049 stays silent (perfscan never reasons
// about, or edits, a dot import).
func dotted(a, b string) {
	Fprintln(os.Stdout, a, b)
}

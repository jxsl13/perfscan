package ps5038

import (
	. "fmt"
	"os"
)

// Dot-imported fmt: Fprintln is called unqualified — there is no fmt
// selector to match, so PS5038 stays silent (perfscan never reasons
// about, or edits, a dot import).
func dotted(s string) {
	Fprintln(os.Stdout, s)
}

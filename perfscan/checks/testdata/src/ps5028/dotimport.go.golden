package ps5028

import (
	. "fmt"
	"os"
)

// Dot-imported fmt: Fprintf is called unqualified — there is no fmt
// selector to match, so PS5028 stays silent (perfscan never reasons
// about, or edits, a dot import).
func dotted() {
	Fprintf(os.Stdout, "dotted\n")
}

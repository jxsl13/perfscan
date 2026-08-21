package ps2130

import (
	"fmt"
	f "fmt"
)

// Pathological: fmt imported under two names in one file. Rewriting this
// f.Sprintf would remove the only use of the f alias while fmt.* stays
// used via the plain name — the name-blind ref count cannot tell, so a
// fix could orphan the f spec ("imported as f and not used"). PS2130
// stays advisory.
func multiFmt(s string) string {
	fmt.Println("keep the plain fmt name used")
	return f.Sprintf("%s", s) // want `fmt\.Sprintf\("%s", s\) on a plain string pays fmt's format parse, interface boxing and a fresh string copy just to return the bytes s already holds; s itself is bit-identical`
}

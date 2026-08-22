package ps5015

// ALIASED-IMPORT positive: the fixable Appendf is written through an fmt
// alias (`import f2 "fmt"`), and strconv is imported under an alias too —
// the rewrite must reuse the sc qualifier instead of adding a second
// strconv import. The rewrite orphans f2 and the fix pipeline prunes it.

import (
	f2 "fmt"
	sc "strconv"
)

var _ = sc.IntSize

func aliased(buf []byte, i int) []byte {
	return f2.Appendf(buf, "%d", i) // want `fmt\.Appendf of a single %d integer value`
}

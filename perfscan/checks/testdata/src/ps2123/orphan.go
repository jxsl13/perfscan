package ps2123

// The only fmt reference in this FILE is the fixable Sprint itself: the
// concatenation rewrite orphans the import, and the fix pipeline prunes
// the now-unused fmt import afterwards, so the fix applies (non-cgo file).

import (
	"fmt"
)

func orphanConcat(a, b string) string {
	return fmt.Sprint(a, b) // want `fmt\.Sprint over only plain strings inserts no separators and boxes every operand through fmt's reflection machinery; direct \+ concatenation builds the identical string`
}

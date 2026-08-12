package ps2123

// The only fmt reference in this FILE is the fixable Sprint itself: the
// concatenation rewrite would orphan the import, so advisory only.

import (
	"fmt"
)

func orphanConcat(a, b string) string {
	return fmt.Sprint(a, b) // want `fmt\.Sprint over only plain strings inserts no separators and boxes every operand through fmt's reflection machinery; direct \+ concatenation builds the identical string`
}

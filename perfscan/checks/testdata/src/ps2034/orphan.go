package ps2034

// The only fmt reference in this FILE is the fixable Sprintf itself: the
// concatenation rewrite orphans the import, and the fix pipeline prunes
// the now-unused fmt import afterwards, so the fix applies (non-cgo file).

import (
	"fmt"
)

func orphanSplice(a, b string) string {
	return fmt.Sprintf("a=%s b=%s", a, b) // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
}

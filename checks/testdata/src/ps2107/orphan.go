package ps2107

// The only fmt reference in this FILE is the fixable Sprintf itself: the
// rewrite orphans the import, and the fix pipeline prunes the now-unused
// fmt import afterwards, so the fix applies (non-cgo file) and adds the
// strconv import it needs.

import "fmt"

func orphanDecimal(i int) string {
	return fmt.Sprintf("%d", i) // want `fmt\.Sprintf of a single %d value boxes the argument and walks fmt's formatter state machine; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

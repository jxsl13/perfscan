package ps2107

// The only fmt reference in this FILE is the fixable Sprintf itself: the
// rewrite would orphan the import (the runner never prunes imports), so
// advisory only.

import "fmt"

func orphanDecimal(i int) string {
	return fmt.Sprintf("%d", i) // want `fmt\.Sprintf of a single %d value boxes the argument and walks fmt's formatter state machine; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

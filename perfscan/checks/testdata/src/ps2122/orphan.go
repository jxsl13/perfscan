package ps2122

// The only fmt reference in this FILE is the fixable Sprintf itself: the
// concatenation rewrite would orphan the import, so advisory only.

import (
	"fmt"
)

func orphanConcat(a, b string) string {
	return fmt.Sprintf("%s%s", a, b) // want `fmt\.Sprintf with a format of only %s verbs over plain strings boxes every argument and walks fmt's formatter state machine; direct \+ concatenation builds the identical string`
}

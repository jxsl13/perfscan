package ps2044

// The only fmt reference in this FILE is the fixable Appendf itself: the
// append-chain rewrite orphans the import, and the fix pipeline prunes the
// now-unused fmt import afterwards, so the fix applies (non-cgo file).

import (
	"fmt"
)

func orphanSplice(buf []byte, k, v string) []byte {
	return fmt.Appendf(buf, "%s=%s;", k, v) // want `fmt\.Appendf splicing plain strings into literal text`
}

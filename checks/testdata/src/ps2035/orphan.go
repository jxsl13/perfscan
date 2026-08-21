package ps2035

// The only fmt reference in this FILE is the fixable Appendf itself: the
// rewrite orphans the import, and the fix pipeline prunes the now-unused
// fmt import afterwards, so the fix applies (non-cgo file) and adds the
// strconv import it needs.

import "fmt"

func orphanFloat(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%v", f) // want `fmt\.Appendf of a single %v float value`
}

package ps2036

// The only fmt reference in this FILE is the fixable Append itself: the
// rewrite orphans the import, and the fix pipeline prunes the now-unused
// fmt import afterwards, so the fix applies (non-cgo file) and adds the
// strconv import it needs.

import "fmt"

func orphanAppend(buf []byte, f float64) []byte {
	return fmt.Append(buf, f) // want `fmt\.Append with a single int/uint/bool/float operand`
}

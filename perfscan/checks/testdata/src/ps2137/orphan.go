package ps2137

// The only fmt reference in this FILE is the fixable Sprintf itself: the
// rewrite orphans the import, and the fix pipeline prunes the now-unused
// fmt import afterwards, so the fix applies (non-cgo file) and adds the
// strconv import it needs.

import "fmt"

func orphanSprintf(n int) string {
	return fmt.Sprintf("%v", n) // want `fmt\.Sprint\(i\) / fmt\.Sprintf\("%v", i\) on an integer pays fmt's reflection and boxing for a plain decimal; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

package ps2031

// The only fmt reference in this FILE is the fixable Errorf itself: the
// rewrite orphans the fmt import, the fix pipeline prunes it, and the
// errors import errors.New needs is added (non-cgo file).

import "fmt"

func orphan(msg string) error {
	return fmt.Errorf("%s", msg) // want `fmt\.Errorf with a bare %s verb`
}

package ps2126

// The only fmt reference in this FILE is the fixable Errorf itself: the
// rewrite orphans the fmt import, the fix pipeline prunes it, and the
// errors import errors.New needs is added (non-cgo file).

import "fmt"

func orphan() error {
	return fmt.Errorf("only fmt reference in this file") // want `fmt\.Errorf with a verbless constant message`
}

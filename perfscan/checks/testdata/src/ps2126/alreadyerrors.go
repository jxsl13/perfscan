package ps2126

import (
	"errors"
	"fmt"
)

// errors is ALREADY imported: no duplicate import is added, and since the
// rewrite removes the file's only fmt reference the fmt spec is deleted.
func alreadyErrors(err error) error {
	if errors.Is(err, errSentinel) {
		return err
	}
	return fmt.Errorf("permission denied") // want `fmt\.Errorf on a constant verb-free message runs the whole fmt printer just to copy the string; errors\.New returns the identical \*errors\.errorString without the printer allocation or format scan`
}

var errSentinel = errors.New("sentinel")

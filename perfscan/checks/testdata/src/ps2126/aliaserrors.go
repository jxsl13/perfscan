package ps2126

import (
	errs "errors"
	"fmt"
)

// errors is imported under an ALIAS: the fix spells the call with the
// alias and deletes the now-orphaned fmt spec.
func aliasedErrors(err error) error {
	if errs.Is(err, errSentinel) {
		return err
	}
	return fmt.Errorf("timed out") // want `fmt\.Errorf on a constant verb-free message runs the whole fmt printer just to copy the string; errors\.New returns the identical \*errors\.errorString without the printer allocation or format scan`
}

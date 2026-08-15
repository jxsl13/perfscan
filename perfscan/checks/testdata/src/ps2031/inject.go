package ps2031

// errors is NOT imported here and fmt is used twice (the Errorf to fix
// plus a Fprintln that survives), so the fix adds the errors import to
// the block and keeps the fmt import.

import (
	"fmt"
	"os"
)

func inject(msg string) error {
	fmt.Fprintln(os.Stderr, "log line")
	return fmt.Errorf("%v", msg) // want `fmt\.Errorf with a bare %v verb`
}

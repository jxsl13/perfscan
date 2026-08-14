package ps2126

// errors is NOT imported here and fmt is used twice (the Errorf to fix plus a
// Fprintln that survives), so the fix adds the errors import to the block and
// keeps the fmt import.

import (
	"fmt"
	"os"
)

func inject() error {
	fmt.Fprintln(os.Stderr, "log line")
	return fmt.Errorf("needs an errors import") // want `fmt\.Errorf with a verbless constant message`
}

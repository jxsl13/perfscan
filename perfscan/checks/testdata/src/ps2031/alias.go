package ps2031

// Both packages are imported under aliases here: fmt.Errorf is matched
// by IMPORT PATH (not the qualifier spelling), and the fix reuses the
// existing errors alias verbatim instead of adding a second import.

import (
	ers "errors"
	f "fmt"
)

var _ = ers.New

func aliased(msg string) error {
	f.Print("keep the fmt alias alive")
	return f.Errorf("%s", msg) // want `fmt\.Errorf with a bare %s verb`
}

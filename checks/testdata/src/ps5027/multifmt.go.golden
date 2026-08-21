package ps5027

import (
	"fmt"
	f "fmt"
)

// Pathological: fmt imported under two names in one file. Rewriting this
// f.Sprintf would remove the only use of the f alias while fmt.* stays
// used via the plain name — the name-blind ref count cannot tell, so a
// fix could orphan the f spec ("imported as f and not used"). PS5027
// stays advisory (same rule as PS2130).
func multiFmt() string {
	fmt.Println("keep the plain fmt name used")
	return f.Sprintf("multi-spec constant") // want `fmt\.Sprintf on a verbless constant string`
}

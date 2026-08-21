package ps3025

// The only fmt reference in this FILE is the fixable Appendf itself: fixing it
// would orphan the fmt import, and the runner never prunes imports, so the fix
// is withheld and the report stays advisory (golden identical).

import "fmt"

func orphan(buf []byte) []byte {
	return fmt.Appendf(buf, "static") // want `fmt\.Appendf on a verbless constant format`
}

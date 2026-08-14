package ps2141

// The only fmt reference in this FILE is the fixable Appendf itself: fixing it
// would orphan the fmt import, and the runner never prunes imports, so the fix
// is withheld and the report stays advisory (golden identical).

import "fmt"

func orphan(buf []byte, s string) []byte {
	return fmt.Appendf(buf, "%s", s) // want `fmt\.Appendf with a lone %s over a string/\[\]byte`
}

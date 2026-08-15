package ps2033

// The only fmt reference in this FILE is the fixable Appendf itself: fixing it
// would orphan the fmt import, and the runner never prunes imports, so the fix
// is withheld and the report stays advisory (golden identical).

import "fmt"

func orphan(buf []byte, a, b string) []byte {
	return fmt.Appendf(buf, "%s%s", a, b) // want `fmt\.Appendf whose format is only repeated %s verbs`
}

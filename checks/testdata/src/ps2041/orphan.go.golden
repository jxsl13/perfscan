package ps2041

// The only fmt reference in this FILE is the fixable Appendln itself: fixing
// it would orphan the fmt import, and the runner never prunes imports, so the
// fix is withheld and the report stays advisory (golden identical).

import "fmt"

func orphan(buf []byte, a, b string) []byte {
	return fmt.Appendln(buf, a, b) // want `fmt\.Appendln over string operands`
}

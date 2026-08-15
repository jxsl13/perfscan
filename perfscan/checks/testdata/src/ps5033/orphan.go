package ps5033

// The only fmt reference in this FILE is the fixable Append itself: fixing it
// would orphan the fmt import, and the runner never prunes imports, so the fix
// is withheld and the report stays advisory (golden identical).

import "fmt"

func orphan(buf []byte, s string) []byte {
	return fmt.Append(buf, s) // want `fmt\.Append with a single string operand`
}

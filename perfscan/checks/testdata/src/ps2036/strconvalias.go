package ps2036

// EXISTING-ALIAS positive: the file already imports strconv under an alias.
// The rewrite must reuse that alias (sc.AppendUint) instead of adding a
// second import of the same path. fmt stays referenced by another call, so
// the import survives.

import (
	"fmt"
	sc "strconv"
)

func keepAlias() { fmt.Println(sc.Itoa(2)) }

func aliasedStrconv(buf []byte, u uint16) []byte {
	return fmt.Append(buf, u) // want `fmt\.Append with a single int/uint/bool/float operand`
}

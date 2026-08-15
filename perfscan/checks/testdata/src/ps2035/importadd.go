package ps2035

// The file lacks a strconv import: the first fix adds it (into the sorted
// position of the import group). fmt stays referenced via keepFmt, so the
// import survives the rewrites.

import (
	"fmt"
)

func keepFmt() { fmt.Println("x") }

func addImportInt(buf []byte, i int) []byte {
	return fmt.Appendf(buf, "%v", i) // want `fmt\.Appendf of a single %v integer value`
}

// The second fix in the same file must NOT add the import again.
func addImportBool(buf []byte, b bool) []byte {
	return fmt.Appendf(buf, "%v", b) // want `fmt\.Appendf of a single %v bool value`
}

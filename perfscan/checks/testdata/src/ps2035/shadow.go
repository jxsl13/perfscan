package ps2035

// The strconv name is owned by a LOCAL at the call site and the file does
// not import the package: no usable qualifier exists, so the report stays
// advisory and nothing is rewritten.

import "fmt"

func keepFmtShadow() { fmt.Println("y") }

func shadowedStrconv(buf []byte, i int) []byte {
	strconv := len(buf)
	_ = strconv
	return fmt.Appendf(buf, "%v", i) // want `fmt\.Appendf of a single %v integer value`
}

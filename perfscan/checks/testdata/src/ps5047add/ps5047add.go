package ps5047addadd

import "fmt"

// The file lacks a strconv import: the first fix adds it. fmt stays
// referenced via keepFmt.
func addImport(dst []byte, n int) []byte {
	return fmt.Appendf(dst, "%b", n) // want `fmt\.Appendf\(buf, "%b", n\) parses the format and boxes n to binary-print one integer; strconv\.AppendInt/AppendUint appends the base-2 digits directly`
}

func keepFmt() { fmt.Println("keep") }

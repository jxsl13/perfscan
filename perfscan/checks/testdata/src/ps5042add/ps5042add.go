package ps5042add

import "fmt"

// The file lacks a strconv import: the first fix adds it. fmt stays
// referenced via keepFmt.
func addImport(dst []byte, n int) []byte {
	return fmt.Appendf(dst, "%d", n) // want `fmt\.Appendf\(buf, "%d", n\) parses the format and boxes n to print one integer; strconv\.AppendInt/AppendUint appends the decimal digits directly`
}

func keepFmt() { fmt.Println("keep") }

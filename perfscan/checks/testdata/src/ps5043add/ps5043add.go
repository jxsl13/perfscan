package ps5043add

import "fmt"

// The file lacks a strconv import: the first fix adds it. fmt stays
// referenced via keepFmt.
func addImport(dst []byte, n int) []byte {
	return fmt.Appendf(dst, "%x", n) // want `fmt\.Appendf\(buf, "%x", n\) parses the format and boxes n to hex-print one integer; strconv\.AppendInt/AppendUint appends the lowercase hex digits directly`
}

func keepFmt() { fmt.Println("keep") }

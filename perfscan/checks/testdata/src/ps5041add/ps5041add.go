package ps5041add

import "fmt"

// The file lacks a strconv import: the first fix adds it. fmt stays
// referenced via keepFmt, so the fmt import survives the rewrite.
func addImport(dst []byte, s string) []byte {
	return fmt.Appendf(dst, "%q", s) // want `fmt\.Appendf\(buf, "%q", s\) parses the format and boxes s to quote one string; strconv\.AppendQuote\(buf, s\) appends the identical Go string literal directly`
}

func keepFmt() { fmt.Println("keep") }

package ps5059add

import "fmt"

var _ = fmt.Println // keeps fmt alive after the rewrite

func addStrconv(buf []byte, n int) []byte {
	return fmt.Appendf(buf, "id=%d;", n) // want `fmt\.Appendf splicing one integer verb into literal text`
}

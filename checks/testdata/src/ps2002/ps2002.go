package ps2002

import (
	"bytes"
	"strings"
)

func unsized(parts []string) string {
	var b strings.Builder
	for _, s := range parts {
		b.WriteString(s) // want `b is written in a loop with no b\.Grow call in this function; a capacity hint removes the geometric growth copies`
	}
	return b.String()
}

func sized(parts []string, total int) string {
	var b strings.Builder
	b.Grow(total)
	for _, s := range parts {
		b.WriteString(s)
	}
	return b.String()
}

func buffer(parts [][]byte) []byte {
	var buf bytes.Buffer
	for _, p := range parts {
		buf.Write(p) // want `buf is written in a loop with no buf\.Grow call in this function; a capacity hint removes the geometric growth copies`
	}
	return buf.Bytes()
}

func outsideLoop(s string) string {
	var b strings.Builder
	b.WriteString(s)
	return b.String()
}

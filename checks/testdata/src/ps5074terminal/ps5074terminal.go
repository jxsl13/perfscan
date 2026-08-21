package ps5074terminal

import (
	"bytes"
	"strings"
)

// PS5084 owns these full terminal rewrites; PS5074 must not emit overlapping
// nested-clone diagnostics even when run independently.
func copyTerminal(dst, src []byte) int {
	return copy(dst, bytes.Clone(bytes.Clone(src)))
}

func conversionTerminal(src string) []byte {
	return []byte(strings.Clone(strings.Clone(src)))
}

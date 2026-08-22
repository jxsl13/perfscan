package ps5084dot

import . "bytes"

// Dot-imported Clone is deliberately outside the import-liveness rewrite.
func dotImport(dst, src []byte) int {
	return copy(dst, Clone(src))
}

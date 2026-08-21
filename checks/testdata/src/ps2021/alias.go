package ps2021

import str "strings"

// An aliased strings import keeps its qualifier in the rewrite.
func aliased(s string) string {
	parts := str.Split(s, ",") // want `strings\.Split with a single-byte separator allocates every field just to read the last one; s\[strings\.LastIndexByte\(s, c\)\+1:\] is bit-identical with zero allocations`
	return parts[len(parts)-1]
}

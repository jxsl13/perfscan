package ps3001

import f "fmt"

// The stdlib fmt imported under an alias still resolves to the same
// package: the check must fire through the alias exactly as it does
// through the canonical qualifier.
func aliasedParse(lines []string) (int, int) {
	var a, b int
	for _, line := range lines {
		f.Sscanf(line, "%d %d", &a, &b) // want `fmt\.Sscanf in a loop pays format parsing and reflection per iteration; use strconv or manual parsing`
	}
	return a, b
}

// NEGATIVE: outside a loop the per-call cost is paid once: silent.
func aliasedOnce(line string) (int, int) {
	var a, b int
	f.Sscanf(line, "%d %d", &a, &b)
	return a, b
}

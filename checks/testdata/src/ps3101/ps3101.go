package ps3101

import "bytes"

func use(bool) {}

func invariantConv(lines [][]byte, sep string) {
	for _, line := range lines {
		use(bytes.Contains(line, []byte(sep))) // want `\[\]byte\(sep\) copies its operand on every iteration but sep is loop-invariant; hoist the conversion above the loop`
	}
}

func stringConv(bufs [][]byte, key []byte) map[string]bool {
	seen := map[string]bool{}
	for range bufs {
		seen[string(key)] = true // want `string\(key\) copies its operand on every iteration but key is loop-invariant; hoist the conversion above the loop`
	}
	return seen
}

// The operand is the loop variable: it changes per iteration, silent.
func perItemConv(lines []string) [][]byte {
	out := make([][]byte, 0, len(lines))
	for _, line := range lines {
		out = append(out, []byte(line))
	}
	return out
}

// Reassigned in the body: not invariant, silent.
func reassigned(lines [][]byte, sep string) {
	for _, line := range lines {
		use(bytes.Contains(line, []byte(sep)))
		sep = "y"
	}
}

func hoisted(lines [][]byte, sep string) {
	bsep := []byte(sep)
	for _, line := range lines {
		use(bytes.Contains(line, bsep))
	}
}

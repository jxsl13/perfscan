package ps2003

import "strings"

func emit(string) {}

func inLoop(names []string) {
	for _, name := range names {
		emit(strings.ReplaceAll(name, "-", "_")) // want `strings\.ReplaceAll in a loop allocates a fresh string per iteration; hoist the transform, build a strings\.Replacer once, or reuse a byte buffer`
	}
}

func repeatInLoop(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += strings.Repeat("x", i) // want `strings\.Repeat in a loop allocates a fresh string per iteration; hoist the transform, build a strings\.Replacer once, or reuse a byte buffer`
	}
	return s
}

func outside(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}

func replacer(names []string) {
	r := strings.NewReplacer("-", "_")
	for _, name := range names {
		emit(r.Replace(name))
	}
}

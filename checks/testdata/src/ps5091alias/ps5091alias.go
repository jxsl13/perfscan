package ps5091alias

import text "strings"

func lookup(values map[string]int, key string) int {
	return values[text.Clone(text.Clone(key))] // want "read-only map lookup key consumes 2 throwaway strings.Clone layer"
}

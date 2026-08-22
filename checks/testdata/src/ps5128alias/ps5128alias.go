package ps5128alias

import text "strings"

func split(value, separator string) []string {
	// want +1 `strings.Contains scans before strings.Split repeats the separator search`
	if text.Contains(value, separator) {
		return text.Split(value, separator)
	}
	return []string{value}
}

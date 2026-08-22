package ps5125alias

import text "strings"

func replace(value string) string {
	// want +1 `strings.Contains scans before strings.Replace repeats the same lookup`
	if text.Contains(value, ":") {
		value = text.Replace(value, ":", "-", 1)
	}
	return value
}

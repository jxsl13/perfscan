package ps5123alias

import text "strings"

func locate(value string) int {
	// want +1 `strings.Contains searches before strings.Index repeats the same lookup`
	if text.Contains(value, ":") {
		return text.Index(value, ":")
	}
	return -1
}

package ps5126alias

import text "strings"

func locate(value string) int {
	// want +1 `strings.Contains searches forward before strings.LastIndex repeats the lookup backward`
	if text.Contains(value, ":") {
		return text.LastIndex(value, ":")
	}
	return -1
}

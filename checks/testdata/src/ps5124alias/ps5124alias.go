package ps5124alias

import text "strings"

func count(value string) int {
	// want +1 `strings.Contains searches before strings.Count repeats the lookup and counts all matches`
	if text.Contains(value, ":") {
		return text.Count(value, ":")
	}
	return 0
}

package ps5122alias

import s "strings"

func clean(value string) string {
	// want +1 `strings.Contains scans before strings.ReplaceAll repeats the match work`
	if s.Contains(value, ":") {
		value = s.ReplaceAll(value, ":", "-")
	}
	return value
}

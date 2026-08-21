package ps5119alias

import text "strings"

func trim(value, prefix string) string {
	if text.HasPrefix(value, prefix) { // want `strings.HasPrefix proves the boundary and strings.TrimPrefix immediately repeats that proof; strings.CutPrefix returns the identical remainder and predicate in one direct call`
		return text.TrimPrefix(value, prefix)
	}
	return value
}

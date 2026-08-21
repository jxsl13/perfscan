package ps5079alias

import text "strings"

func alias(s string) string {
	return text.TrimPrefix(text.TrimRight(s, ""), "") // want `2 adjacent strings boundary operation\(s\) use an empty prefix`
}

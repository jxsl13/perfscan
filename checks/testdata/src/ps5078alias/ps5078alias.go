package ps5078alias

import text "strings"

func alias(s string) string {
	return text.Trim(text.TrimSpace(s), " \t") // want `strings\.TrimSpace precedes 1 adjacent constant whitespace-only Trim layer`
}

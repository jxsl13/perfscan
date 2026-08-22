package ps5077alias

import text "strings"

const cut = "ab"

func alias(s string) string {
	return text.Trim(text.Trim(s, cut), "ab") // want `strings\.Trim is applied 2 times with the same constant cutset`
}

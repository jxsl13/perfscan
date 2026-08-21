package ps5112alias

import text "strings"

func inverse(s string) string {
	return text.Join(text.SplitAfterN(s, "::", -2), "") // want `strings.Join exactly reverses strings.SplitAfterN and reconstructs its original plain-string input`
}

package ps5118alias

import text "strings"

func canonical(payload string) string {
	return text.ReplaceAll(text.Replace(payload, "x", "", -1), "x", "unused") // want `strings.Replace eliminates byte "x", so 1 enclosing Replace/ReplaceAll pass\(es\) cannot change the proven result`
}

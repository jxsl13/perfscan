package ps5118comment

import "strings"

func canonical(payload string) string {
	return strings.ReplaceAll( /* preserve sanitizer boundary */ strings.ReplaceAll(payload, "x", ""), "x", "unused") // want `strings.ReplaceAll eliminates byte "x", so 1 enclosing Replace/ReplaceAll pass\(es\) cannot change the proven result`
}

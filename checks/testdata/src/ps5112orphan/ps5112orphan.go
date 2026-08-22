package ps5112orphan

import "strings"

func inverse(s string) string {
	return strings.Join(strings.Split(s, ","), ",") // want `strings.Join exactly reverses strings.Split and reconstructs its original plain-string input`
}

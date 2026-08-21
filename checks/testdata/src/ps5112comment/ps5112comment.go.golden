package ps5112comment

import "strings"

func commented(s string) string {
	return strings.Join( /* retain inverse rationale */ strings.Split(s, ","), ",") // want `strings.Join exactly reverses strings.Split and reconstructs its original plain-string input`
}

func localConstant(s string) string {
	const separator = ","
	return strings.Join(strings.Split(s, separator), separator) // want `strings.Join exactly reverses strings.Split and reconstructs its original plain-string input`
}

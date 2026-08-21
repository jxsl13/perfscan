package ps5117comment

import "strings"

func canonical(payload string) string {
	return strings.Join(strings.Fields( /* preserve normalization boundary */ strings.Join(strings.Fields(payload), " ")), " ") // want `strings.Join\+Fields canonicalization is applied 2 times to an already canonical result; remove 1 redundant scan/allocation layer`
}
